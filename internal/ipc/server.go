package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Session is one connected nm-host (i.e. one browser extension instance).
type Session struct {
	ID        int64
	Hello     Hello
	Connected time.Time
	conn      *Conn
	hub       *Hub
	done      chan struct{}
}

// Send writes a message to the extension behind this session.
func (s *Session) Send(m Message) error { return s.conn.Send(m) }

// Done is closed when the session disconnects.
func (s *Session) Done() <-chan struct{} { return s.done }

// EventFunc receives messages that are not replies to a pending request.
type EventFunc func(s *Session, m Message)

// Hub listens on the Unix socket and multiplexes request/response pairs
// over every connected nm-host session.
type Hub struct {
	ln      net.Listener
	path    string
	nextID  atomic.Int64
	OnEvent EventFunc
	Logf    func(format string, args ...any)

	mu       sync.Mutex
	closed   bool
	handlers sync.WaitGroup
	sessions []*Session
	pending  map[string]chan Message
	changed  chan struct{} // closed and replaced whenever sessions change
}

// Listen creates the Unix socket. A stale socket file left by a crashed
// process is removed; a live one (another app instance) is an error.
func Listen(path string) (*Hub, error) {
	if _, err := os.Stat(path); err == nil {
		if c, err := net.DialTimeout("unix", path, 300*time.Millisecond); err == nil {
			c.Close()
			return nil, fmt.Errorf("another app instance is already listening on %s", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// Only the current user may talk to the browser bridge.
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	h := &Hub{
		ln:      ln,
		path:    path,
		pending: map[string]chan Message{},
		changed: make(chan struct{}),
		Logf:    func(string, ...any) {},
	}
	return h, nil
}

// Path returns the socket path.
func (h *Hub) Path() string { return h.path }

// Serve accepts connections until Close is called.
func (h *Hub) Serve() error {
	for {
		c, err := h.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		h.mu.Lock()
		if h.closed {
			h.mu.Unlock()
			c.Close()
			return nil
		}
		h.handlers.Add(1)
		h.mu.Unlock()
		go h.handle(NewConn(c))
	}
}

// Close stops listening, disconnects every session, waits for their
// handlers to finish and removes the socket file. It is idempotent.
func (h *Hub) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	err := h.ln.Close()
	for _, s := range h.sessions {
		s.conn.Close()
	}
	h.mu.Unlock()
	h.handlers.Wait()
	_ = os.Remove(h.path)
	return err
}

func (h *Hub) notifyLocked() {
	close(h.changed)
	h.changed = make(chan struct{})
}

func (h *Hub) handle(c *Conn) {
	s := &Session{ID: h.nextID.Add(1), Connected: time.Now(), conn: c, hub: h, done: make(chan struct{})}
	h.mu.Lock()
	if h.closed { // Close ran between Accept and here
		h.mu.Unlock()
		c.Close()
		h.handlers.Done()
		return
	}
	h.sessions = append(h.sessions, s)
	h.notifyLocked()
	h.mu.Unlock()
	h.Logf("ipc: session #%d connected", s.ID)

	defer func() {
		defer h.handlers.Done()
		c.Close()
		close(s.done)
		h.mu.Lock()
		for i, x := range h.sessions {
			if x == s {
				h.sessions = append(h.sessions[:i], h.sessions[i+1:]...)
				break
			}
		}
		h.notifyLocked()
		h.mu.Unlock()
		h.Logf("ipc: session #%d disconnected", s.ID)
	}()

	for {
		m, err := c.Recv()
		if err != nil {
			return
		}
		switch {
		case m.Type == TypeHello:
			_ = json.Unmarshal(m.Payload, &s.Hello)
			h.Logf("ipc: session #%d hello origin=%s pid=%d", s.ID, s.Hello.Origin, s.Hello.PID)
		case m.Type == TypePing:
			// Liveness check initiated by the extension: answer immediately.
			_ = s.Send(Message{ID: m.ID, Type: TypePong, Payload: json.RawMessage(`{"from":"app"}`)})
			h.Logf("ipc: session #%d ping -> pong", s.ID)
		case m.ID != "" && h.deliver(m):
			// reply to a pending request
		default:
			if h.OnEvent != nil {
				h.OnEvent(s, m)
			}
		}
	}
}

func (h *Hub) deliver(m Message) bool {
	h.mu.Lock()
	ch, ok := h.pending[m.ID]
	if ok {
		delete(h.pending, m.ID)
	}
	h.mu.Unlock()
	if ok {
		ch <- m
	}
	return ok
}

// Sessions returns a snapshot of connected sessions.
func (h *Hub) Sessions() []*Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]*Session(nil), h.sessions...)
}

// Current returns the most recently connected session, or nil.
func (h *Hub) Current() *Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.sessions) == 0 {
		return nil
	}
	return h.sessions[len(h.sessions)-1]
}

// WaitSession blocks until at least one session is connected.
func (h *Hub) WaitSession(ctx context.Context) (*Session, error) {
	for {
		h.mu.Lock()
		ch := h.changed
		var s *Session
		if n := len(h.sessions); n > 0 {
			s = h.sessions[n-1]
		}
		h.mu.Unlock()
		if s != nil {
			return s, nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, fmt.Errorf("no browser session connected: %w", ctx.Err())
		}
	}
}

// ErrRemote wraps an error reported by the extension.
type ErrRemote struct{ Msg string }

func (e *ErrRemote) Error() string { return "extension: " + e.Msg }

// Request sends typ/payload to the current session and waits for the reply.
func (h *Hub) Request(ctx context.Context, typ string, payload any) (Message, error) {
	s, err := h.WaitSession(ctx)
	if err != nil {
		return Message{}, err
	}
	return h.RequestOn(ctx, s, typ, payload)
}

// RequestOn is Request against a specific session.
func (h *Hub) RequestOn(ctx context.Context, s *Session, typ string, payload any) (Message, error) {
	m, err := NewMessage(NewID(), typ, payload)
	if err != nil {
		return Message{}, err
	}
	ch := make(chan Message, 1)
	h.mu.Lock()
	h.pending[m.ID] = ch
	h.mu.Unlock()
	cleanup := func() {
		h.mu.Lock()
		delete(h.pending, m.ID)
		h.mu.Unlock()
	}
	if err := s.Send(m); err != nil {
		cleanup()
		return Message{}, err
	}
	select {
	case r := <-ch:
		if r.Type == TypeError || r.Error != "" {
			return r, &ErrRemote{Msg: r.Error}
		}
		return r, nil
	case <-s.done:
		cleanup()
		return Message{}, fmt.Errorf("session #%d disconnected while waiting for %s reply", s.ID, typ)
	case <-ctx.Done():
		cleanup()
		return Message{}, fmt.Errorf("waiting for %s reply: %w", typ, ctx.Err())
	}
}
