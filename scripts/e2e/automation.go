package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/share026/web-cli/internal/audit"
)

// Secrets handed to app through its environment: they must reach the page
// but never appear in app's own output for the typing commands.
const (
	e2ePassword    = "E2E-s3cret-pw"
	replayPassword = "replay-pw-42"
)

// automationChecks drives a login form and the page/network inspection
// commands through the real extension (real mode only: they use chrome.tabs,
// chrome.scripting MAIN world, chrome.debugger and chrome.storage, which the
// emulated harness does not provide).
func (h *harness) automationChecks(browserCtx context.Context, s *site, auditDir string) {
	const P = "Automation"
	uuid := `([0-9a-f-]{36})`

	// --- navigation + form input + Enter-submit --------------------------------
	_, lines, ok := h.cmd("open "+s.URL+"/login", `^opened https://\S+/login "Login"`)
	h.check(P, "open <url> waits for the load", ok, "%s", first(grep(lines, "opened")))

	_, lines, ok = h.cmd("type Email alice@example.com", `^typed 17 char\(s\) into <input:email> "Email"`)
	h.check(P, "type <label text> <value> (target by <label>)", ok, "%s", first(grep(lines, "typed")))

	from := h.mark()
	_, lines, ok = h.cmd("type Password $E2E_PASSWORD", `^typed \d+ char\(s\) into <input:password> "Password"`)
	leaked := strings.Contains(strings.Join(h.linesFrom(from), "\n"), e2ePassword)
	h.check(P, "type password from $VAR, value never printed", ok && !leaked, "%s; value in app output=%v", first(grep(lines, "typed")), leaked)

	_, lines, ok = h.cmd("select Country Japan", `^selected "Japan" in <select>`)
	h.check(P, "select <select> option by text", ok, "%s", first(grep(lines, "selected")))

	_, lines, ok = h.cmd(`check "Remember me"`, `^checked <input:checkbox> "Remember me" checked=true`)
	h.check(P, "check a checkbox (wrapping <label>)", ok, "%s", first(grep(lines, "checked")))

	m, lines, ok := h.cmd("press Enter Password", `^pressed Enter on .* action_id=`+uuid)
	pressAction := ""
	if ok {
		pressAction = m[1]
	}
	h.check(P, "press Enter in a field submits the form", ok, "%s", first(grep(lines, "pressed")))

	_, lines, ok = h.cmd("waitfor text=Welcome alice@example.com", `^found text=`)
	h.check(P, "waitfor text=... (after navigation)", ok, "%s", first(grep(lines, "found|error")))

	time.Sleep(300 * time.Millisecond)
	sess := s.requests("/session")
	body, hdr := "", ""
	if len(sess) > 0 {
		body, hdr = sess[len(sess)-1].Body, sess[len(sess)-1].Header.Get(audit.ActionHeader)
	}
	wantBody := "email=alice%40example.com&password=" + e2ePassword + "&country=jp&remember=on"
	h.check(P, "server received the filled form (POST /session)", body == wantBody && hdr == "",
		"body=%q action header upstream=%q", body, hdr)

	_, lines, ok = h.cmd("log 100", `POST https://\S+/session -> 303 .*action=`+regexp.QuoteMeta(pressAction))
	sessID := ""
	if mm := regexp.MustCompile(`#(\d+) POST https://\S+/session`).FindStringSubmatch(first(grep(lines, "/session"))); mm != nil {
		sessID = mm[1]
	}
	dashID := ""
	if mm := regexp.MustCompile(`#(\d+) GET https://\S+/dashboard\?`).FindStringSubmatch(strings.Join(grep(lines, "/dashboard"), "\n")); mm != nil {
		dashID = mm[1]
	}
	h.check(P, "form POST + redirect linked to the Enter action", ok, "%s", strings.Join(grep(lines, "/session|/dashboard"), " | "))

	// --- request inspection -------------------------------------------------------
	_, lines, ok = h.cmd("show "+sessID, `^--- end`)
	h.check(P, "show <id>: request headers + form body + response", ok &&
		len(grep(lines, `^--- request body`)) == 1 && len(grep(lines, `email=alice%40example.com`)) == 1 &&
		len(grep(lines, `^--- response 303`)) == 1 && len(grep(lines, `^Location: /dashboard`)) == 1,
		"%s", strings.Join(grep(lines, `^#\d+|^action:|^--- response|^Location`), " | "))

	served := filepath.Join(auditDir, "dashboard-served.html")
	_, lines, ok = h.cmd("body "+dashID+" "+served, `^wrote \d+ byte`)
	sb, _ := os.ReadFile(served)
	h.check(P, "body <id> <file>: HTML as served (before JavaScript)", ok && bytes.Contains(sb, []byte(`<p id="js">static</p>`)),
		"%s", first(lines))

	// --- page content ----------------------------------------------------------
	_, lines, ok = h.cmd("text", `^--- end of text`)
	h.check(P, "text: visible page text", ok && len(grep(lines, `^\s*Welcome alice@example.com$`)) == 1, "%s", strings.Join(grep(lines, "Welcome|rendered"), " | "))

	src := filepath.Join(auditDir, "dashboard-rendered.html")
	_, lines, ok = h.cmd("source "+src, `^saved rendered DOM`)
	rb, _ := os.ReadFile(src)
	h.check(P, "source <file>: rendered DOM (after JavaScript)", ok && bytes.Contains(rb, []byte(`<p id="js">rendered by js</p>`)) && bytes.HasPrefix(rb, []byte("<!DOCTYPE html>")),
		"%s", first(lines))

	_, lines, ok = h.cmd("eval document.title", `^=> "Dashboard"`)
	h.check(P, "eval <js> (page main world)", ok, "%s", first(grep(lines, "=>|error")))
	_, lines, ok = h.cmd("eval ({n: 1 + 1, path: location.pathname, theme: localStorage.theme})", `^=> \{"n":2,"path":"/dashboard","theme":"dark"\}`)
	h.check(P, "eval returns JSON values", ok, "%s", first(grep(lines, "=>|error")))

	stor := filepath.Join(auditDir, "storage.json")
	_, lines, ok = h.cmd("storage dump "+stor, `^dumped 1 localStorage \+ 1 sessionStorage`)
	h.check(P, "storage dump (localStorage + sessionStorage)", ok && len(grep(lines, `local   theme=dark`)) == 1, "%s", strings.Join(grep(lines, "dumped|theme|tab="), " | "))

	shot := filepath.Join(auditDir, "dashboard.png")
	_, lines, ok = h.cmd("screenshot "+shot, `^saved screenshot`)
	pb, _ := os.ReadFile(shot)
	h.check(P, "screenshot <file> (PNG)", ok && bytes.HasPrefix(pb, []byte("\x89PNG\r\n\x1a\n")) && len(pb) > 1000, "%s", first(lines))

	// --- history + tabs ---------------------------------------------------------
	_, lines, ok = h.cmd("back", `^back https://\S+/login`)
	h.check(P, "back", ok, "%s", first(grep(lines, "back|error")))
	_, lines, ok = h.cmd("forward", `^forward https://\S+/dashboard`)
	h.check(P, "forward", ok, "%s", first(grep(lines, "forward|error")))
	_, lines, ok = h.cmd("reload", `^reloaded https://\S+/dashboard`)
	h.check(P, "reload", ok, "%s", first(grep(lines, "reloaded|error")))

	_, lines, ok = h.cmd("newtab "+s.URL+"/next", `^opened https://\S+/next "Next"`)
	_, lines2, ok2 := h.cmd("tabs", `^2 tab\(s\)`)
	_, lines3, ok3 := h.cmd("closetab", `^1 tab\(s\)`)
	h.check(P, "newtab / tabs / closetab", ok && ok2 && ok3 && len(grep(lines2, `^\s*\* \d+  https://\S+/next`)) == 1,
		"%s | %s | %s", first(grep(lines, "opened")), strings.Join(grep(lines2, "tab|https"), " ; "), first(grep(lines3, "tab")))

	// --- eval on a page whose CSP forbids eval -> chrome.debugger fallback ---------
	h.cmd("open "+s.URL+"/csp", `^opened https://\S+/csp`)
	_, lines, ok = h.cmd("eval 6 * 7", `^=> 42 .*via debugger`)
	h.check(P, "eval under a strict CSP (debugger fallback)", ok, "%s", first(grep(lines, "=>|error")))

	// --- recording real user input, then replaying it -----------------------------
	h.cmd("open "+s.URL+"/login", `^opened https://\S+/login`)
	recFile := filepath.Join(auditDir, "login.webcli")
	_, lines, ok = h.cmd("record start "+recFile, `^recording to `)
	h.check(P, "record start", ok, "%s", first(lines))
	time.Sleep(300 * time.Millisecond)
	// trusted input through the DevTools protocol = a person using the page
	err := chromedp.Run(browserCtx,
		chromedp.Click("#email", chromedp.ByQuery),
		chromedp.SendKeys("#email", "bob@example.com", chromedp.ByQuery),
		chromedp.Click("#password", chromedp.ByQuery),
		chromedp.SendKeys("#password", "typed-by-hand", chromedp.ByQuery),
		chromedp.Click("#remember", chromedp.ByQuery),
		chromedp.Click(`button[type=submit]`, chromedp.ByQuery),
	)
	var title string
	waitTitle(browserCtx, "Dashboard", &title)
	time.Sleep(500 * time.Millisecond)
	_, lines, ok = h.cmd("record stop", `^recording stopped: (\d+) step`)
	rec, _ := os.ReadFile(recFile)
	rs := string(rec)
	h.check(P, "recording of real user input", err == nil && ok && title == "Dashboard" &&
		strings.Contains(rs, "open "+s.URL+"/login\n") &&
		strings.Contains(rs, "type css=#email bob@example.com\n") &&
		strings.Contains(rs, "type css=#password $WEBCLI_PASSWORD\n") &&
		strings.Contains(rs, "check css=#remember\n") &&
		regexp.MustCompile(`(?m)^click "?css=.*button`).MatchString(rs) &&
		!strings.Contains(rs, "typed-by-hand"),
		"err=%v %s; script:\n%s", err, first(lines), rs)

	before := len(s.requests("/session"))
	_, lines, ok = h.cmd("run "+recFile, `^run \S+: \d+ step\(s\) OK`)
	var t2 string
	waitTitle(browserCtx, "Dashboard", &t2)
	time.Sleep(500 * time.Millisecond)
	sess = s.requests("/session")
	replayed := ""
	if len(sess) > before {
		replayed = sess[len(sess)-1].Body
	}
	h.check(P, "run <recording> replays it ($WEBCLI_PASSWORD from env)", ok &&
		replayed == "email=bob%40example.com&password="+replayPassword+"&country=us&remember=on",
		"%s; server got %q", first(grep(lines, `^run |error`)), replayed)
}
