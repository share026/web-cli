package proxy

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"

	"github.com/share026/web-cli/internal/audit"
)

// wsEcho is a real WebSocket server (gobwas/ws) that answers "echo:<msg>".
func wsEcho(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		return
	}
	go func() {
		defer conn.Close()
		for {
			msg, op, err := wsutil.ReadClientData(conn)
			if err != nil {
				return
			}
			if err := wsutil.WriteServerMessage(conn, op, append([]byte("echo:"), msg...)); err != nil {
				return
			}
		}
	}()
}

// rawFrame builds a single frame by hand so the parser is tested against the
// RFC, not against another library's writer.
func rawFrame(fin, rsv1 bool, op byte, payload []byte, mask bool) []byte {
	var b bytes.Buffer
	b0 := op
	if fin {
		b0 |= 0x80
	}
	if rsv1 {
		b0 |= 0x40
	}
	b.WriteByte(b0)
	var m byte
	if mask {
		m = 0x80
	}
	switch n := len(payload); {
	case n < 126:
		b.WriteByte(m | byte(n))
	case n < 1<<16:
		b.WriteByte(m | 126)
		binary.Write(&b, binary.BigEndian, uint16(n))
	default:
		b.WriteByte(m | 127)
		binary.Write(&b, binary.BigEndian, uint64(n))
	}
	p := append([]byte(nil), payload...)
	if mask {
		key := []byte{0x12, 0x34, 0x56, 0x78}
		b.Write(key)
		for i := range p {
			p[i] ^= key[i&3]
		}
	}
	b.Write(p)
	return b.Bytes()
}

func TestWSParserFramingAndDeflate(t *testing.T) {
	var got []audit.WSFrame
	p := &wsParser{dir: "send", deflate: true, emit: func(f audit.WSFrame) { got = append(got, f) }}

	var stream bytes.Buffer
	// 1. fragmented, masked text message
	stream.Write(rawFrame(false, false, 0x1, []byte("hel"), true))
	stream.Write(rawFrame(true, false, 0x0, []byte("lo"), true))
	// 2. ping between messages
	stream.Write(rawFrame(true, false, 0x9, []byte("p"), true))
	// 3+4. two compressed messages sharing one deflate context (context
	// takeover): the second is mostly back-references into the first.
	fw, _ := flate.NewWriter(nil, flate.BestCompression)
	msgA := strings.Repeat("the quick brown fox ", 20)
	msgB := strings.Repeat("the quick brown fox ", 21) + "!"
	var comp bytes.Buffer
	fw.Reset(&comp)
	var parts [][]byte
	for _, m := range []string{msgA, msgB} {
		start := comp.Len()
		fw.Write([]byte(m))
		fw.Flush()
		part := append([]byte(nil), comp.Bytes()[start:]...)
		parts = append(parts, bytes.TrimSuffix(part, []byte{0, 0, 0xff, 0xff}))
	}
	stream.Write(rawFrame(true, true, 0x1, parts[0], true))
	stream.Write(rawFrame(true, true, 0x1, parts[1], true))
	// 5. large binary frame (16-bit length) and close with a code
	big := bytes.Repeat([]byte{0xAB}, 70000)
	stream.Write(rawFrame(true, false, 0x2, big, true))
	stream.Write(rawFrame(true, false, 0x8, append([]byte{0x03, 0xE8}, "bye"...), true))

	// Feed in awkward 7-byte chunks to exercise incremental parsing.
	all := stream.Bytes()
	for i := 0; i < len(all); i += 7 {
		p.feed(all[i:min(i+7, len(all))])
	}

	if len(got) != 6 {
		t.Fatalf("want 6 frames, got %d: %+v", len(got), got)
	}
	check := func(i int, op, data string) {
		t.Helper()
		if got[i].Opcode != op || string(got[i].Data) != data {
			t.Errorf("frame %d: got %s %q (note %q), want %s %q", i, got[i].Opcode, trunc(got[i].Data), got[i].Note, op, trunc([]byte(data)))
		}
	}
	check(0, "text", "hello")
	check(1, "ping", "p")
	check(2, "text", msgA)
	check(3, "text", msgB)
	if !got[3].Compressed || got[3].Len != len(msgB) {
		t.Errorf("frame 3 should be compressed with inflated len %d: %+v", len(msgB), got[3])
	}
	if got[4].Opcode != "binary" || got[4].Len != 70000 || !got[4].Truncated || len(got[4].Data) != wsMaxStored {
		t.Errorf("binary frame: %+v (stored %d)", got[4], len(got[4].Data))
	}
	check(5, "close", "bye")
	if got[5].Note != "code=1000" {
		t.Errorf("close note = %q", got[5].Note)
	}
}

