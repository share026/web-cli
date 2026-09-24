package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/share026/web-cli/internal/audit"
	"github.com/share026/web-cli/internal/ipc"
	"github.com/share026/web-cli/internal/proxy"
)

// App holds the runtime state shared by all commands.
type App struct {
	cfg       config
	hub       *ipc.Hub
	store     *audit.Store
	rules     *proxy.RuleSet
	log       *log.Logger
	proxyAddr string
	ca        tls.Certificate
}

// ClickResult is the extension's reply to a click request.
type ClickResult struct {
	ActionID string `json:"action_id"`
	Kind     string `json:"kind"`
	Hint     int    `json:"hint"`
	Tag      string `json:"tag"`
	Text     string `json:"text"`
	URL      string `json:"url"`
}

const requestTimeout = 15 * time.Second

const helpText = `commands:
  status                         sessions, proxy address, capture counts
  wait [30s]                     wait until the browser extension is connected
  ping                           round-trip ping to the extension
  hints [query]                  collect visible elements -> fzf -> click
                                 (with a query: non-interactive fzf --filter, best match)
  click <hint>                   click an element from the last 'hints' collection
  actions                        list DOM actions and their linked request count
  log [n]                        show the last n captured requests (default 20)
  export [file] [action=<id>] [host=<regexp>]
                                 write captured requests as a .http file (kulala.nvim / REST Client)
  cookies dump [file] [url]      dump cookies of the active tab (or url) to JSON
  cookies import <file>          import cookies from JSON through chrome.cookies.set
  rule add <action> k=v ...      add an interception rule, e.g.
                                   rule add block host=^ads\. status=451
                                   rule add set-req-header url=/api/ name=X-Debug value=1
                                   rule add set-resp-header host=example name=X-Audited value=yes
                                   rule add replace-body url=/config.json value="{}"
  rule list | rule del <id> | rule load <file.json>
  ca                             show CA certificate path and SPKI hash
  quit`

func (a *App) exec(line string) (quit bool, err error) {
	args := strings.Fields(line)
	if len(args) == 0 {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	switch args[0] {
	case "help", "?":
		fmt.Println(helpText)
	case "quit", "exit", "q":
		return true, nil
	case "status":
		a.cmdStatus()
	case "wait":
		d := 30 * time.Second
		if len(args) > 1 {
			if d, err = time.ParseDuration(args[1]); err != nil {
				return false, err
			}
		}
		wctx, wcancel := context.WithTimeout(context.Background(), d)
		defer wcancel()
		s, err := a.hub.WaitSession(wctx)
		if err != nil {
			return false, err
		}
		fmt.Printf("browser session #%d connected (origin %s)\n", s.ID, s.Hello.Origin)
	case "ping":
		start := time.Now()
		r, err := a.hub.Request(ctx, ipc.TypePing, map[string]any{"from": "app"})
		if err != nil {
			return false, err
		}
		fmt.Printf("%s from extension in %s: %s\n", r.Type, time.Since(start).Round(time.Microsecond), string(r.Payload))
	case "hints", "h", "f":
		return false, a.cmdHints(ctx, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), args[0])))
	case "click":
		if len(args) != 2 {
			return false, fmt.Errorf("usage: click <hint>")
		}
		hint, err := strconv.Atoi(args[1])
		if err != nil {
			return false, err
		}
		return false, a.click(ctx, hint)
	case "actions":
		a.cmdActions()
	case "log":
		n := 20
		if len(args) > 1 {
			n, _ = strconv.Atoi(args[1])
		}
		exs := a.store.Exchanges(nil)
		if len(exs) > n {
			exs = exs[len(exs)-n:]
		}
		for _, ex := range exs {
			fmt.Println(summary(ex))
		}
	case "export":
		return false, a.cmdExport(args[1:])
	case "cookies":
		return false, a.cmdCookies(ctx, args[1:])
	case "rule", "rules":
		return false, a.cmdRule(line, args[1:])
	case "ca":
		if a.proxyAddr == "" {
			return false, fmt.Errorf("proxy disabled")
		}
		fmt.Printf("CA certificate: %s\nSPKI sha256:    %s\nproxy:          http://%s (GET /ca.pem to download)\n",
			filepath.Join(a.cfg.caDir, proxy.CACertFile), proxy.SPKIHash(a.ca), a.proxyAddr)
	default:
		return false, fmt.Errorf("unknown command %q (try 'help')", args[0])
	}
	return false, nil
}

