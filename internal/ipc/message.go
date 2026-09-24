package ipc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
)

// DefaultSocketPath is the Unix Domain Socket shared by app and nm-host.
// It can be overridden with the WEBCLI_SOCKET environment variable.
const DefaultSocketPath = "/tmp/my_browser_ipc.sock"

// Message types exchanged between the extension (via nm-host) and app.
const (
	TypePing             = "ping"         // either direction; answered with pong
	TypePong             = "pong"         // reply to ping
	TypeHello            = "hello"        // nm-host -> app, first line after connect
	TypeError            = "error"        // reply carrying Error
	TypeCollect          = "collect"      // app -> ext: collect visible hint targets
	TypeElements         = "elements"     // ext -> app: reply to collect
	TypeClick            = "click"        // app -> ext: click element by hint
	TypeClickResult      = "click_result" // ext -> app: reply to click
	TypeCookiesGet       = "cookies_get"  // app -> ext: dump cookies
	TypeCookies          = "cookies"      // ext -> app: reply to cookies_get
	TypeCookiesSet       = "cookies_set"  // app -> ext: import cookies
	TypeCookiesSetResult = "cookies_set_result"
	TypeLog              = "log" // ext -> app: diagnostic log line (no reply)

	// Page automation (app -> ext, each answered with "<type>_result").
	TypeDOM              = "dom" // DOM op in the active tab: click/type/select/check/press/submit/scroll/text/html/exists/info/storage_*
	TypeDOMResult        = "dom_result"
	TypeNavigate         = "navigate" // open/back/forward/reload, waits for the load
	TypeNavResult        = "nav_result"
	TypeTabs             = "tabs" // list/select/close tabs
	TypeTabsResult       = "tabs_result"
	TypeEval             = "eval" // JavaScript in the page main world (CSP fallback: chrome.debugger)
	TypeEvalResult       = "eval_result"
	TypeScreenshot       = "screenshot"
	TypeScreenshotResult = "screenshot_result"
	TypeRecord           = "record" // start/stop recording real user input
	TypeRecordResult     = "record_result"
	TypeRecordEvent      = "record_event" // ext -> app: one recorded user action (no reply)
	TypeUploadChunk      = "upload_chunk" // app -> ext: part of a file for 'upload' (<=512 KB base64)
	TypeUploadResult     = "upload_chunk_result"
	TypeTiming           = "timing" // navigation/paint/resource timing of the active tab
	TypeTimingResult     = "timing_result"
)

// Message is the envelope used on every hop (extension <-> nm-host <-> app).
// Requests carry a unique ID; replies echo the same ID.
type Message struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// NewID returns a random 128-bit hex identifier for request correlation.
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// NewMessage builds a message whose payload is the JSON encoding of v.
func NewMessage(id, typ string, v any) (Message, error) {
	m := Message{ID: id, Type: typ}
	if v != nil {
		raw, err := json.Marshal(v)
		if err != nil {
			return m, err
		}
		m.Payload = raw
	}
	return m, nil
}

// Hello is sent by nm-host right after connecting to app.
type Hello struct {
	Origin string `json:"origin"` // chrome-extension://<id>/ passed by Chrome as argv[1]
	PID    int    `json:"pid"`
}
