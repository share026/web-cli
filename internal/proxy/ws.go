package proxy

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/share026/web-cli/internal/audit"
)

// WebSocket capture.
//
// goproxy handles the 101 upgrade itself: after our OnResponse handler it
// type-asserts resp.Body to io.ReadWriter and pumps bytes in both directions
// (Read = server→client, Write = client→server). We therefore replace the body
// with wsTap, which forwards every byte untouched and feeds a copy into an
// incremental RFC 6455 parser per direction. Parsing is purely observational:
// a parse error disables capture for that direction but never alters traffic.

const (
	wsMaxStored  = 64 << 10 // bytes of payload kept per message
	wsMaxMessage = 16 << 20 // larger messages are counted, not buffered/inflated
	wsWindow     = 32 << 10 // LZ77 window for permessage-deflate context takeover
)

var wsOpNames = map[byte]string{0x1: "text", 0x2: "binary", 0x8: "close", 0x9: "ping", 0xA: "pong"}

// wsDeflate describes negotiated permessage-deflate parameters (RFC 7692).
type wsDeflate struct {
	enabled                            bool
	serverNoTakeover, clientNoTakeover bool
}

func parseWSExtensions(h http.Header) wsDeflate {
	var d wsDeflate
	for _, v := range h.Values("Sec-WebSocket-Extensions") {
		for _, ext := range strings.Split(v, ",") {
			parts := strings.Split(ext, ";")
			if strings.TrimSpace(parts[0]) != "permessage-deflate" {
				continue
			}
			d.enabled = true
			for _, p := range parts[1:] {
				switch strings.TrimSpace(strings.SplitN(p, "=", 2)[0]) {
				case "server_no_context_takeover":
					d.serverNoTakeover = true
				case "client_no_context_takeover":
					d.clientNoTakeover = true
				}
			}
			return d // only the first accepted extension applies
		}
	}
	return d
}

// wsParser reassembles messages from a byte stream of frames.
type wsParser struct {
	dir        string
	deflate    bool
	noTakeover bool
	emit       func(audit.WSFrame)

	buf    []byte
	failed bool

	// current (possibly fragmented) data message
	inMsg      bool
	msgOp      byte
	msgRSV1    bool
	msg        []byte
	msgLen     int
	msgTooBig  bool
	skip       int64 // payload bytes of an oversized frame still to discard
	pendingFin bool  // the oversized frame being skipped ends its message

	window []byte // uncompressed history for context takeover
}

func (p *wsParser) feed(b []byte) {
	if p.failed || len(b) == 0 {
		return
	}
	if p.skip > 0 {
		n := int64(len(b))
		if n > p.skip {
			n = p.skip
		}
		p.skip -= n
		b = b[n:]
		if p.skip == 0 && p.pendingFin {
			p.finishMessage()
		}
		if len(b) == 0 {
			return
		}
	}
	p.buf = append(p.buf, b...)
	for !p.failed {
		n, ok := p.frame(p.buf)
		if !ok {
			break
		}
		p.buf = p.buf[n:]
		if p.skip > 0 {
			break
		}
	}
	if len(p.buf) == 0 {
		p.buf = nil // release memory between frames
	}
}

// frame parses one frame from b; ok=false means more bytes are needed.
func (p *wsParser) frame(b []byte) (int, bool) {
	if len(b) < 2 {
		return 0, false
	}
	fin, rsv1, op := b[0]&0x80 != 0, b[0]&0x40 != 0, b[0]&0x0F
	masked, l := b[1]&0x80 != 0, int64(b[1]&0x7F)
	h := 2
	switch l {
	case 126:
		if len(b) < 4 {
			return 0, false
		}
		l, h = int64(binary.BigEndian.Uint16(b[2:4])), 4
	case 127:
		if len(b) < 10 {
			return 0, false
		}
		l, h = int64(binary.BigEndian.Uint64(b[2:10])), 10
	}
	var key [4]byte
	if masked {
		if len(b) < h+4 {
			return 0, false
		}
		copy(key[:], b[h:h+4])
		h += 4
	}
	if l < 0 || l > wsMaxMessage {
		// Too large to buffer: account for it and discard the payload.
		p.startOrContinue(op, rsv1, fin)
		p.msgTooBig = true
		p.msgLen += int(min(l, 1<<40))
		avail := int64(len(b) - h)
		if avail >= l {
			if fin {
				p.finishMessage()
			}
			return h + int(l), true
		}
		p.skip = l - avail
		p.pendingFin = fin
		return len(b), true
	}
	if int64(len(b)-h) < l {
		return 0, false
	}
	payload := append([]byte(nil), b[h:h+int(l)]...)
	if masked {
		for i := range payload {
			payload[i] ^= key[i&3]
		}
	}
	if op >= 0x8 { // control frame, never fragmented or compressed
		f := audit.WSFrame{Time: time.Now(), Dir: p.dir, Opcode: opName(op), Len: len(payload)}
		if op == 0x8 && len(payload) >= 2 {
			f.Note = fmt.Sprintf("code=%d", binary.BigEndian.Uint16(payload[:2]))
			payload = payload[2:]
		}
		f.Data = payload
		p.emit(f)
		return h + int(l), true
	}
	if op != 0 && p.inMsg || op == 0 && !p.inMsg {
		p.failed = true // protocol violation; stop observing this direction
		return 0, false
	}
	p.startOrContinue(op, rsv1, fin)
	p.msgLen += len(payload)
	if !p.msgTooBig {
		if len(p.msg)+len(payload) > wsMaxMessage {
			p.msgTooBig, p.msg = true, nil
		} else {
			p.msg = append(p.msg, payload...)
		}
	}
	if fin {
		p.finishMessage()
	}
	return h + int(l), true
}

