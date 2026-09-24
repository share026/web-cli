package main

// exthost runs the real extension/background.js and extension/content.js
// inside a real Blink engine (Chromium headless shell) and provides the
// chrome.* APIs they use on top of the Chrome DevTools Protocol:
//
//	chrome.runtime.connectNative   -> spawns bin/nm-host exactly like Chrome
//	                                  (argv[1] = origin, stdio length-prefixed JSON),
//	                                  after validating the installed host manifest
//	chrome.tabs.query/sendMessage  -> the page target / its isolated world
//	chrome.scripting.executeScript -> Page.createIsolatedWorld + Runtime.evaluate
//	chrome.declarativeNetRequest   -> Network.setExtraHTTPHeaders on the page target
//	chrome.cookies.getAll/set      -> Network.getCookies / Network.setCookie
//
// This is needed because the only Chromium build obtainable in the sandbox is
// the headless shell, which cannot load unpacked extensions. The JavaScript
// under test is byte-for-byte the shipped extension code.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/share026/web-cli/internal/ipc"
)

const (
	hostName  = "com.share026.webcli"
	pageTabID = 1
	worldName = "web-cli content script"
	bgURL     = "https://web-cli-extension.invalid/_generated_background_page.html"
)

type extHost struct {
	extDir      string
	extID       string
	userDataDir string
	socket      string
	logf        func(string, ...any)

	bgCtx, pageCtx context.Context
	bgMu, pageMu   sync.Mutex

	ctxMu     sync.Mutex
	contentID runtime.ExecutionContextID

	dnrMu sync.Mutex
	dnr   map[int64]json.RawMessage

	nativeMu    sync.Mutex
	nativeCmd   *exec.Cmd
	nativeStdin io.WriteCloser
	nativeDone  chan struct{}

	queue    chan string
	consoleC chan string
}

func (h *extHost) readExt(name string) string {
	b, err := os.ReadFile(filepath.Join(h.extDir, name))
	if err != nil {
		panic(err)
	}
	return string(b)
}

// bgShim defines the chrome.* surface background.js uses.
func (h *extHost) bgShim() string {
	manifest := h.readExt("manifest.json")
	return `(() => {
  const host = (o) => __webcliHost(JSON.stringify(o));
  const pending = new Map(); let seq = 0;
  const call = (api, ...args) => new Promise((resolve, reject) => {
    const id = ++seq; pending.set(id, {resolve, reject}); host({kind: "call", callId: id, api, args});
  });
  globalThis.__webcliResolve = (id, ok, value) => {
    const p = pending.get(id); pending.delete(id);
    if (!p) return;
    ok ? p.resolve(value) : p.reject(new Error(value));
  };
  const ev = () => { const ls = []; return { addListener: (f) => ls.push(f), _fire: (...a) => ls.forEach((f) => f(...a)) }; };
  let nativePort = null;
  globalThis.__webcliNative = {
    deliver: (m) => { if (nativePort) nativePort.onMessage._fire(m); },
    disconnect: (err) => {
      const p = nativePort; nativePort = null;
      if (!p) return;
      chrome.runtime.lastError = err ? { message: err } : undefined;
      p.onDisconnect._fire(p);
      chrome.runtime.lastError = undefined;
    },
  };
  const manifest = ` + manifest + `;
  globalThis.chrome = {
    runtime: {
      id: "` + h.extID + `",
      lastError: undefined,
      getManifest: () => manifest,
      onStartup: ev(), onInstalled: ev(),
      connectNative(name) {
        const port = { name, onMessage: ev(), onDisconnect: ev(),
          postMessage: (m) => host({ kind: "native_post", msg: m }),
          disconnect: () => host({ kind: "native_close" }) };
        nativePort = port;
        host({ kind: "native_connect", name });
        return port;
      },
    },
    tabs: {
      query: (q) => call("tabs.query", q),
      sendMessage: (tabId, msg) => call("tabs.sendMessage", tabId, msg),
    },
    scripting: { executeScript: (inj) => call("scripting.executeScript", inj) },
    declarativeNetRequest: { updateSessionRules: (o) => call("declarativeNetRequest.updateSessionRules", o) },
    cookies: { getAll: (d) => call("cookies.getAll", d), set: (d) => call("cookies.set", d) },
  };
})();`
}