func (a *App) cmdStatus() {
	ss := a.hub.Sessions()
	fmt.Printf("socket:   %s\n", a.cfg.socket)
	fmt.Printf("sessions: %d\n", len(ss))
	for _, s := range ss {
		fmt.Printf("  #%d origin=%s nm-host pid=%d since %s\n", s.ID, s.Hello.Origin, s.Hello.PID, s.Connected.Format(time.RFC3339))
	}
	if a.proxyAddr != "" {
		fmt.Printf("proxy:    http://%s\n", a.proxyAddr)
	} else {
		fmt.Println("proxy:    disabled")
	}
	fmt.Printf("captured: %d request(s), %d action(s), %d rule(s)\n",
		len(a.store.Exchanges(nil)), len(a.store.Actions()), len(a.rules.List()))
}

func (a *App) cmdHints(ctx context.Context, query string) error {
	r, err := a.hub.Request(ctx, ipc.TypeCollect, nil)
	if err != nil {
		return err
	}
	var p ElementsPayload
	if err := json.Unmarshal(r.Payload, &p); err != nil {
		return fmt.Errorf("decode elements: %w", err)
	}
	fmt.Printf("collected %d visible element(s) on %s\n", len(p.Elements), p.URL)
	for _, e := range p.Elements {
		fmt.Printf("  %s\n", strings.ReplaceAll(e.fzfLine(), "\t", "  "))
	}
	hint, err := selectWithFzf(a.cfg.fzf, p.Elements, query, p.Title+"  "+p.URL)
	if err != nil {
		return err
	}
	fmt.Printf("fzf selected hint %d\n", hint)
	// fzf may have taken a while (interactive); use a fresh deadline for the click.
	cctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	return a.click(cctx, hint)
}

func (a *App) click(ctx context.Context, hint int) error {
	r, err := a.hub.Request(ctx, ipc.TypeClick, map[string]any{"hint": hint, "tag": a.proxyAddr != ""})
	if err != nil {
		return err
	}
	var res ClickResult
	if err := json.Unmarshal(r.Payload, &res); err != nil {
		return fmt.Errorf("decode click result: %w", err)
	}
	a.store.AddAction(audit.Action{
		ID: res.ActionID, Time: time.Now(), Kind: res.Kind, Hint: res.Hint,
		Tag: res.Tag, Text: res.Text, PageURL: res.URL,
	})
	fmt.Printf("%s hint=%d <%s> %q action_id=%s\n", res.Kind, res.Hint, res.Tag, res.Text, res.ActionID)
	return nil
}

func (a *App) cmdActions() {
	exs := a.store.Exchanges(nil)
	count := map[string]int{}
	for _, ex := range exs {
		if ex.ActionID != "" {
			count[ex.ActionID]++
		}
	}
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ACTION ID\tKIND\tHINT\tELEMENT\tREQUESTS\tPAGE")
	for _, act := range a.store.Actions() {
		fmt.Fprintf(tw, "%s\t%s\t%d\t<%s> %q\t%d\t%s\n", act.ID, act.Kind, act.Hint, act.Tag, act.Text, count[act.ID], act.PageURL)
	}
	tw.Flush()
}

func (a *App) cmdExport(args []string) error {
	file := filepath.Join(a.cfg.outDir, "requests.http")
	var actionID string
	var hostRe *regexp.Regexp
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "action="):
			actionID = strings.TrimPrefix(arg, "action=")
		case strings.HasPrefix(arg, "host="):
			re, err := regexp.Compile(strings.TrimPrefix(arg, "host="))
			if err != nil {
				return err
			}
			hostRe = re
		default:
			file = arg
		}
	}
	exs := a.store.Exchanges(func(ex *audit.Exchange) bool {
		if actionID != "" && ex.ActionID != actionID {
			return false
		}
		if hostRe != nil && !hostRe.MatchString(ex.URL) {
			return false
		}
		return true
	})
	f, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := audit.WriteHTTPFile(f, exs, audit.HTTPFileOptions{Actions: a.store.Action}); err != nil {
		return err
	}
	fmt.Printf("wrote %d request(s) to %s\n", len(exs), file)
	return nil
}

