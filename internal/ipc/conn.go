package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// maxLine bounds a single NDJSON line on the socket (matches the largest
// message Chrome may send to a native host).
const maxLine = MaxFromBrowser + 1024

// Conn is a framed NDJSON connection over a Unix Domain Socket.
// Send is safe for concurrent use; Recv must be called from one goroutine.
type Conn struct {
	c  net.Conn
	sc *bufio.Scanner
	mu sync.Mutex
}

// NewConn wraps an established net.Conn.
func NewConn(c net.Conn) *Conn {
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 64*1024), maxLine)
	return &Conn{c: c, sc: sc}
}

// Dial connects to the app socket, retrying until timeout elapses.
func Dial(path string, timeout time.Duration) (*Conn, error) {
	deadline := time.Now().Add(timeout)
	delay := 50 * time.Millisecond
	for {
		c, err := net.Dial("unix", path)
		if err == nil {
			return NewConn(c), nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("dial %s: %w", path, err)
		}
		time.Sleep(delay)
		if delay < time.Second {
			delay *= 2
		}
	}
}

// SendRaw writes one pre-encoded JSON value as a single line.
func (c *Conn) SendRaw(raw []byte) error {
	if !json.Valid(raw) {
		return fmt.Errorf("SendRaw: invalid JSON")
	}
	line := make([]byte, 0, len(raw)+1)
	line = append(line, raw...)
	line = append(line, '\n')
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.c.Write(line)
	return err
}

// Send encodes and writes a Message.
func (c *Conn) Send(m Message) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return c.SendRaw(raw)
}

// RecvRaw reads the next line (one JSON value). The returned slice is a copy.
func (c *Conn) RecvRaw() ([]byte, error) {
	if !c.sc.Scan() {
		if err := c.sc.Err(); err != nil {
			return nil, err
		}
		return nil, net.ErrClosed
	}
	b := c.sc.Bytes()
	out := make([]byte, len(b))
	copy(out, b)
	return out, nil
}

// Recv reads and decodes the next Message.
func (c *Conn) Recv() (Message, error) {
	var m Message
	raw, err := c.RecvRaw()
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("decode message: %w", err)
	}
	return m, nil
}

// Close closes the underlying socket.
func (c *Conn) Close() error { return c.c.Close() }
