package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/share026/web-cli/internal/audit"
	"github.com/share026/web-cli/internal/ipc"
)

// defaultWait is how long element targets are retried (the page may still be
// loading or navigating) before a command fails.
const defaultWait = 10 * time.Second

// DOMResult is the extension's reply to a "dom" request.
type DOMResult struct {
	Op       string            `json:"op"`
	ActionID string            `json:"action_id"`
	Kind     string            `json:"kind"`
	Tag      string            `json:"tag"`
	Type     string            `json:"type"`
	Text     string            `json:"text"`
	Selector string            `json:"selector"`
	URL      string            `json:"url"`
	Title    string            `json:"title"`
	Value    string            `json:"value"`
	Key      string            `json:"key"`
	Length   int               `json:"length"`
	Checked  *bool             `json:"checked"`
	Found    bool              `json:"found"`
	HTML     string            `json:"html"`
	Origin   string            `json:"origin"`
	Local    map[string]string `json:"local"`
	Session  map[string]string `json:"session"`
	Set      int               `json:"set"`
	ScrollY  float64           `json:"scroll_y"`
}

func (r DOMResult) element() string {
	k := r.Tag
	if r.Type != "" {
		k += ":" + r.Type
	}
	return fmt.Sprintf("<%s> %q", k, r.Text)
}

// browserCommands are handled by execBrowser (quoted arguments, $VAR).
var browserCommands = map[string]bool{
	"list": true, "ls": true, "click": true, "type": true, "fill": true, "clear": true,
	"select": true, "check": true, "uncheck": true, "press": true, "submit": true,
	"focus": true, "scroll": true, "text": true, "html": true, "source": true,
	"waitfor": true, "sleep": true, "timeout": true, "open": true, "goto": true,
	"newtab": true, "back": true, "forward": true, "reload": true, "tabs": true,
	"tab": true, "closetab": true, "url": true, "eval": true, "js": true,
	"screenshot": true, "storage": true, "show": true, "body": true,
	"record": true, "run": true,
}

// recordable commands are written to an active recording after they succeed.
var recordable = map[string]bool{
	"click": true, "type": true, "fill": true, "clear": true, "select": true, "check": true,
	"uncheck": true, "press": true, "submit": true, "focus": true, "scroll": true,
	"waitfor": true, "sleep": true, "open": true, "goto": true, "newtab": true,
	"back": true, "forward": true, "reload": true, "eval": true, "js": true,
}

func (a *App) execBrowser(line string) error {
	cmd := strings.Fields(line)[0]
	var toks []token
	var err error
	if cmd == "eval" || cmd == "js" {
		toks = []token{{Value: cmd, Raw: cmd}} // the rest of the line is JavaScript, taken verbatim
	} else if toks, err = splitArgs(line, nil); err != nil {
		return err
	}
	args := make([]string, len(toks))
	for i, t := range toks {
		args[i] = t.Value
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout+a.waitTimeout())
	defer cancel()
	rec := &recordLine{toks: toks}
	if err := a.browserCmd(ctx, line, args, rec); err != nil {
		return err
	}
	if recordable[cmd] && a.rec.active() {
		a.rec.writeCommand(rec.render())
	}
	return nil
}

// recordLine is the raw command as typed, with hint numbers replaced by the
// stable selector the extension reported (hints are only valid until the
// page changes).
type recordLine struct {
	toks []token
}

func (r *recordLine) replaceTarget(i int, selector string) {
	if i < len(r.toks) && selector != "" && isHint(r.toks[i].Value) {
		q := quoteArg("css=" + selector)
		r.toks[i] = token{Value: "css=" + selector, Raw: q}
	}
}

func (r *recordLine) render() string {
	parts := make([]string, len(r.toks))
	for i, t := range r.toks {
		parts[i] = t.Raw
	}
	return strings.Join(parts, " ")
}

func isHint(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil && s != ""
}

func (a *App) waitTimeout() time.Duration {
	if a.wait > 0 {
		return a.wait
	}
	return defaultWait
}