func (a *App) cmdCookies(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: cookies dump [file] [url] | cookies import <file>")
	}
	switch args[0] {
	case "dump":
		file := filepath.Join(a.cfg.outDir, "cookies.json")
		req := map[string]any{}
		for _, arg := range args[1:] {
			if strings.Contains(arg, "://") {
				req["url"] = arg
			} else {
				file = arg
			}
		}
		r, err := a.hub.Request(ctx, ipc.TypeCookiesGet, req)
		if err != nil {
			return err
		}
		var d audit.CookieDump
		if err := json.Unmarshal(r.Payload, &d); err != nil {
			return fmt.Errorf("decode cookies: %w", err)
		}
		if err := audit.SaveCookies(file, d); err != nil {
			return err
		}
		fmt.Printf("dumped %d cookie(s) for %s to %s\n", len(d.Cookies), d.URL, file)
		for _, c := range d.Cookies {
			fmt.Printf("  %s=%s; domain=%s; path=%s; secure=%v; httpOnly=%v\n", c.Name, c.Value, c.Domain, c.Path, c.Secure, c.HTTPOnly)
		}
	case "import":
		if len(args) != 2 {
			return fmt.Errorf("usage: cookies import <file>")
		}
		d, err := audit.LoadCookies(args[1])
		if err != nil {
			return err
		}
		details := make([]audit.CookieSetDetails, 0, len(d.Cookies))
		for _, c := range d.Cookies {
			sd, err := c.ToSetDetails()
			if err != nil {
				return err
			}
			details = append(details, sd)
		}
		r, err := a.hub.Request(ctx, ipc.TypeCookiesSet, map[string]any{"cookies": details})
		if err != nil {
			return err
		}
		var res struct {
			Set    int      `json:"set"`
			Errors []string `json:"errors"`
		}
		if err := json.Unmarshal(r.Payload, &res); err != nil {
			return err
		}
		fmt.Printf("imported %d/%d cookie(s) from %s\n", res.Set, len(details), args[1])
		for _, e := range res.Errors {
			fmt.Printf("  error: %s\n", e)
		}
	default:
		return fmt.Errorf("unknown cookies subcommand %q", args[0])
	}
	return nil
}

func (a *App) cmdRule(line string, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		rs := a.rules.List()
		if len(rs) == 0 {
			fmt.Println("no rules")
		}
		for _, r := range rs {
			fmt.Println(r.String())
		}
		return nil
	}
	switch args[0] {
	case "add":
		_, spec, _ := strings.Cut(line, "add")
		r, err := proxy.ParseRule(strings.TrimSpace(spec))
		if err != nil {
			return err
		}
		r, err = a.rules.Add(r)
		if err != nil {
			return err
		}
		fmt.Printf("added rule %s\n", r.String())
	case "del", "rm":
		if len(args) != 2 {
			return fmt.Errorf("usage: rule del <id>")
		}
		id, err := strconv.Atoi(strings.TrimPrefix(args[1], "#"))
		if err != nil {
			return err
		}
		if !a.rules.Delete(id) {
			return fmt.Errorf("no rule #%d", id)
		}
		fmt.Printf("deleted rule #%d\n", id)
	case "load":
		if len(args) != 2 {
			return fmt.Errorf("usage: rule load <file.json>")
		}
		n, err := a.rules.LoadFile(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("loaded %d rule(s)\n", n)
	default:
		return fmt.Errorf("unknown rule subcommand %q", args[0])
	}
	return nil
}

func summary(ex audit.Exchange) string {
	s := fmt.Sprintf("#%d %s %s -> ", ex.ID, ex.Method, ex.URL)
	switch {
	case ex.Error != "":
		s += "error: " + ex.Error
	case ex.Blocked:
		s += fmt.Sprintf("%d BLOCKED", ex.Status)
	default:
		s += fmt.Sprintf("%d (%d bytes, %s)", ex.Status, len(ex.RespBody), ex.Duration.Round(time.Millisecond))
	}
	if ex.ActionID != "" {
		s += " action=" + ex.ActionID
	}
	if len(ex.Intercepts) > 0 {
		s += fmt.Sprintf(" intercepts=%d", len(ex.Intercepts))
	}
	return s
}
