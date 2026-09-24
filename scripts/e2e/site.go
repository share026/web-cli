package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
)

// dummyPage exercises every branch of the Vimium C style visibility filter.
const dummyPage = `<!doctype html>
<html><head><meta charset="utf-8"><title>web-cli E2E dummy</title>
<style>
  body { font: 14px sans-serif; margin: 0; padding: 10px; }
  .row { margin: 6px 0; }
</style></head>
<body>
  <h1>Dummy app</h1>
  <!-- visible targets -->
  <div class="row"><a id="next" href="/next">Next page</a></div>
  <div class="row"><button id="login" onclick="login()">Login</button>
       <button id="whoami" onclick="whoami()">Who am I</button></div>
  <div class="row"><input id="q" placeholder="Search products"></div>
  <div class="row"><select name="lang"><option>English</option><option>日本語</option></select></div>
  <div class="row"><div role="button" id="rolebtn" style="display:inline-block">Role Button</div>
       <span id="span" onclick="document.getElementById('status').textContent='span clicked'">Onclick span</span></div>
  <p id="status">idle</p>
  <p id="status2"></p>

  <!-- hidden: each must be rejected by content.js -->
  <button style="display:none">Hidden display</button>
  <button style="visibility:hidden">Hidden visibility</button>
  <button style="opacity:0">Hidden opacity</button>
  <div style="opacity:0"><a href="#x">Hidden parent opacity</a></div>
  <button style="width:0;height:0;padding:0;border:0;overflow:hidden">Zero size</button>
  <button style="position:absolute;top:-500px;left:10px">Offscreen</button>
  <button disabled>Disabled</button>
  <input type="hidden" name="csrf" value="t">
  <button style="position:absolute;left:420px;top:120px">Covered button</button>
  <div style="position:absolute;left:400px;top:100px;width:220px;height:80px;background:#eee;z-index:10">modal overlay</div>
  <div style="height:3000px"></div>
  <button>Below fold</button>

<script>
async function login() {
  const r = await fetch('/api/login', {method: 'POST', headers: {'Content-Type': 'application/json'},
                                       body: JSON.stringify({user: 'alice', password: 'wonderland'})});
  const j = await r.json();
  document.getElementById('status').textContent = 'logged in: ' + j.user;
}
async function whoami() {
  const r = await fetch('/api/whoami');
  const j = await r.json();
  document.getElementById('status').textContent = 'cookies: ' + j.cookies.join(',');
  const b = await fetch('/blocked');
  document.getElementById('status2').textContent = 'blocked status: ' + b.status;
}
</script>
</body></html>`

const nextPage = `<!doctype html><html><head><title>Next</title></head>
<body><h1>Next page</h1><a href="/">Back home</a><button onclick="this.textContent='clicked'">Press me</button></body></html>`

// loginPage is an ordinary HTML form login (labels, select, checkbox,
// required fields, submit by Enter or button).
const loginPage = `<!doctype html><html><head><meta charset="utf-8"><title>Login</title></head>
<body><h1>Sign in</h1>
<form id="loginform" method="post" action="/session">
  <p><label for="email">Email</label> <input id="email" name="email" type="email" required></p>
  <p><label for="password">Password</label> <input id="password" name="password" type="password" required></p>
  <p><label for="country">Country</label> <select id="country" name="country">
       <option value="us">United States</option><option value="jp">Japan</option></select></p>
  <p><label><input id="remember" type="checkbox" name="remember"> Remember me</label></p>
  <button type="submit">Sign in</button>
</form></body></html>`

// dashboardPage renders part of its content and web storage with JavaScript,
// so 'source' (rendered DOM) and 'body' (served HTML) differ.
const dashboardPage = `<!doctype html><html><head><meta charset="utf-8"><title>Dashboard</title></head>
<body><h1 id="dashboard">Welcome %s</h1><p id="js">static</p>
<script>
  document.getElementById('js').textContent = 'rendered by js';
  localStorage.setItem('theme', 'dark');
  sessionStorage.setItem('tab', 'home');
</script></body></html>`

// seenRequest is what the upstream (origin) server actually received.
type seenRequest struct {
	Method string      `json:"method"`
	Path   string      `json:"path"`
	Header http.Header `json:"header"`
	Body   string      `json:"body,omitempty"`
}

type site struct {
	*httptest.Server
	mu   sync.Mutex
	seen []seenRequest
}

func newSite() *site {
	s := &site{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, dummyPage)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, nextPage)
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, loginPage)
	})
	mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.ParseForm() != nil || r.PostForm.Get("email") == "" {
			http.Error(w, "bad login", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "auth", Value: "ok-" + r.PostForm.Get("country"), Path: "/", HttpOnly: true, Secure: true})
		http.Redirect(w, r, "/dashboard?user="+url.QueryEscape(r.PostForm.Get("email")), http.StatusSeeOther)
	})
	mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, dashboardPage, html.EscapeString(r.URL.Query().Get("user")))
	})
	mux.HandleFunc("/csp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "script-src 'self'")
		fmt.Fprint(w, `<!doctype html><html><head><title>CSP</title></head><body><p>strict CSP: no eval</p></body></html>`)
	})
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ User string }
		json.NewDecoder(r.Body).Decode(&in)
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "user": in.User})
	})
	mux.HandleFunc("/api/whoami", func(w http.ResponseWriter, r *http.Request) {
		var names []string
		for _, c := range r.Cookies() {
			names = append(names, c.Name+"="+c.Value)
		}
		sort.Strings(names)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"cookies": names, "injected": r.Header.Get("X-Injected")})
	})
	mux.HandleFunc("/blocked", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "this must never be served: the proxy blocks it")
	})
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// record the body without consuming it for the handler
		var body strings.Builder
		if r.Body != nil {
			buf := make([]byte, 4096)
			n, _ := r.Body.Read(buf)
			body.Write(buf[:n])
			r.Body = readCloser{strings.NewReader(body.String())}
		}
		s.mu.Lock()
		s.seen = append(s.seen, seenRequest{Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: body.String()})
		s.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	return s
}

type readCloser struct{ *strings.Reader }

func (readCloser) Close() error { return nil }

func (s *site) requests(path string) []seenRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []seenRequest
	for _, r := range s.seen {
		if r.Path == path {
			out = append(out, r)
		}
	}
	return out
}