func (a *App) browserCmd(ctx context.Context, line string, args []string, rec *recordLine) error {
	usage := func(u string) error { return fmt.Errorf("usage: %s", u) }
	switch args[0] {
	case "list", "ls":
		scope, query := "viewport", ""
		rest := args[1:]
		if len(rest) > 0 && rest[0] == "all" {
			scope, rest = "page", rest[1:]
		}
		query = strings.Join(rest, " ")
		return a.cmdList(ctx, scope, query)

	case "click":
		if len(args) != 2 {
			return usage("click <hint|css=selector|text>")
		}
		if isHint(args[1]) && !a.rec.active() {
			hint, _ := strconv.Atoi(args[1])
			return a.click(ctx, hint) // same path as 'hints'
		}
		r, err := a.domOnTarget(ctx, "click", args[1], nil)
		if err != nil {
			return err
		}
		rec.replaceTarget(1, r.Selector)
		fmt.Printf("%s %s selector=%s action_id=%s\n", r.Kind, r.element(), r.Selector, r.ActionID)

	case "focus":
		if len(args) != 2 {
			return usage("focus <target>")
		}
		r, err := a.domOnTarget(ctx, "focus", args[1], nil)
		if err != nil {
			return err
		}
		rec.replaceTarget(1, r.Selector)
		fmt.Printf("focused %s selector=%s\n", r.element(), r.Selector)

	case "type", "fill":
		if len(args) < 3 {
			return usage(`type <target> <text|"text with spaces"|$VAR|->  (- prompts without echo)`)
		}
		text := strings.Join(args[2:], " ")
		if len(args) == 3 && args[2] == "-" {
			s, err := readSecret(fmt.Sprintf("text for %s (not echoed): ", args[1]))
			if err != nil {
				return err
			}
			text = s
		}
		r, err := a.domOnTarget(ctx, "type", args[1], map[string]any{"text": text})
		if err != nil {
			return err
		}
		rec.replaceTarget(1, r.Selector)
		// never print the value: it may be a password
		fmt.Printf("typed %d char(s) into %s selector=%s action_id=%s\n", r.Length, r.element(), r.Selector, r.ActionID)

	case "clear":
		if len(args) != 2 {
			return usage("clear <target>")
		}
		r, err := a.domOnTarget(ctx, "clear", args[1], nil)
		if err != nil {
			return err
		}
		rec.replaceTarget(1, r.Selector)
		fmt.Printf("cleared %s selector=%s\n", r.element(), r.Selector)

	case "select":
		if len(args) < 3 {
			return usage("select <target> <option text|value>")
		}
		r, err := a.domOnTarget(ctx, "select", args[1], map[string]any{"value": strings.Join(args[2:], " ")})
		if err != nil {
			return err
		}
		rec.replaceTarget(1, r.Selector)
		fmt.Printf("selected %q in %s selector=%s action_id=%s\n", r.Value, r.element(), r.Selector, r.ActionID)

	case "check", "uncheck":
		if len(args) != 2 {
			return usage(args[0] + " <target>")
		}
		r, err := a.domOnTarget(ctx, "check", args[1], map[string]any{"on": args[0] == "check"})
		if err != nil {
			return err
		}
		rec.replaceTarget(1, r.Selector)
		fmt.Printf("%sed %s checked=%v selector=%s action_id=%s\n", args[0], r.element(), r.Checked != nil && *r.Checked, r.Selector, r.ActionID)

	case "press":
		if len(args) < 2 || len(args) > 3 {
			return usage("press <Enter|Tab|Escape|Space|Backspace|ArrowDown|...|a> [target]")
		}
		var r DOMResult
		var err error
		if len(args) == 3 {
			r, err = a.domOnTarget(ctx, "press", args[2], map[string]any{"key": args[1]})
			rec.replaceTarget(2, r.Selector)
		} else {
			r, err = a.dom(ctx, "press", nil, map[string]any{"key": args[1]})
		}
		if err != nil {
			return err
		}
		fmt.Printf("pressed %s on %s action_id=%s\n", r.Key, r.element(), r.ActionID)

	case "submit":
		var r DOMResult
		var err error
		if len(args) == 2 {
			r, err = a.domOnTarget(ctx, "submit", args[1], nil)
			rec.replaceTarget(1, r.Selector)
		} else {
			r, err = a.dom(ctx, "submit", nil, nil)
		}
		if err != nil {
			return err
		}
		fmt.Printf("submitted form %s action_id=%s\n", r.Selector, r.ActionID)

	case "scroll":
		if len(args) != 2 {
			return usage("scroll <down|up|top|bottom|target>")
		}
		switch args[1] {
		case "down", "up", "top", "bottom":
			r, err := a.dom(ctx, "scroll", nil, map[string]any{"to": args[1]})
			if err != nil {
				return err
			}
			fmt.Printf("scrolled %s (scrollY=%.0f)\n", args[1], r.ScrollY)
		default:
			r, err := a.domOnTarget(ctx, "scroll", args[1], nil)
			if err != nil {
				return err
			}
			rec.replaceTarget(1, r.Selector)
			fmt.Printf("scrolled to %s\n", r.element())
		}

	case "text":
		var r DOMResult
		var err error
		if len(args) >= 2 {
			r, err = a.domOnTarget(ctx, "text", args[1], nil)
		} else {
			r, err = a.dom(ctx, "text", nil, nil)
		}
		if err != nil {
			return err
		}
		fmt.Printf("--- text of %s (%s)\n%s\n--- end of text\n", r.URL, r.Title, strings.TrimRight(r.Text, "\n"))
		if len(args) == 3 {
			return writeFile(args[2], []byte(r.Text), "text")
		}

	case "html":
		if len(args) < 2 {
			return usage("html <target> [file]")
		}
		r, err := a.domOnTarget(ctx, "html", args[1], nil)
		if err != nil {
			return err
		}
		if len(args) == 3 {
			return writeFile(args[2], []byte(r.HTML), "outerHTML")
		}
		fmt.Println(r.HTML)

	case "source":
		r, err := a.dom(ctx, "html", nil, nil)
		if err != nil {
			return err
		}
		file := ""
		if len(args) >= 2 {
			file = args[1]
		} else {
			file = filepath.Join(a.cfg.outDir, "source-"+hostOf(r.URL)+"-"+time.Now().Format("150405")+".html")
		}
		if err := os.WriteFile(file, []byte(r.HTML), 0o600); err != nil {
			return err
		}
		fmt.Printf("saved rendered DOM of %s (%d bytes) to %s\n", r.URL, len(r.HTML), file)
		fmt.Println("  (the HTML as served by the server: 'log' to find the request, then 'body <id> <file>')")

	case "waitfor":
		if len(args) < 2 || len(args) > 3 {
			return usage("waitfor <target|text=...|url=regexp|title=...> [timeout]")
		}
		d := a.waitTimeout()
		if len(args) == 3 {
			var err error
			if d, err = time.ParseDuration(args[2]); err != nil {
				return err
			}
		}
		return a.cmdWaitFor(args[1], d)

	case "sleep":
		if len(args) != 2 {
			return usage("sleep <duration>")
		}
		d, err := time.ParseDuration(args[1])
		if err != nil {
			return err
		}
		time.Sleep(d)

	case "timeout":
		if len(args) != 2 {
			fmt.Printf("element wait timeout: %s\n", a.waitTimeout())
			return nil
		}
		d, err := time.ParseDuration(args[1])
		if err != nil {
			return err
		}
		a.wait = d
		fmt.Printf("element wait timeout: %s\n", d)

	case "open", "goto", "newtab":
		if len(args) != 2 {
			return usage(args[0] + " <url>")
		}
		u := args[1]
		if !strings.Contains(u, "://") && !strings.HasPrefix(u, "about:") {
			u = "https://" + u
		}
		return a.navigate(ctx, "open", u, args[0] == "newtab")

	case "back", "forward", "reload":
		return a.navigate(ctx, args[0], "", false)

	case "url":
		r, err := a.dom(ctx, "info", nil, nil)
		if err != nil {
			return err
		}
		fmt.Printf("%s  %q\n", r.URL, r.Title)

	case "tabs":
		return a.cmdTabs(ctx, "list", 0)
	case "tab":
		if len(args) != 2 {
			return usage("tab <id>")
		}
		id, err := strconv.Atoi(args[1])
		if err != nil {
			return err
		}
		return a.cmdTabs(ctx, "select", id)
	case "closetab":
		id := 0
		if len(args) == 2 {
			var err error
			if id, err = strconv.Atoi(args[1]); err != nil {
				return err
			}
		}
		return a.cmdTabs(ctx, "close", id)

	case "eval", "js":
		code := restAfter(line, 1)
		if code == "" {
			return usage("eval <javascript>")
		}
		r, err := a.hub.Request(ctx, ipc.TypeEval, map[string]any{"code": code})
		if err != nil {
			return err
		}
		var res struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
			Via   string          `json:"via"`
		}
		if err := json.Unmarshal(r.Payload, &res); err != nil {
			return err
		}
		v := string(res.Value)
		if v == "" {
			v = res.Type
		}
		fmt.Printf("=> %s  (%s, via %s)\n", v, res.Type, res.Via)

	case "screenshot":
		r, err := a.hub.Request(ctx, ipc.TypeScreenshot, nil)
		if err != nil {
			return err
		}
		var res struct {
			URL string `json:"url"`
			PNG string `json:"png_base64"`
		}
		if err := json.Unmarshal(r.Payload, &res); err != nil {
			return err
		}
		png, err := base64.StdEncoding.DecodeString(res.PNG)
		if err != nil {
			return err
		}
		file := filepath.Join(a.cfg.outDir, "screenshot-"+time.Now().Format("150405")+".png")
		if len(args) == 2 {
			file = args[1]
		}
		if err := os.WriteFile(file, png, 0o600); err != nil {
			return err
		}
		fmt.Printf("saved screenshot of %s (%d bytes) to %s\n", res.URL, len(png), file)

	case "storage":
		return a.cmdStorage(ctx, args[1:])

	case "show":
		if len(args) != 2 {
			return usage("show <request id>")
		}
		return a.cmdShow(args[1])

	case "body":
		if len(args) < 2 || len(args) > 3 {
			return usage("body <request id> [file]")
		}
		return a.cmdBody(args[1:])

	case "record":
		return a.cmdRecord(ctx, args[1:])

	case "run":
		if len(args) != 2 {
			return usage("run <script file>")
		}
		return a.runScript(args[1])
	}
	return nil
}