// contentShim defines chrome.runtime.onMessage for content.js.
func (h *extHost) contentShim() string {
	return `(() => {
  if (globalThis.__webcliContentDispatch) return;
  const listeners = [];
  globalThis.chrome = { runtime: { id: "` + h.extID + `", onMessage: { addListener: (f) => listeners.push(f) } } };
  globalThis.__webcliContentDispatch = (msg) => new Promise((resolve) => {
    let done = false, async = false;
    const sendResponse = (r) => { if (!done) { done = true; resolve(r); } };
    for (const l of listeners) { if (l(msg, { id: chrome.runtime.id }, sendResponse) === true) async = true; }
    if (!done && !async) resolve(undefined);
  });
})();`
}

func (h *extHost) start(browserCtx context.Context) error {
	h.dnr = map[int64]json.RawMessage{}
	h.queue = make(chan string, 256)
	h.consoleC = make(chan string, 256)
	h.pageCtx = browserCtx
	h.bgCtx, _ = chromedp.NewContext(browserCtx) // second target hosts the service worker code

	chromedp.ListenTarget(h.bgCtx, func(ev any) {
		switch e := ev.(type) {
		case *runtime.EventBindingCalled:
			if e.Name == "__webcliHost" {
				h.queue <- e.Payload
			}
		case *runtime.EventConsoleAPICalled:
			var parts []string
			for _, a := range e.Args {
				var s string
				if json.Unmarshal(a.Value, &s) == nil {
					parts = append(parts, s)
				} else {
					parts = append(parts, string(a.Value))
				}
			}
			line := strings.Join(parts, " ")
			h.logf("[background.js console] %s", line)
			select {
			case h.consoleC <- line:
			default:
			}
		case *runtime.EventExceptionThrown:
			h.logf("[background.js exception] %s", e.ExceptionDetails.Error())
		case *fetch.EventRequestPaused:
			// Serve the background host document locally; it never touches the network/proxy.
			go chromedp.Run(h.bgCtx, fetch.FulfillRequest(e.RequestID, 200).
				WithResponseHeaders([]*fetch.HeaderEntry{{Name: "Content-Type", Value: "text/html"}}).
				WithBody("PCFkb2N0eXBlIGh0bWw+PHRpdGxlPmJhY2tncm91bmQ8L3RpdGxlPg==")) // "<!doctype html><title>background</title>"
		}
	})
	chromedp.ListenTarget(h.pageCtx, func(ev any) {
		switch e := ev.(type) {
		case *page.EventLoadEventFired:
			// manifest content_scripts run_at=document_idle
			go func() {
				if err := h.injectContent(); err != nil {
					h.logf("inject content.js: %v", err)
				}
			}()
		case *runtime.EventExecutionContextsCleared:
			h.ctxMu.Lock()
			h.contentID = 0
			h.ctxMu.Unlock()
		case *runtime.EventExecutionContextDestroyed:
			h.ctxMu.Lock()
			if h.contentID == e.ExecutionContextID {
				h.contentID = 0
			}
			h.ctxMu.Unlock()
		}
	})
	go h.worker()

	if err := chromedp.Run(h.pageCtx, network.Enable(), page.Enable(), runtime.Enable()); err != nil {
		return err
	}
	// Service-worker globals are secure-context only (crypto.randomUUID), so
	// the background code runs on an https:// document fulfilled via the
	// Fetch domain (standing in for the chrome-extension:// origin).
	var secure bool
	if err := chromedp.Run(h.bgCtx,
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: bgURL + "*"}}),
		chromedp.Navigate(bgURL),
		runtime.Enable(),
		runtime.AddBinding("__webcliHost"),
		chromedp.Evaluate(`isSecureContext && typeof crypto.randomUUID === "function"`, &secure),
	); err != nil {
		return err
	}
	if !secure {
		return errors.New("background context is not a secure context (crypto.randomUUID unavailable)")
	}
	h.bgMu.Lock()
	defer h.bgMu.Unlock()
	return chromedp.Run(h.bgCtx, chromedp.Evaluate(h.bgShim()+"\n"+h.readExt("background.js")+"\n;true", nil))
}