func trunc(b []byte) string {
	if len(b) > 40 {
		return string(b[:40]) + "…"
	}
	return string(b)
}

func TestParseWSExtensions(t *testing.T) {
	h := http.Header{}
	h.Set("Sec-WebSocket-Extensions", "permessage-deflate; server_no_context_takeover; client_max_window_bits=15")
	d := parseWSExtensions(h)
	if !d.enabled || !d.serverNoTakeover || d.clientNoTakeover {
		t.Fatalf("%+v", d)
	}
	if parseWSExtensions(http.Header{}).enabled {
		t.Fatal("no extension header must mean no deflate")
	}
}

// TestWebSocketThroughProxy dials a real WebSocket server through the MITM
// proxy (HTTPS/CONNECT) and checks that traffic flows and frames are logged.
func TestWebSocketThroughProxy(t *testing.T) {
	f := newFixture(t)
	req, _ := http.NewRequest("GET", f.up.URL+"/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	// Transport directly: http.Client.Timeout wraps the body and hides Write.
	resp, err := f.client.Transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status %d", resp.StatusCode)
	}
	conn, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		t.Fatal("101 body is not a ReadWriteCloser")
	}
	for _, m := range []string{"hello", "日本語もOK"} {
		if err := wsutil.WriteClientText(conn, []byte(m)); err != nil {
			t.Fatal(err)
		}
		echo, err := wsutil.ReadServerText(conn)
		if err != nil {
			t.Fatal(err)
		}
		if string(echo) != "echo:"+m {
			t.Fatalf("echo %q", echo)
		}
	}
	_ = ws.WriteFrame(conn, ws.MaskFrame(ws.NewCloseFrame(ws.NewCloseFrameBody(ws.StatusNormalClosure, "done"))))
	conn.Close()

	deadline := time.Now().Add(3 * time.Second)
	for {
		exs := f.store.Exchanges(func(e *audit.Exchange) bool { return strings.HasSuffix(e.URL, "/ws") })
		if len(exs) == 1 && exs[0].WSFrameCount >= 5 {
			var lines []string
			for _, fr := range exs[0].WSFrames {
				lines = append(lines, fr.Dir+" "+fr.Opcode+" "+string(fr.Data))
			}
			joined := strings.Join(lines, "\n")
			for _, want := range []string{"send text hello", "recv text echo:hello", "send text 日本語もOK", "recv text echo:日本語もOK", "send close done"} {
				if !strings.Contains(joined, want) {
					t.Errorf("missing %q in frames:\n%s", want, joined)
				}
			}
			if exs[0].Status != 101 {
				t.Errorf("status %d", exs[0].Status)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("frames not captured: %+v", exs)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestThrottleRule(t *testing.T) {
	f := newFixture(t)
	r, err := ParseRule(`throttle url=/slow latency=300ms kbps=800`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.rules.Add(r); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRule(`throttle host=x`); err == nil {
		t.Fatal("throttle without parameters must be rejected")
	}

	// Unthrottled baseline.
	t0 := time.Now()
	resp, err := f.client.Get(f.up.URL + "/fast")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	fast := time.Since(t0)

	// 50 KB up + ~50 KB echoed back at 800 kbit/s (100 KB/s) ≈ 1 s, plus 300 ms.
	body := strings.Repeat("x", 50_000)
	t0 = time.Now()
	resp, err = f.client.Post(f.up.URL+"/slow", "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	slow := time.Since(t0)
	if !bytes.Contains(got, []byte(body)) {
		t.Fatal("throttled body corrupted")
	}
	t.Logf("fast=%v slow=%v", fast, slow)
	if slow < 1100*time.Millisecond || slow > 4*time.Second {
		t.Fatalf("throttled request took %v, want ≈1.3s", slow)
	}
	if fast > 500*time.Millisecond {
		t.Fatalf("unmatched request was slowed: %v", fast)
	}
	exs := f.waitExchanges(t, 2)
	if len(exs[1].Intercepts) == 0 || !strings.Contains(exs[1].Intercepts[0], "throttle") {
		t.Fatalf("intercept note missing: %+v", exs[1].Intercepts)
	}
}
