package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsflate"
)

// Pages for the Phase 6 checks: iframes (same- and cross-origin), open
// shadow DOM, file upload, WebSocket and throttling/timing.

// framesPage embeds a same-origin iframe, a cross-origin iframe (other port =
// other origin: the parent cannot script it) and a web component whose
// controls live in an open shadow root.
const framesPage = `<!doctype html><html><head><meta charset="utf-8"><title>Frames</title></head>
<body style="margin:8px"><h1>Frames and shadow DOM</h1>
<iframe id="same" src="/frame-inner" style="width:420px;height:130px;border:2px solid #888"></iframe>
<iframe id="xo" src="%s/xo-inner" style="width:420px;height:130px;border:2px solid #888"></iframe>
<x-widget></x-widget>
<script>
customElements.define('x-widget', class extends HTMLElement {
  constructor() {
    super();
    const r = this.attachShadow({mode: 'open'});
    r.innerHTML = '<p><label for="shadow-in">Shadow input</label> <input id="shadow-in"> ' +
      '<button id="shadow-btn">Shadow Save</button></p><p id="out">none</p>';
    r.getElementById('shadow-btn').addEventListener('click', () => {
      r.getElementById('out').textContent = 'saved: ' + r.getElementById('shadow-in').value;
    });
  }
});
</script></body></html>`

// frameForm is served inside both iframes; which = "same" or "xo". Submitting
// posts "<which>:<name>" so the server sees what was typed in which frame.
const frameForm = `<!doctype html><html><head><meta charset="utf-8"><title>%[1]s frame</title></head>
<body style="margin:4px"><form id="f"><label for="name">%[2]s Name</label> <input id="name">
<button id="%[1]s-btn" type="submit">%[2]s Submit</button></form><p id="res">-</p>
<script>
document.getElementById('f').addEventListener('submit', async (e) => {
  e.preventDefault();
  const r = await fetch('/api/frame', {method: 'POST', body: '%[1]s:' + document.getElementById('name').value});
  document.getElementById('res').textContent = await r.text();
});
</script></body></html>`

// uploadPage: a hidden multiple file input behind a label (the usual styled
// upload button), a drop zone, and a multipart form submitted to /upload.
const uploadPage = `<!doctype html><html><head><meta charset="utf-8"><title>Upload</title></head>
<body><h1>Upload</h1>
<form id="up" method="post" action="/upload" enctype="multipart/form-data">
  <label for="file" style="border:1px solid #333;padding:4px">Choose files</label>
  <input id="file" name="file" type="file" multiple style="display:none">
  <button type="submit">Send files</button>
</form>
<p id="picked">none</p>
<div id="drop" style="width:300px;height:80px;border:2px dashed #555">Drop files here</div>
<p id="dropped">none</p>
<script>
document.getElementById('file').addEventListener('change', (e) => {
  document.getElementById('picked').textContent = Array.from(e.target.files).map(f => f.name + ':' + f.size + ':' + f.type).join(',');
});
const d = document.getElementById('drop');
d.addEventListener('dragover', (e) => e.preventDefault());
d.addEventListener('drop', async (e) => {
  e.preventDefault();
  const f = e.dataTransfer.files[0];
  document.getElementById('dropped').textContent = f.name + ':' + (await f.text()).trim();
});
</script></body></html>`

// wsPage talks to /ws (permessage-deflate negotiated by the server).
const wsPage = `<!doctype html><html><head><meta charset="utf-8"><title>WebSocket</title></head>
<body><p id="ws">connecting</p>
<script>
const s = new WebSocket('wss://' + location.host + '/ws');
const got = [];
s.onopen = () => s.send('hello-ws');
s.onmessage = (e) => {
  got.push(e.data);
  document.getElementById('ws').textContent = got.join(' | ');
  if (got.length === 1) s.send(JSON.stringify({n: 2, text: 'second message'}));
  else s.close(1000, 'bye');
};
</script></body></html>`

type uploadRecord struct {
	Name, Type string
	Size       int64
	SHA256     string
}

type phase6 struct {
	mu      sync.Mutex
	uploads []uploadRecord
}

func (p *phase6) lastUploads() []uploadRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]uploadRecord(nil), p.uploads...)
}

func (p *phase6) register(mux *http.ServeMux, xoURL func() string) {
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, body)
		}
	}
	mux.HandleFunc("/frames", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, framesPage, xoURL())
	})
	mux.HandleFunc("/frame-inner", page(fmt.Sprintf(frameForm, "same", "Frame")))
	mux.HandleFunc("/xo-inner", page(fmt.Sprintf(frameForm, "xo", "XO")))
	mux.HandleFunc("/api/frame", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "server got %s", b)
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			page(uploadPage)(w, r)
			return
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var recs []uploadRecord
		for _, fh := range r.MultipartForm.File["file"] {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			h := sha256.New()
			n, _ := io.Copy(h, f)
			f.Close()
			recs = append(recs, uploadRecord{fh.Filename, fh.Header.Get("Content-Type"), n, hex.EncodeToString(h.Sum(nil))})
		}
		p.mu.Lock()
		p.uploads = recs
		p.mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!doctype html><title>Uploaded</title><p>received %d file(s)</p>", len(recs))
	})
	mux.HandleFunc("/ws-page", page(wsPage))
	mux.HandleFunc("/ws", wsEchoDeflate)
	// /slow is used with 'rule add throttle': the origin itself is fast.
	mux.HandleFunc("/slow", page(`<!doctype html><title>Slow</title><p>`+strings.Repeat("payload ", 4000)+`</p>`))
}

// wsEchoDeflate answers each text message with "echo:<msg>", compressing
// replies with permessage-deflate when the browser offered it (Chrome does).
func wsEchoDeflate(w http.ResponseWriter, r *http.Request) {
	ext := &wsflate.Extension{Parameters: wsflate.DefaultParameters}
	u := ws.HTTPUpgrader{Negotiate: ext.Negotiate}
	conn, _, _, err := u.Upgrade(r, w)
	if err != nil {
		return
	}
	_, deflate := ext.Accepted()
	go func() {
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(time.Minute))
		for {
			f, err := ws.ReadFrame(conn)
			if err != nil {
				return
			}
			f = ws.UnmaskFrameInPlace(f)
			switch f.Header.OpCode {
			case ws.OpClose:
				ws.WriteFrame(conn, ws.NewCloseFrame(f.Payload))
				return
			case ws.OpText, ws.OpBinary:
				if ok, _ := wsflate.IsCompressed(f.Header); ok {
					if f, err = wsflate.DecompressFrame(f); err != nil {
						return
					}
				}
				reply := ws.NewTextFrame(append([]byte("echo:"), f.Payload...))
				if deflate {
					if reply, err = wsflate.CompressFrame(reply); err != nil {
						return
					}
				}
				if ws.WriteFrame(conn, reply) != nil {
					return
				}
			}
		}
	}()
}
