// Package audit records captured HTTP exchanges, links them to the DOM
// actions that caused them, and renders them as .http files and JSON logs.
package audit

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// ActionHeader is attached by the extension to requests fired right after a
// DOM action and stripped by the proxy before forwarding upstream.
const ActionHeader = "X-Audit-Action-Id"

// Action describes one DOM operation performed through the extension.
type Action struct {
	ID      string    `json:"id"`
	Time    time.Time `json:"time"`
	Kind    string    `json:"kind"` // e.g. "click", "focus"
	Hint    int       `json:"hint"`
	Tag     string    `json:"tag,omitempty"`
	Text    string    `json:"text,omitempty"`
	PageURL string    `json:"page_url,omitempty"`
}

// Exchange is one request/response pair seen by the proxy.
type Exchange struct {
	ID         int64         `json:"id"`
	Start      time.Time     `json:"start"`
	Duration   time.Duration `json:"duration_ns"`
	ActionID   string        `json:"action_id,omitempty"`
	Method     string        `json:"method"`
	URL        string        `json:"url"`
	Proto      string        `json:"proto"`
	ReqHeader  http.Header   `json:"request_headers"`
	ReqBody    []byte        `json:"-"`
	Status     int           `json:"status"`
	RespHeader http.Header   `json:"response_headers,omitempty"`
	RespBody   []byte        `json:"-"`
	Truncated  bool          `json:"truncated,omitempty"`
	Blocked    bool          `json:"blocked,omitempty"`
	Intercepts []string      `json:"intercepts,omitempty"` // human readable notes of applied rules
	Error      string        `json:"error,omitempty"`
	// WebSocket: frames seen after a 101 upgrade (capped at MaxWSFrames;
	// WSFrameCount keeps counting past the cap).
	WSFrames     []WSFrame `json:"-"`
	WSFrameCount int       `json:"ws_frames,omitempty"`
}

// MaxWSFrames caps how many WebSocket frames are retained per connection.
const MaxWSFrames = 2000

// WSFrame is one WebSocket message (fragments reassembled, permessage-deflate
// already inflated) or control frame seen on an upgraded connection.
type WSFrame struct {
	Time       time.Time `json:"time"`
	Dir        string    `json:"dir"`    // "send" (client→server) or "recv" (server→client)
	Opcode     string    `json:"opcode"` // text, binary, close, ping, pong
	Len        int       `json:"len"`    // payload length after decompression
	Compressed bool      `json:"compressed,omitempty"`
	Truncated  bool      `json:"truncated,omitempty"`
	Data       []byte    `json:"-"`
	Note       string    `json:"note,omitempty"` // e.g. close code or decode error
}

// AddWSFrame appends a frame to an upgraded exchange and mirrors it to the
// JSONL log.
func (s *Store) AddWSFrame(ex *Exchange, f WSFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ex.WSFrameCount++
	if len(ex.WSFrames) < MaxWSFrames {
		ex.WSFrames = append(ex.WSFrames, f)
	}
	if s.jsonl != nil {
		_ = json.NewEncoder(s.jsonl).Encode(struct {
			Record     string `json:"record"`
			ExchangeID int64  `json:"exchange_id"`
			WSFrame
			Data *Body `json:"data,omitempty"`
		}{"ws_frame", ex.ID, f, EncodeBody(f.Data, nil)})
	}
}

// Store is an in-memory, concurrency-safe audit log, optionally mirrored to a
// JSONL file.
type Store struct {
	mu        sync.Mutex
	nextID    int64
	exchanges []*Exchange
	actions   map[string]*Action
	order     []string
	jsonl     io.Writer
	OnRecord  func(ex Exchange) // optional notification after a response completes
}

// NewStore returns an empty store. If jsonl is non-nil, each completed
// exchange and each action is appended to it as one JSON line.
func NewStore(jsonl io.Writer) *Store {
	return &Store{actions: map[string]*Action{}, jsonl: jsonl}
}