// worker processes binding calls in order (connect before post); API calls
// that may block run in their own goroutine.
func (h *extHost) worker() {
	for payload := range h.queue {
		var m struct {
			Kind   string            `json:"kind"`
			Name   string            `json:"name"`
			Msg    json.RawMessage   `json:"msg"`
			CallID int64             `json:"callId"`
			API    string            `json:"api"`
			Args   []json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			h.logf("bad binding payload: %v", err)
			continue
		}
		switch m.Kind {
		case "native_connect":
			if err := h.connectNative(m.Name); err != nil {
				h.logf("connectNative: %v", err)
				h.bgEval(fmt.Sprintf("__webcliNative.disconnect(%q)", err.Error()))
			}
		case "native_post":
			h.nativeMu.Lock()
			w := h.nativeStdin
			h.nativeMu.Unlock()
			if w != nil {
				if err := ipc.WriteNativeMessage(w, m.Msg); err != nil {
					h.logf("write to nm-host: %v", err)
				}
			}
		case "native_close":
			h.nativeMu.Lock()
			if h.nativeStdin != nil {
				h.nativeStdin.Close()
			}
			h.nativeMu.Unlock()
		case "call":
			go h.call(m.CallID, m.API, m.Args)
		}
	}
}

func (h *extHost) bgEval(js string) {
	h.bgMu.Lock()
	defer h.bgMu.Unlock()
	if err := chromedp.Run(h.bgCtx, chromedp.Evaluate(js+";true", nil)); err != nil && h.bgCtx.Err() == nil {
		h.logf("bg eval: %v", err)
	}
}

// connectNative follows Chrome's lookup: read the host manifest from
// <user-data-dir>/NativeMessagingHosts/<name>.json, check allowed_origins,
// then start "path" with the caller origin as the first argument.
func (h *extHost) connectNative(name string) error {
	mpath := filepath.Join(h.userDataDir, "NativeMessagingHosts", name+".json")
	raw, err := os.ReadFile(mpath)
	if err != nil {
		return fmt.Errorf("Specified native messaging host not found. (%v)", err)
	}
	var man struct {
		Name           string   `json:"name"`
		Path           string   `json:"path"`
		Type           string   `json:"type"`
		AllowedOrigins []string `json:"allowed_origins"`
	}
	if err := json.Unmarshal(raw, &man); err != nil {
		return err
	}
	origin := "chrome-extension://" + h.extID + "/"
	allowed := false
	for _, o := range man.AllowedOrigins {
		allowed = allowed || o == origin
	}
	if man.Name != name || man.Type != "stdio" || !allowed {
		return errors.New("Access to the specified native messaging host is forbidden.")
	}
	h.logf("native host manifest OK: %s -> %s (allowed_origins=%v)", mpath, man.Path, man.AllowedOrigins)

	cmd := exec.Command(man.Path, origin)
	cmd.Env = append(os.Environ(), "WEBCLI_SOCKET="+h.socket)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	h.nativeMu.Lock()
	h.nativeCmd, h.nativeStdin, h.nativeDone = cmd, stdin, done
	h.nativeMu.Unlock()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			h.logf("%s", sc.Text())
		}
	}()
	go func() {
		r := bufio.NewReader(stdout)
		for {
			msg, err := ipc.ReadNativeMessage(r)
			if err != nil {
				break
			}
			h.bgEval("__webcliNative.deliver(" + string(msg) + ")")
		}
		cmd.Wait()
		close(done)
		h.bgEval(`__webcliNative.disconnect("Native host has exited.")`)
	}()
	return nil
}

func (h *extHost) call(id int64, api string, args []json.RawMessage) {
	ctx, cancel := context.WithTimeout(h.pageCtx, 10*time.Second)
	defer cancel()
	res, err := h.dispatch(ctx, api, args)
	var js string
	if err != nil {
		js = fmt.Sprintf("__webcliResolve(%d,false,%s)", id, jsonString(err.Error()))
	} else {
		b, _ := json.Marshal(res)
		js = fmt.Sprintf("__webcliResolve(%d,true,%s)", id, b)
	}
	h.bgEval(js)
}