// dom sends one DOM operation and records page-changing ones as actions.
func (a *App) dom(ctx context.Context, op string, target map[string]any, extra map[string]any) (DOMResult, error) {
	p := map[string]any{"op": op, "tag": a.proxyAddr != ""}
	if target != nil {
		p["target"] = target
	}
	for k, v := range extra {
		p[k] = v
	}
	r, err := a.hub.Request(ctx, ipc.TypeDOM, p)
	if err != nil {
		return DOMResult{}, err
	}
	var res DOMResult
	if err := json.Unmarshal(r.Payload, &res); err != nil {
		return DOMResult{}, fmt.Errorf("decode dom result: %w", err)
	}
	if res.ActionID != "" {
		hint := 0
		if target != nil {
			if h, ok := target["hint"].(int); ok {
				hint = h
			}
		}
		text := res.Text
		if res.Value != "" && op == "select" {
			text += " = " + res.Value
		}
		if op == "press" {
			text = res.Key + " on " + text
		}
		a.store.AddAction(audit.Action{
			ID: res.ActionID, Time: time.Now(), Kind: op, Hint: hint,
			Tag: res.Tag, Text: text, PageURL: res.URL,
		})
	}
	return res, nil
}

// domOnTarget resolves a target and runs op on it, retrying while the
// element does not exist yet (page loading, navigation in progress).
//
// Targets: a hint number from the last 'list'/'hints', css=<selector>, or
// any other text, matched with fzf --filter against the page's controls
// (labels, placeholders, button texts, ...).
func (a *App) domOnTarget(ctx context.Context, op, spec string, extra map[string]any) (DOMResult, error) {
	deadline := time.Now().Add(a.waitTimeout())
	var lastErr error
	for {
		target, err := a.resolveTarget(ctx, spec)
		if err == nil {
			var r DOMResult
			r, err = a.dom(ctx, op, target, extra)
			if err == nil {
				return r, nil
			}
		}
		lastErr = err
		if !retryable(err) || time.Now().After(deadline) || isHint(spec) {
			return DOMResult{}, lastErr
		}
		select {
		case <-ctx.Done():
			return DOMResult{}, lastErr
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, k := range []string{"not found", "no element matches", "no visible elements", "Receiving end does not exist",
		"Could not establish connection", "no active tab", "message channel closed", "context invalidated",
		"stale", "Frame with ID", "No tab with id", "Cannot access contents"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func (a *App) resolveTarget(ctx context.Context, spec string) (map[string]any, error) {
	switch {
	case isHint(spec):
		h, _ := strconv.Atoi(spec)
		return map[string]any{"hint": h}, nil
	case strings.HasPrefix(spec, "css="):
		return map[string]any{"css": strings.TrimPrefix(spec, "css=")}, nil
	}
	query := strings.TrimPrefix(spec, "text=")
	r, err := a.hub.Request(ctx, ipc.TypeCollect, map[string]any{"scope": "page"})
	if err != nil {
		return nil, err
	}
	var p ElementsPayload
	if err := json.Unmarshal(r.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode elements: %w", err)
	}
	hint, err := selectWithFzf(a.cfg.fzf, p.Elements, query, "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"hint": hint}, nil
}

func (a *App) cmdList(ctx context.Context, scope, query string) error {
	r, err := a.hub.Request(ctx, ipc.TypeCollect, map[string]any{"scope": scope})
	if err != nil {
		return err
	}
	var p ElementsPayload
	if err := json.Unmarshal(r.Payload, &p); err != nil {
		return fmt.Errorf("decode elements: %w", err)
	}
	els := p.Elements
	if query != "" {
		ranked, err := filterWithFzf(a.cfg.fzf, els, query)
		if err != nil {
			return err
		}
		els = ranked
	}
	fmt.Printf("%d element(s) on %s  %q (scope %s)\n", len(els), p.URL, p.Title, scope)
	for _, e := range els {
		fmt.Printf("  %s\n", strings.ReplaceAll(e.fzfLine(), "\t", "  "))
	}
	return nil
}

func (a *App) cmdWaitFor(spec string, d time.Duration) error {
	start := time.Now()
	deadline := start.Add(d)
	var check func(ctx context.Context) (bool, string, error)
	switch {
	case strings.HasPrefix(spec, "text="):
		want := strings.TrimPrefix(spec, "text=")
		check = func(ctx context.Context) (bool, string, error) {
			r, err := a.dom(ctx, "exists", nil, map[string]any{"text": want})
			return r.Found, r.URL, err
		}
	case strings.HasPrefix(spec, "url="):
		re, err := regexp.Compile(strings.TrimPrefix(spec, "url="))
		if err != nil {
			return err
		}
		check = func(ctx context.Context) (bool, string, error) {
			r, err := a.dom(ctx, "info", nil, nil)
			return err == nil && re.MatchString(r.URL), r.URL, err
		}
	case strings.HasPrefix(spec, "title="):
		want := strings.TrimPrefix(spec, "title=")
		check = func(ctx context.Context) (bool, string, error) {
			r, err := a.dom(ctx, "info", nil, nil)
			return err == nil && strings.Contains(r.Title, want), r.URL, err
		}
	case strings.HasPrefix(spec, "css="):
		check = func(ctx context.Context) (bool, string, error) {
			r, err := a.dom(ctx, "exists", map[string]any{"css": strings.TrimPrefix(spec, "css=")}, nil)
			return r.Found, r.URL, err
		}
	default:
		check = func(ctx context.Context) (bool, string, error) {
			_, err := a.resolveTarget(ctx, spec)
			return err == nil, "", err
		}
	}
	var lastErr error
	for {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		ok, u, err := check(ctx)
		cancel()
		if ok {
			fmt.Printf("found %s after %s %s\n", spec, time.Since(start).Round(time.Millisecond), u)
			return nil
		}
		if err != nil {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil && !retryable(lastErr) {
				return fmt.Errorf("waitfor %s: %w", spec, lastErr)
			}
			return fmt.Errorf("waitfor %s: timed out after %s", spec, d)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (a *App) navigate(ctx context.Context, action, u string, newTab bool) error {
	r, err := a.hub.Request(ctx, ipc.TypeNavigate, map[string]any{
		"action": action, "url": u, "new_tab": newTab, "tag": a.proxyAddr != "",
	})
	if err != nil {
		return err
	}
	var res struct {
		ActionID string `json:"action_id"`
		TabID    int    `json:"tab_id"`
		URL      string `json:"url"`
		Title    string `json:"title"`
	}
	if err := json.Unmarshal(r.Payload, &res); err != nil {
		return err
	}
	a.store.AddAction(audit.Action{ID: res.ActionID, Time: time.Now(), Kind: "navigate:" + action, Text: u, PageURL: res.URL})
	verb := map[string]string{"open": "opened", "back": "back", "forward": "forward", "reload": "reloaded"}[action]
	fmt.Printf("%s %s %q tab=%d action_id=%s\n", verb, res.URL, res.Title, res.TabID, res.ActionID)
	return nil
}

func (a *App) cmdTabs(ctx context.Context, op string, id int) error {
	p := map[string]any{"op": op}
	if id != 0 {
		p["id"] = id
	}
	r, err := a.hub.Request(ctx, ipc.TypeTabs, p)
	if err != nil {
		return err
	}
	var res struct {
		Tabs []struct {
			ID     int    `json:"id"`
			Active bool   `json:"active"`
			URL    string `json:"url"`
			Title  string `json:"title"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(r.Payload, &res); err != nil {
		return err
	}
	fmt.Printf("%d tab(s)\n", len(res.Tabs))
	for _, t := range res.Tabs {
		mark := " "
		if t.Active {
			mark = "*"
		}
		fmt.Printf("  %s %d  %s  %q\n", mark, t.ID, t.URL, t.Title)
	}
	return nil
}

// StorageDump is the file format of 'storage dump' / 'storage import'.
type StorageDump struct {
	Origin  string            `json:"origin"`
	Local   map[string]string `json:"localStorage"`
	Session map[string]string `json:"sessionStorage"`
}

func (a *App) cmdStorage(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: storage dump [file] | storage import <file>")
	}
	switch args[0] {
	case "dump":
		r, err := a.dom(ctx, "storage_get", nil, nil)
		if err != nil {
			return err
		}
		file := filepath.Join(a.cfg.outDir, "storage.json")
		if len(args) > 1 {
			file = args[1]
		}
		d := StorageDump{Origin: r.Origin, Local: r.Local, Session: r.Session}
		js, _ := json.MarshalIndent(d, "", "  ")
		if err := os.WriteFile(file, append(js, '\n'), 0o600); err != nil {
			return err
		}
		fmt.Printf("dumped %d localStorage + %d sessionStorage item(s) for %s to %s\n", len(d.Local), len(d.Session), d.Origin, file)
		for _, k := range sortedKeys(d.Local) {
			fmt.Printf("  local   %s=%s\n", k, clip(d.Local[k], 100))
		}
		for _, k := range sortedKeys(d.Session) {
			fmt.Printf("  session %s=%s\n", k, clip(d.Session[k], 100))
		}
	case "import":
		if len(args) != 2 {
			return fmt.Errorf("usage: storage import <file>")
		}
		raw, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		var d StorageDump
		if err := json.Unmarshal(raw, &d); err != nil {
			return err
		}
		r, err := a.dom(ctx, "storage_set", nil, map[string]any{"local": d.Local, "session": d.Session})
		if err != nil {
			return err
		}
		if d.Origin != "" && d.Origin != r.Origin {
			fmt.Printf("  note: dump is from %s, imported into %s\n", d.Origin, r.Origin)
		}
		fmt.Printf("imported %d item(s) into %s\n", r.Set, r.Origin)
	default:
		return fmt.Errorf("unknown storage subcommand %q", args[0])
	}
	return nil
}

func (a *App) exchange(idArg string) (audit.Exchange, error) {
	id, err := strconv.ParseInt(strings.TrimPrefix(idArg, "#"), 10, 64)
	if err != nil {
		return audit.Exchange{}, err
	}
	exs := a.store.Exchanges(func(ex *audit.Exchange) bool { return ex.ID == id })
	if len(exs) == 0 {
		return audit.Exchange{}, fmt.Errorf("no captured request #%d (see 'log')", id)
	}
	return exs[0], nil
}

func (a *App) cmdShow(idArg string) error {
	ex, err := a.exchange(idArg)
	if err != nil {
		return err
	}
	fmt.Printf("#%d %s %s %s\n", ex.ID, ex.Method, ex.URL, ex.Proto)
	if ex.ActionID != "" {
		if act, ok := a.store.Action(ex.ActionID); ok {
			fmt.Printf("action: %s %s <%s> %q on %s\n", ex.ActionID, act.Kind, act.Tag, act.Text, act.PageURL)
		} else {
			fmt.Printf("action: %s\n", ex.ActionID)
		}
	}
	for _, n := range ex.Intercepts {
		fmt.Printf("intercept: %s\n", n)
	}
	fmt.Println("--- request headers")
	printHeaders(ex.ReqHeader)
	if len(ex.ReqBody) > 0 {
		fmt.Println("--- request body")
		fmt.Println(bodyPreview(ex.ReqBody, ex.ReqHeader.Get("Content-Type"), ex.ReqHeader.Get("Content-Encoding")))
	}
	switch {
	case ex.Error != "":
		fmt.Printf("--- error: %s\n", ex.Error)
	case ex.Blocked:
		fmt.Printf("--- blocked by rule (status %d)\n", ex.Status)
	default:
		fmt.Printf("--- response %d (%s)\n", ex.Status, ex.Duration.Round(time.Millisecond))
		printHeaders(ex.RespHeader)
		if len(ex.RespBody) > 0 {
			fmt.Println("--- response body")
			fmt.Println(bodyPreview(ex.RespBody, ex.RespHeader.Get("Content-Type"), ex.RespHeader.Get("Content-Encoding")))
		}
	}
	if ex.Truncated {
		fmt.Println("(body truncated by the proxy's size limit)")
	}
	fmt.Println("--- end")
	return nil
}

func (a *App) cmdBody(args []string) error {
	ex, err := a.exchange(args[0])
	if err != nil {
		return err
	}
	b, _, _ := audit.Decompress(ex.RespBody, ex.RespHeader.Get("Content-Encoding"))
	if len(args) == 1 {
		os.Stdout.Write(b)
		fmt.Println()
		return nil
	}
	if err := os.WriteFile(args[1], b, 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote %d byte(s) of the response body of #%d (%s) to %s\n", len(b), ex.ID, ex.RespHeader.Get("Content-Type"), args[1])
	return nil
}

func printHeaders(h map[string][]string) {
	for _, k := range sortedKeysSlice(h) {
		for _, v := range h[k] {
			fmt.Printf("%s: %s\n", k, v)
		}
	}
}

func bodyPreview(b []byte, ctype, cenc string) string {
	dec, _, _ := audit.Decompress(b, cenc)
	ct := strings.ToLower(ctype)
	textual := ct == "" || strings.HasPrefix(ct, "text/") || strings.Contains(ct, "json") ||
		strings.Contains(ct, "javascript") || strings.Contains(ct, "xml") || strings.Contains(ct, "x-www-form-urlencoded")
	if !textual {
		return fmt.Sprintf("[%d bytes of %s; save with 'body <id> <file>']", len(dec), ctype)
	}
	const max = 4000
	if len(dec) > max {
		return string(dec[:max]) + fmt.Sprintf("\n… [%d more bytes; 'body <id> <file>' saves all]", len(dec)-max)
	}
	return string(dec)
}

func writeFile(path string, b []byte, what string) error {
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	fmt.Printf("saved %s (%d bytes) to %s\n", what, len(b), path)
	return nil
}

func hostOf(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return "page"
	}
	return strings.NewReplacer(":", "_", "/", "_").Replace(p.Host)
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedKeysSlice(m map[string][]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

var errNoTTY = errors.New("no terminal to read a secret from; pass it as $VAR instead")