// OpenJSONL opens (append) a JSONL log file.
func OpenJSONL(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

// Begin allocates a new exchange for a request.
func (s *Store) Begin(ex *Exchange) *Exchange {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	ex.ID = s.nextID
	s.exchanges = append(s.exchanges, ex)
	return ex
}

// Update mutates an in-flight exchange under the store lock.
func (s *Store) Update(ex *Exchange, mut func(*Exchange)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mut(ex)
}

// Finish applies a final mutation, then writes the exchange to the JSONL log
// and notifies OnRecord.
func (s *Store) Finish(ex *Exchange, mut func(*Exchange)) {
	s.mu.Lock()
	if mut != nil {
		mut(ex)
	}
	cp := *ex
	if s.jsonl != nil {
		_ = json.NewEncoder(s.jsonl).Encode(struct {
			Record string `json:"record"`
			Exchange
			ReqBody  *Body `json:"request_body,omitempty"`
			RespBody *Body `json:"response_body,omitempty"`
		}{"exchange", cp, EncodeBody(cp.ReqBody, cp.ReqHeader), EncodeBody(cp.RespBody, cp.RespHeader)})
	}
	cb := s.OnRecord
	s.mu.Unlock()
	if cb != nil {
		cb(cp)
	}
}

// AddAction registers a DOM action.
func (s *Store) AddAction(a Action) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.actions[a.ID]; !ok {
		s.order = append(s.order, a.ID)
	}
	cp := a
	s.actions[a.ID] = &cp
	if s.jsonl != nil {
		_ = json.NewEncoder(s.jsonl).Encode(struct {
			Record string `json:"record"`
			Action
		}{"action", a})
	}
}

// Action looks up an action by ID.
func (s *Store) Action(id string) (Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.actions[id]
	if !ok {
		return Action{}, false
	}
	return *a, true
}

// Actions returns actions in registration order.
func (s *Store) Actions() []Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Action, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, *s.actions[id])
	}
	return out
}

// Exchanges returns a snapshot of all exchanges, optionally filtered.
func (s *Store) Exchanges(filter func(*Exchange) bool) []Exchange {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Exchange, 0, len(s.exchanges))
	for _, ex := range s.exchanges {
		if filter == nil || filter(ex) {
			cp := *ex
			cp.WSFrames = append([]WSFrame(nil), ex.WSFrames...)
			out = append(out, cp)
		}
	}
	return out
}

// Body is the JSON representation of a captured body.
type Body struct {
	Encoding string `json:"encoding"` // "utf8" or "base64"
	Data     string `json:"data"`
	Decoded  string `json:"content_decoded,omitempty"` // "gzip"/"deflate" if Data was decompressed for readability
}

// EncodeBody renders a body for logs, transparently decompressing
// gzip/deflate content so logs stay human readable.
func EncodeBody(b []byte, h http.Header) *Body {
	if len(b) == 0 {
		return nil
	}
	out := &Body{}
	if dec, enc, ok := Decompress(b, h.Get("Content-Encoding")); ok {
		b, out.Decoded = dec, enc
	}
	if utf8.Valid(b) {
		out.Encoding, out.Data = "utf8", string(b)
	} else {
		out.Encoding, out.Data = "base64", base64.StdEncoding.EncodeToString(b)
	}
	return out
}

// Decompress decodes gzip/deflate/br/zstd bodies (what Chrome accepts). ok is false when no decoding applied.
func Decompress(b []byte, contentEncoding string) ([]byte, string, bool) {
	var r io.ReadCloser
	var err error
	switch contentEncoding {
	case "gzip", "x-gzip":
		r, err = gzip.NewReader(bytes.NewReader(b))
	case "deflate":
		r = flate.NewReader(bytes.NewReader(b))
	case "br":
		r = io.NopCloser(brotli.NewReader(bytes.NewReader(b)))
	case "zstd":
		zr, zerr := zstd.NewReader(bytes.NewReader(b))
		if zerr != nil {
			return b, "", false
		}
		r = zr.IOReadCloser()
	default:
		return b, "", false
	}
	if err != nil {
		return b, "", false
	}
	defer r.Close()
	dec, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return b, "", false
	}
	return dec, contentEncoding, true
}