func jsonString(s string) string { b, _ := json.Marshal(s); return string(b) }

func (h *extHost) dispatch(ctx context.Context, api string, args []json.RawMessage) (any, error) {
	arg := func(i int, v any) error {
		if i >= len(args) {
			return fmt.Errorf("%s: missing argument %d", api, i)
		}
		return json.Unmarshal(args[i], v)
	}
	switch api {
	case "tabs.query":
		var url, title string
		h.pageMu.Lock()
		err := chromedp.Run(h.pageCtx, chromedp.Location(&url), chromedp.Title(&title))
		h.pageMu.Unlock()
		if err != nil {
			return nil, err
		}
		return []map[string]any{{"id": pageTabID, "url": url, "title": title, "active": true}}, nil

	case "tabs.sendMessage":
		var msg json.RawMessage
		if err := arg(1, &msg); err != nil {
			return nil, err
		}
		return h.sendToContent(ctx, msg)

	case "scripting.executeScript":
		if err := h.injectContent(); err != nil {
			return nil, err
		}
		return []map[string]any{{"frameId": 0, "result": nil}}, nil

	case "declarativeNetRequest.updateSessionRules":
		var o struct {
			RemoveRuleIDs []int64 `json:"removeRuleIds"`
			AddRules      []struct {
				ID     int64           `json:"id"`
				Action json.RawMessage `json:"action"`
				Raw    json.RawMessage `json:"-"`
			} `json:"addRules"`
		}
		var raw struct {
			AddRules []json.RawMessage `json:"addRules"`
		}
		if err := arg(0, &o); err != nil {
			return nil, err
		}
		arg(0, &raw)
		h.dnrMu.Lock()
		for _, id := range o.RemoveRuleIDs {
			delete(h.dnr, id)
		}
		for i, r := range o.AddRules {
			h.dnr[r.ID] = raw.AddRules[i]
		}
		headers := network.Headers{}
		for _, r := range h.dnr {
			var rule struct {
				Action struct {
					Type           string `json:"type"`
					RequestHeaders []struct {
						Header, Operation, Value string
					} `json:"requestHeaders"`
				} `json:"action"`
				Condition struct {
					TabIDs []int `json:"tabIds"`
				} `json:"condition"`
			}
			json.Unmarshal(r, &rule)
			applies := len(rule.Condition.TabIDs) == 0
			for _, t := range rule.Condition.TabIDs {
				applies = applies || t == pageTabID
			}
			if rule.Action.Type != "modifyHeaders" || !applies {
				continue
			}
			for _, rh := range rule.Action.RequestHeaders {
				if rh.Operation == "set" {
					headers[rh.Header] = rh.Value
				}
			}
		}
		h.dnrMu.Unlock()
		h.logf("declarativeNetRequest session rules -> extra headers %v", headers)
		h.pageMu.Lock()
		defer h.pageMu.Unlock()
		return nil, chromedp.Run(h.pageCtx, network.SetExtraHTTPHeaders(headers))

	case "cookies.getAll":
		var d struct {
			URL string `json:"url"`
		}
		if err := arg(0, &d); err != nil {
			return nil, err
		}
		var cookies []*network.Cookie
		h.pageMu.Lock()
		err := chromedp.Run(h.pageCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().WithURLs([]string{d.URL}).Do(ctx)
			return err
		}))
		h.pageMu.Unlock()
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(cookies))
		for _, c := range cookies {
			m := map[string]any{
				"name": c.Name, "value": c.Value, "domain": c.Domain, "path": c.Path,
				"secure": c.Secure, "httpOnly": c.HTTPOnly, "session": c.Session,
				"hostOnly": !strings.HasPrefix(c.Domain, "."), "storeId": "0",
				"sameSite": map[network.CookieSameSite]string{
					network.CookieSameSiteLax: "lax", network.CookieSameSiteStrict: "strict", network.CookieSameSiteNone: "no_restriction",
				}[c.SameSite],
			}
			if m["sameSite"] == "" {
				m["sameSite"] = "unspecified"
			}
			if !c.Session {
				m["expirationDate"] = c.Expires
			}
			out = append(out, m)
		}
		return out, nil

	case "cookies.set":
		var d struct {
			URL            string   `json:"url"`
			Name           string   `json:"name"`
			Value          string   `json:"value"`
			Domain         string   `json:"domain"`
			Path           string   `json:"path"`
			Secure         bool     `json:"secure"`
			HTTPOnly       bool     `json:"httpOnly"`
			SameSite       string   `json:"sameSite"`
			ExpirationDate *float64 `json:"expirationDate"`
		}
		if err := arg(0, &d); err != nil {
			return nil, err
		}
		p := network.SetCookie(d.Name, d.Value).WithURL(d.URL).WithSecure(d.Secure).WithHTTPOnly(d.HTTPOnly)
		if d.Domain != "" {
			p = p.WithDomain(d.Domain)
		}
		if d.Path != "" {
			p = p.WithPath(d.Path)
		}
		switch d.SameSite {
		case "lax":
			p = p.WithSameSite(network.CookieSameSiteLax)
		case "strict":
			p = p.WithSameSite(network.CookieSameSiteStrict)
		case "no_restriction":
			p = p.WithSameSite(network.CookieSameSiteNone)
		}
		if d.ExpirationDate != nil {
			t := cdp.TimeSinceEpoch(time.Unix(int64(*d.ExpirationDate), 0))
			p = p.WithExpires(&t)
		}
		h.pageMu.Lock()
		err := chromedp.Run(h.pageCtx, chromedp.ActionFunc(func(ctx context.Context) error { return p.Do(ctx) }))
		h.pageMu.Unlock()
		if err != nil {
			return nil, err // chrome.cookies.set rejects too
		}
		return map[string]any{"name": d.Name, "value": d.Value, "domain": d.Domain, "path": d.Path}, nil
	}
	return nil, fmt.Errorf("unsupported API %s", api)
}

