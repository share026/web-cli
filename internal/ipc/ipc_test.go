package ipc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	msg := []byte(`{"type":"ping","payload":{"n":1}}`)
	if err := WriteNativeMessage(&buf, msg); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	if got := binary.NativeEndian.Uint32(raw[:4]); int(got) != len(msg) {
		t.Fatalf("length prefix = %d, want %d", got, len(msg))
	}
	got, err := ReadNativeMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("got %s", got)
	}
	if _, err := ReadNativeMessage(&buf); !errors.Is(err, io.EOF) {
		t.Fatalf("want io.EOF at clean end, got %v", err)
	}
}

func TestNativeMessageErrors(t *testing.T) {
	// truncated body
	var buf bytes.Buffer
	var hdr [4]byte
	binary.NativeEndian.PutUint32(hdr[:], 10)
	buf.Write(hdr[:])
	buf.WriteString(`{"a"`)
	if _, err := ReadNativeMessage(&buf); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("want truncated error, got %v", err)
	}
	// oversize header
	buf.Reset()
	binary.NativeEndian.PutUint32(hdr[:], MaxFromBrowser+1)
	buf.Write(hdr[:])
	if _, err := ReadNativeMessage(&buf); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	// invalid JSON
	buf.Reset()
	binary.NativeEndian.PutUint32(hdr[:], 3)
	buf.Write(hdr[:])
	buf.WriteString("abc")
	if _, err := ReadNativeMessage(&buf); err == nil {
		t.Fatal("want invalid JSON error")
	}
	// too large to browser
	big := make([]byte, MaxToBrowser+1)
	if err := WriteNativeMessage(io.Discard, big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}

func startHub(t *testing.T) *Hub {
	t.Helper()
	h, err := Listen(filepath.Join(t.TempDir(), "ipc.sock"))
	if err != nil {
		t.Fatal(err)
	}
	h.Logf = t.Logf
	go h.Serve()
	t.Cleanup(func() { h.Close() })
	return h
}

func TestHubPingPongAndRequest(t *testing.T) {
	h := startHub(t)
	events := make(chan Message, 1)
	h.OnEvent = func(_ *Session, m Message) { events <- m }

	c, err := Dial(h.Path(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	hello, _ := NewMessage("", TypeHello, Hello{Origin: "chrome-extension://test/", PID: 42})
	c.Send(hello)

	// extension-initiated ping is answered by the hub itself
	if err := c.Send(Message{ID: "p1", Type: TypePing}); err != nil {
		t.Fatal(err)
	}
	m, err := c.Recv()
	if err != nil || m.Type != TypePong || m.ID != "p1" {
		t.Fatalf("got %+v %v", m, err)
	}

	// app-initiated request: fake extension answers
	go func() {
		for {
			req, err := c.Recv()
			if err != nil {
				return
			}
			switch req.Type {
			case TypeCollect:
				c.Send(Message{ID: req.ID, Type: TypeElements, Payload: json.RawMessage(`{"elements":[]}`)})
			case TypeClick:
				c.Send(Message{ID: req.ID, Type: TypeError, Error: "hint 9 is stale"})
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := h.Request(ctx, TypeCollect, nil)
	if err != nil || r.Type != TypeElements {
		t.Fatalf("collect: %+v %v", r, err)
	}
	if s := h.Current(); s == nil || s.Hello.PID != 42 {
		t.Fatalf("hello not recorded: %+v", s)
	}
	_, err = h.Request(ctx, TypeClick, map[string]int{"hint": 9})
	var re *ErrRemote
	if !errors.As(err, &re) || re.Msg != "hint 9 is stale" {
		t.Fatalf("want remote error, got %v", err)
	}

	// unsolicited messages go to OnEvent
	c.Send(Message{Type: TypeLog, Payload: json.RawMessage(`"hi"`)})
	select {
	case m := <-events:
		if m.Type != TypeLog {
			t.Fatalf("event %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
}

func TestHubDisconnectFailsPending(t *testing.T) {
	h := startHub(t)
	c, err := Dial(h.Path(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := h.WaitSession(ctx); err != nil {
		t.Fatal(err)
	}
	go func() {
		c.Recv() // read the request, then hang up without replying
		c.Close()
	}()
	if _, err := h.Request(ctx, TypeCollect, nil); err == nil || !strings.Contains(err.Error(), "disconnected") {
		t.Fatalf("want disconnected error, got %v", err)
	}
}

func TestListenRejectsLiveSocket(t *testing.T) {
	h := startHub(t)
	if _, err := Listen(h.Path()); err == nil {
		t.Fatal("second Listen on a live socket must fail")
	}
}