func (p *wsParser) startOrContinue(op byte, rsv1, fin bool) {
	if op >= 0x8 {
		return
	}
	if op != 0 {
		p.inMsg, p.msgOp, p.msgRSV1, p.msg, p.msgLen, p.msgTooBig = true, op, rsv1, nil, 0, false
	}
}

func (p *wsParser) finishMessage() {
	p.pendingFin = false
	f := audit.WSFrame{Time: time.Now(), Dir: p.dir, Opcode: opName(p.msgOp), Len: p.msgLen}
	data := p.msg
	if p.msgRSV1 && p.deflate {
		f.Compressed = true
		if p.msgTooBig {
			f.Note = "compressed message too large to inflate"
			data = nil
			p.window = nil // history is lost; later messages may fail to inflate
		} else if out, err := p.inflate(data); err != nil {
			f.Note = "inflate: " + err.Error()
			data = nil
		} else {
			data = out
			f.Len = len(out)
		}
	}
	if len(data) > wsMaxStored {
		data, f.Truncated = data[:wsMaxStored], true
	}
	if p.msgTooBig && !f.Compressed {
		f.Truncated = true
	}
	f.Data = append([]byte(nil), data...)
	p.inMsg, p.msg, p.msgLen, p.msgTooBig = false, nil, 0, false
	p.emit(f)
}

// inflate decompresses one permessage-deflate message. With context takeover
// the sender's LZ77 window spans messages, which is exactly a preset
// dictionary made of the previously inflated bytes.
func (p *wsParser) inflate(data []byte) ([]byte, error) {
	var dict []byte
	if !p.noTakeover {
		dict = p.window
	}
	src := io.MultiReader(bytes.NewReader(data), bytes.NewReader([]byte{0x00, 0x00, 0xff, 0xff}))
	r := flate.NewReaderDict(src, dict)
	out, err := io.ReadAll(io.LimitReader(r, wsMaxMessage+1))
	// The stream ends at a sync-flush boundary, not a final block.
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if len(out) > wsMaxMessage {
		return nil, errors.New("inflated message too large")
	}
	if !p.noTakeover {
		p.window = append(p.window, out...)
		if len(p.window) > wsWindow {
			p.window = append([]byte(nil), p.window[len(p.window)-wsWindow:]...)
		}
	}
	return out, nil
}

func opName(op byte) string {
	if n, ok := wsOpNames[op]; ok {
		return n
	}
	return fmt.Sprintf("op%d", op)
}

// wsTap wraps the upgraded upstream connection.
type wsTap struct {
	rw         io.ReadWriteCloser
	send, recv *wsParser // each direction is fed by its own goroutine; the store locks itself
	once       sync.Once
	done       func(err error)
}

func (t *wsTap) Read(b []byte) (int, error) { // server → client
	n, err := t.rw.Read(b)
	if n > 0 {
		t.recv.feed(b[:n])
	}
	if err != nil {
		t.finish(err)
	}
	return n, err
}

func (t *wsTap) Write(b []byte) (int, error) { // client → server
	t.send.feed(b)
	n, err := t.rw.Write(b)
	if err != nil {
		t.finish(err)
	}
	return n, err
}

func (t *wsTap) Close() error {
	t.finish(nil)
	return t.rw.Close()
}

func (t *wsTap) finish(err error) {
	t.once.Do(func() {
		if errors.Is(err, io.EOF) {
			err = nil
		}
		t.done(err)
	})
}

func newWSTap(rw io.ReadWriteCloser, respHeader http.Header, emit func(audit.WSFrame), done func(error)) *wsTap {
	d := parseWSExtensions(respHeader)
	return &wsTap{
		rw:   rw,
		send: &wsParser{dir: "send", deflate: d.enabled, noTakeover: d.clientNoTakeover, emit: emit},
		recv: &wsParser{dir: "recv", deflate: d.enabled, noTakeover: d.serverNoTakeover, emit: emit},
		done: done,
	}
}