// injectContent creates the content-script isolated world (once per document)
// and evaluates the shim + content.js in it.
func (h *extHost) injectContent() error {
	h.pageMu.Lock()
	defer h.pageMu.Unlock()
	h.ctxMu.Lock()
	existing := h.contentID
	h.ctxMu.Unlock()
	src := h.contentShim() + "\n" + h.readExt("content.js") + "\n;true"
	return chromedp.Run(h.pageCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		id := existing
		if id == 0 {
			tree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			id, err = page.CreateIsolatedWorld(tree.Frame.ID).WithWorldName(worldName).Do(ctx)
			if err != nil {
				return err
			}
		}
		_, exc, err := runtime.Evaluate(src).WithContextID(id).Do(ctx)
		if err != nil {
			return err
		}
		if exc != nil {
			return fmt.Errorf("content.js threw: %s", exc.Error())
		}
		h.ctxMu.Lock()
		h.contentID = id
		h.ctxMu.Unlock()
		h.logf("content.js injected into isolated world (context %d)", id)
		return nil
	}))
}

func (h *extHost) sendToContent(ctx context.Context, msg json.RawMessage) (any, error) {
	h.ctxMu.Lock()
	id := h.contentID
	h.ctxMu.Unlock()
	if id == 0 {
		return nil, errors.New("Could not establish connection. Receiving end does not exist.")
	}
	var out json.RawMessage
	h.pageMu.Lock()
	err := chromedp.Run(h.pageCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		ro, exc, err := runtime.Evaluate("__webcliContentDispatch(" + string(msg) + ")").
			WithContextID(id).WithAwaitPromise(true).WithReturnByValue(true).Do(ctx)
		if err != nil {
			return err
		}
		if exc != nil {
			return errors.New(exc.Error())
		}
		out = ro.Value
		return nil
	}))
	h.pageMu.Unlock()
	if err != nil {
		if strings.Contains(err.Error(), "context") {
			return nil, errors.New("Could not establish connection. Receiving end does not exist.")
		}
		return nil, err
	}
	return out, nil
}

func (h *extHost) stop() {
	h.nativeMu.Lock()
	defer h.nativeMu.Unlock()
	if h.nativeStdin != nil {
		h.nativeStdin.Close()
	}
	if h.nativeCmd != nil && h.nativeCmd.Process != nil {
		select {
		case <-h.nativeDone:
		case <-time.After(2 * time.Second):
			h.nativeCmd.Process.Kill()
		}
	}
}
