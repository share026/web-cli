// Command e2e is the end-to-end verification harness for web-cli.
//
// It starts the real `app` (Unix socket server + goproxy MITM proxy + fzf),
// registers the real `nm-host` with scripts/install-host.sh, launches a real
// Chromium (headless) whose traffic goes through the proxy, runs the shipped
// extension/background.js and extension/content.js inside it (see exthost.go),
// and drives the whole flow through app's command interface:
//
//	Phase 1  ping/pong across extension <-> nm-host <-> app
//	Phase 2  Vimium C style element collection, fzf selection, click
//	Phase 3  HTTPS capture + interception rules in the proxy
//	Phase 4  action-ID linkage, .http export, cookie dump/import
//
// Usage: CHROME_PATH=/path/to/chromium go run ./scripts/e2e [-out scripts/e2e/out]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/share026/web-cli/internal/audit"
)

type check struct {
	Phase, Name string
	OK          bool
	Detail      string
}

type harness struct {
	root, out string
	logFile   *os.File
	mu        sync.Mutex
	checks    []check

	app      *exec.Cmd
	appIn    io.WriteCloser
	linesMu  sync.Mutex
	lines    []string
	linesSig chan struct{}
}

func (h *harness) logf(format string, args ...any) {
	line := fmt.Sprintf("%s %s", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	h.mu.Lock()
	defer h.mu.Unlock()
	fmt.Fprintln(h.logFile, line)
	fmt.Println(line)
}

func (h *harness) check(phase, name string, ok bool, detail string, args ...any) {
	d := fmt.Sprintf(detail, args...)
	h.mu.Lock()
	h.checks = append(h.checks, check{phase, name, ok, d})
	h.mu.Unlock()
	mark := "PASS"
	if !ok {
		mark = "FAIL"
	}
	h.logf("[%s] %s: %s — %s", mark, phase, name, d)
}

func (h *harness) fatal(format string, args ...any) {
	h.logf("FATAL: "+format, args...)
	h.writeReport()
	os.Exit(1)
}

// --- app process ----------------------------------------------------------

func (h *harness) startApp(args ...string) {
	h.linesSig = make(chan struct{})
	cmd := exec.Command(filepath.Join(h.root, "bin/app"), args...)
	cmd.Dir = h.root
	in, _ := cmd.StdinPipe()
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	transcript, _ := os.Create(filepath.Join(h.out, "app-transcript.log"))
	if err := cmd.Start(); err != nil {
		h.fatal("start app: %v", err)
	}
	h.app, h.appIn = cmd, in
	go func() {
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			line := sc.Text()
			fmt.Fprintln(transcript, line)
			h.logf("  app| %s", line)
			h.linesMu.Lock()
			h.lines = append(h.lines, line)
			close(h.linesSig)
			h.linesSig = make(chan struct{})
			h.linesMu.Unlock()
		}
	}()
}

func (h *harness) mark() int {
	h.linesMu.Lock()
	defer h.linesMu.Unlock()
	return len(h.lines)
}

// waitLine waits for a line matching re at or after index from.
func (h *harness) waitLine(from int, re string, timeout time.Duration) ([]string, bool) {
	rx := regexp.MustCompile(re)
	deadline := time.After(timeout)
	for {
		h.linesMu.Lock()
		sig := h.linesSig
		for i := from; i < len(h.lines); i++ {
			if m := rx.FindStringSubmatch(h.lines[i]); m != nil {
				h.linesMu.Unlock()
				return m, true
			}
		}
		h.linesMu.Unlock()
		select {
		case <-sig:
		case <-deadline:
			return nil, false
		}
	}
}

func (h *harness) linesFrom(from int) []string {
	h.linesMu.Lock()
	defer h.linesMu.Unlock()
	return append([]string(nil), h.lines[from:]...)
}

// cmd sends one command to app and waits for a line matching done (or an error line).
func (h *harness) cmd(command, done string) ([]string, []string, bool) {
	from := h.mark()
	h.logf("> %s", command)
	fmt.Fprintln(h.appIn, command)
	m, ok := h.waitLine(from, `(?:`+done+`)|^error: `, 20*time.Second)
	if ok && strings.HasPrefix(m[0], "error: ") {
		return m, h.linesFrom(from), false
	}
	time.Sleep(100 * time.Millisecond) // let trailing lines arrive
	return m, h.linesFrom(from), ok
}

// --- helpers ---------------------------------------------------------------

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func run(dir, name string, args ...string) (string, error) {
	c := exec.Command(name, args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return string(out), err
}

func main() {
	out := flag.String("out", "scripts/e2e/out", "output directory for logs, report and artifacts")
	chrome := flag.String("chrome", os.Getenv("CHROME_PATH"), "Chromium binary (env CHROME_PATH)")
	flag.Parse()

	root, _ := os.Getwd()
	for !exists(filepath.Join(root, "go.mod")) {
		root = filepath.Dir(root)
	}
	os.RemoveAll(*out)
	os.MkdirAll(*out, 0o755)
	outAbs, _ := filepath.Abs(*out)
	lf, _ := os.Create(filepath.Join(outAbs, "harness.log"))
	h := &harness{root: root, out: outAbs, logFile: lf}
	if *chrome == "" {
		h.fatal("set CHROME_PATH to a Chromium binary")
	}

	// 0. build + host registration --------------------------------------------
	if o, err := run(root, "scripts/build.sh"); err != nil {
		h.fatal("build: %v\n%s", err, o)
	} else {
		h.logf("scripts/build.sh: %s", strings.TrimSpace(o))
	}
	userData := filepath.Join(outAbs, "chrome-profile")
	o, err := run(root, "scripts/install-host.sh", userData)
	if err != nil {
		h.fatal("install-host: %v\n%s", err, o)
	}
	h.logf("scripts/install-host.sh:\n%s", strings.TrimSpace(o))
	extID := strings.TrimSpace(mustRun(root, "scripts/extension-id.sh"))

	// 1. dummy origin server (self-signed HTTPS) ------------------------------
	s := newSite()
	defer s.Close()
	siteCA := filepath.Join(outAbs, "site-ca.pem")
	os.WriteFile(siteCA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0o644)
	h.logf("dummy HTTPS site: %s", s.URL)

	// 2. app -----------------------------------------------------------------
	sock := filepath.Join(outAbs, "ipc.sock")
	proxyAddr := fmt.Sprintf("127.0.0.1:%d", freePort())
	auditDir := filepath.Join(outAbs, "audit")
	h.startApp("-socket", sock, "-proxy", proxyAddr, "-ca-dir", filepath.Join(outAbs, "ca"),
		"-out", auditDir, "-upstream-ca", siteCA)
	defer func() {
		fmt.Fprintln(h.appIn, "quit")
		h.app.Wait()
	}()
	m, ok := h.waitLine(0, `\[proxy\] listening on (\S+) .*SPKI (\S+)\)`, 10*time.Second)
	if !ok {
		h.fatal("app did not start the proxy")
	}
	spki := m[2]
	h.check("Phase 3", "dynamic root CA generated", exists(filepath.Join(outAbs, "ca", "ca.pem")), "ca.pem created, SPKI %s", spki)

	// 3. Chromium through the proxy ------------------------------------------
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(*chrome),
		chromedp.UserDataDir(userData),
		chromedp.NoSandbox,
		chromedp.ProxyServer("http://"+proxyAddr),
		chromedp.Flag("proxy-bypass-list", "<-loopback>"), // send 127.0.0.1 through the proxy too
		// trust exactly our CA (by public key), not "ignore all errors"
		chromedp.Flag("ignore-certificate-errors-spki-list", spki),
		chromedp.WindowSize(1000, 700),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	if err := chromedp.Run(browserCtx); err != nil {
		h.fatal("launch chromium: %v", err)
	}
	var ver string
	chromedp.Run(browserCtx, chromedp.Evaluate(`navigator.userAgent`, &ver))
	h.logf("browser: %s", ver)

	ext := &extHost{extDir: filepath.Join(root, "extension"), extID: extID, userDataDir: userData, socket: sock, logf: h.logf}
	mark := h.mark()
	if err := ext.start(browserCtx); err != nil {
		h.fatal("extension runtime: %v", err)
	}
	defer ext.stop()

	// ===== Phase 1: ping/pong ================================================
	_, ok = h.waitLine(mark, `\[ipc\] session #\d+ hello origin=chrome-extension://`+extID+`/`, 10*time.Second)
	h.check("Phase 1", "nm-host started by extension and connected to app", ok, "hello with origin chrome-extension://%s/", extID)
	_, ok = h.waitLine(mark, `\[ipc\] session #\d+ ping -> pong`, 10*time.Second)
	h.check("Phase 1", "extension {type:ping} answered by Go with {type:pong}", ok, "app log: ping -> pong")
	_, ok = h.waitLine(mark, `\[ext#\d+\] \{"event":"pong_received"`, 10*time.Second)
	h.check("Phase 1", "background.js received pong", ok, "extension reported pong_received back over the bridge")
	m, lines, ok := h.cmd("ping", `^pong from extension in (\S+)`)
	h.check("Phase 1", "app -> extension ping round trip", ok, "%s", first(lines))
	_ = m

	// ===== Phase 2: collect + fzf + click =====================================
	if err := chromedp.Run(browserCtx, chromedp.Navigate(s.URL+"/")); err != nil {
		h.fatal("navigate: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	m, lines, ok = h.cmd("hints Login", `action_id=([0-9a-f-]{36})`)
	h.check("Phase 2", "hints Login -> fzf -> click", ok, "%s", strings.Join(grep(lines, "fzf selected|action_id"), " | "))
	loginAction := ""
	if ok {
		loginAction = m[1]
	}
	visible := parseElements(lines)
	wantVisible := []string{"Next page", "Login", "Who am I", "Search products", "English", "Role Button", "Onclick span"}
	hidden := []string{"Hidden display", "Hidden visibility", "Hidden opacity", "Hidden parent opacity", "Zero size", "Offscreen", "Disabled", "Covered button", "Below fold"}
	missing, leaked := diff(visible, wantVisible, hidden)
	h.check("Phase 2", "Vimium C visibility filter", len(missing) == 0 && len(leaked) == 0,
		"visible=%q missing=%q leaked-hidden=%q", visible, missing, leaked)

	var status string
	waitText(browserCtx, "#status", "logged in: alice", &status)
	h.check("Phase 2", "click fired in page (fetch /api/login ran)", status == "logged in: alice", "#status=%q", status)

	// ===== Phase 4a: action ID linkage =========================================
	time.Sleep(300 * time.Millisecond)
	logins := s.requests("/api/login")
	leakedHdr := false
	for _, r := range logins {
		leakedHdr = leakedHdr || r.Header.Get(audit.ActionHeader) != ""
	}
	h.check("Phase 4", "X-Audit-Action-Id stripped before upstream", len(logins) == 1 && !leakedHdr,
		"origin saw %d POST /api/login, action header present=%v, body=%s", len(logins), leakedHdr, bodyOf(logins))
	_, lines, ok = h.cmd("log 50", `POST https://127\.0\.0\.1:\d+/api/login -> 200 .*action=`+regexp.QuoteMeta(loginAction))
	h.check("Phase 4", "proxy linked POST /api/login to the click action", ok, "%s", first(grep(lines, "/api/login")))

	// ===== Phase 3: interception rules through the browser =====================
	_, lines, ok = h.cmd(`rule add set-req-header url=/api/whoami name=X-Injected value=e2e-rule`, `^added rule`)
	h.check("Phase 3", "rule add set-req-header", ok, "%s", first(lines))
	_, lines, ok = h.cmd(`rule add block url=/blocked status=451`, `^added rule`)
	h.check("Phase 3", "rule add block", ok, "%s", first(lines))
	_, lines, ok = h.cmd(`rule add set-resp-header url=/api/whoami name=X-Audited value=web-cli`, `^added rule`)
	h.check("Phase 3", "rule add set-resp-header", ok, "%s", first(lines))

	// ===== Phase 4b: cookies ===================================================
	cookieFile := filepath.Join(auditDir, "cookies.json")
	_, lines, ok = h.cmd("cookies dump "+cookieFile, `^dumped \d+ cookie`)
	d, _ := audit.LoadCookies(cookieFile)
	hasSession := false
	for _, c := range d.Cookies {
		hasSession = hasSession || (c.Name == "session" && c.Value == "abc123" && c.HTTPOnly && c.Secure)
	}
	h.check("Phase 4", "cookies dump via chrome.cookies.getAll", ok && hasSession, "%s; file has session=abc123 (HttpOnly,Secure)=%v", first(lines), hasSession)

	importFile := filepath.Join(auditDir, "cookies-import.json")
	u := strings.TrimPrefix(s.URL, "https://")
	host := strings.Split(u, ":")[0]
	audit.SaveCookies(importFile, audit.CookieDump{Cookies: []audit.Cookie{
		{Name: "imported", Value: "from-json", Domain: host, HostOnly: true, Path: "/", Secure: true, SameSite: "lax", Session: true},
	}})
	_, lines, ok = h.cmd("cookies import "+importFile, `^imported 1/1 cookie`)
	h.check("Phase 4", "cookies import via chrome.cookies.set", ok, "%s", strings.Join(lines, " | "))

	m, lines, ok = h.cmd("hints who am", `action_id=([0-9a-f-]{36})`)
	whoAction := ""
	if ok {
		whoAction = m[1]
	}
	h.check("Phase 2", "hints 'who am' -> click", ok, "%s", first(grep(lines, "action_id")))
	waitText(browserCtx, "#status2", "blocked status: 451", &status)
	var status1 string
	chromedp.Run(browserCtx, chromedp.Text("#status", &status1))
	who := s.requests("/api/whoami")
	cookieHdr, injected := "", ""
	if len(who) > 0 {
		cookieHdr, injected = who[0].Header.Get("Cookie"), who[0].Header.Get("X-Injected")
	}
	h.check("Phase 4", "imported cookie sent by the browser", strings.Contains(cookieHdr, "imported=from-json") && strings.Contains(cookieHdr, "session=abc123"),
		"origin received Cookie: %q; page shows %q", cookieHdr, status1)
	h.check("Phase 3", "request header injected by rule", injected == "e2e-rule", "origin received X-Injected: %q", injected)
	h.check("Phase 3", "request blocked by rule (never reached origin)", status == "blocked status: 451" && len(s.requests("/blocked")) == 0,
		"page shows %q, origin saw %d /blocked requests", status, len(s.requests("/blocked")))
	_, lines, _ = h.cmd("log 50", `/blocked -> 451 BLOCKED`)
	h.check("Phase 3", "blocked + modified requests recorded with intercept notes",
		len(grep(lines, `/blocked -> 451 BLOCKED action=`+whoAction)) == 1 && len(grep(lines, `/api/whoami -> 200 .*action=`+whoAction+` intercepts=2`)) == 1,
		"%s", strings.Join(grep(lines, "whoami|blocked"), " | "))

	// ===== Phase 2c: navigation click + re-collection on the new page ============
	m, lines, ok = h.cmd("hints next page", `action_id=([0-9a-f-]{36})`)
	navAction := ""
	if ok {
		navAction = m[1]
	}
	h.check("Phase 2", "hints 'next page' -> link click navigates", ok, "%s", first(grep(lines, "action_id")))
	var title string
	waitTitle(browserCtx, "Next", &title)
	h.check("Phase 2", "navigation happened", title == "Next", "document.title=%q", title)
	nextReqs := s.requests("/next")
	h.check("Phase 4", "main_frame navigation tagged, header stripped upstream", len(nextReqs) == 1 && nextReqs[0].Header.Get(audit.ActionHeader) == "",
		"origin saw %d GET /next without action header", len(nextReqs))
	time.Sleep(300 * time.Millisecond)
	m, lines, ok = h.cmd("hints press", `action_id=([0-9a-f-]{36})`)
	h.check("Phase 2", "content script re-injected after navigation; focus/click on new page", ok, "%s", strings.Join(grep(lines, "collected|action_id"), " | "))
	var pressed string
	waitText(browserCtx, "button", "clicked", &pressed)
	h.check("Phase 2", "button on new page clicked", pressed == "clicked", "button text=%q", pressed)
	m, lines, ok = h.cmd("hints search", `action_id=`)
	h.check("Phase 2", "stale page: 'search' has no match on new page", !ok && len(grep(lines, "no element matches")) == 1, "%s", first(grep(lines, "error")))

	// ===== Phase 4c: .http export ==============================================
	httpAll := filepath.Join(auditDir, "requests.http")
	_, lines, ok = h.cmd("export "+httpAll, `^wrote \d+ request`)
	raw, _ := os.ReadFile(httpAll)
	txt := string(raw)
	h.check("Phase 4", "export all captured requests to .http", ok &&
		strings.Contains(txt, "POST "+s.URL+"/api/login HTTP/1.1") &&
		strings.Contains(txt, `{"user":"alice","password":"wonderland"}`) &&
		strings.Contains(txt, "# action: "+loginAction+" click hint=") &&
		!strings.Contains(txt, audit.ActionHeader),
		"%s", first(lines))
	httpLogin := filepath.Join(auditDir, "login-action.http")
	_, lines, ok = h.cmd("export "+httpLogin+" action="+loginAction, `^wrote 1 request`)
	h.check("Phase 4", "export filtered by action ID", ok, "%s", first(lines))
	_, lines, ok = h.cmd("export "+filepath.Join(auditDir, "nav-action.http")+" action="+navAction, `^wrote \d+ request`)
	h.check("Phase 4", "navigation action has linked requests", ok && !strings.Contains(first(lines), "wrote 0"), "%s", first(lines))
	_, lines, _ = h.cmd("actions", `^ACTION ID`)
	time.Sleep(200 * time.Millisecond)
	h.logf("actions table:\n%s", strings.Join(h.linesFrom(h.mark()-6), "\n"))
	_, lines, ok = h.cmd("status", `^captured: `)
	h.check("Phase 1", "status reports session and captures", ok, "%s", strings.Join(grep(lines, "sessions|captured"), " | "))

	// Replay the exported .http login request with curl through nothing but the
	// origin (proves the file is a valid, executable request description).
	h.check("Phase 4", ".http file replays with curl", replayWithCurl(h, httpLogin, siteCA), "see harness.log")

	h.writeReport()
	for _, c := range h.checks {
		if !c.OK {
			os.Exit(1)
		}
	}
}

// replayWithCurl parses the single request in an exported .http file and
// sends it with curl, as kulala.nvim does under the hood.
func replayWithCurl(h *harness, path, caFile string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var reqLine string
	var headers []string
	var body strings.Builder
	state := 0
	for _, l := range strings.Split(string(raw), "\n") {
		switch {
		case state == 0 && (strings.HasPrefix(l, "#") || strings.TrimSpace(l) == ""):
		case state == 0:
			reqLine, state = l, 1
		case state == 1 && l == "":
			state = 2
		case state == 1:
			headers = append(headers, l)
		case state == 2:
			body.WriteString(l)
		}
	}
	parts := strings.Fields(reqLine)
	if len(parts) != 3 {
		return false
	}
	args := []string{"-sS", "--cacert", caFile, "-X", parts[0], parts[1]}
	for _, hd := range headers {
		args = append(args, "-H", hd)
	}
	if body.Len() > 0 {
		args = append(args, "--data-raw", body.String())
	}
	out, err := exec.Command("curl", args...).CombinedOutput()
	h.logf("curl replay of %s: %s %s -> %s (err=%v)", filepath.Base(path), parts[0], parts[1], strings.TrimSpace(string(out)), err)
	return err == nil && strings.Contains(string(out), `"user":"alice"`)
}

func (h *harness) writeReport() {
	var b strings.Builder
	pass := 0
	for _, c := range h.checks {
		if c.OK {
			pass++
		}
	}
	fmt.Fprintf(&b, "# web-cli E2E report\n\n- date: %s\n- result: %d/%d checks passed\n\n", time.Now().Format(time.RFC3339), pass, len(h.checks))
	b.WriteString("| # | Phase | Check | Result | Evidence |\n|---|---|---|---|---|\n")
	for i, c := range h.checks {
		r := "✅ PASS"
		if !c.OK {
			r = "❌ FAIL"
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n", i+1, c.Phase, c.Name, r, strings.ReplaceAll(strings.ReplaceAll(c.Detail, "|", "\\|"), "\n", " "))
	}
	os.WriteFile(filepath.Join(h.out, "e2e-report.md"), []byte(b.String()), 0o644)
	js, _ := json.MarshalIndent(h.checks, "", "  ")
	os.WriteFile(filepath.Join(h.out, "e2e-report.json"), js, 0o644)
	h.logf("E2E: %d/%d checks passed — report: %s", pass, len(h.checks), filepath.Join(h.out, "e2e-report.md"))
}

// parseElements extracts element texts from app's "collected" listing.
func parseElements(lines []string) []string {
	var out []string
	in := false
	rx := regexp.MustCompile(`^  \d+  \[[^\]]+\]  (.*?)  `)
	for _, l := range lines {
		if strings.HasPrefix(l, "collected ") {
			in = true
			continue
		}
		if in {
			m := rx.FindStringSubmatch(l + "  ")
			if m == nil {
				break
			}
			out = append(out, m[1])
		}
	}
	return out
}

func diff(got, want, hidden []string) (missing, leaked []string) {
	set := map[string]bool{}
	for _, g := range got {
		set[g] = true
	}
	for _, w := range want {
		if !set[w] {
			missing = append(missing, w)
		}
	}
	for _, hdn := range hidden {
		if set[hdn] {
			leaked = append(leaked, hdn)
		}
	}
	sort.Strings(missing)
	return
}

func grep(lines []string, re string) []string {
	rx := regexp.MustCompile(re)
	var out []string
	for _, l := range lines {
		if rx.MatchString(l) {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func first(lines []string) string {
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

func bodyOf(rs []seenRequest) string {
	if len(rs) == 0 {
		return ""
	}
	return rs[0].Body
}

func waitText(ctx context.Context, sel, want string, got *string) {
	for i := 0; i < 50; i++ {
		c, cancel := context.WithTimeout(ctx, time.Second)
		chromedp.Run(c, chromedp.Text(sel, got, chromedp.ByQuery))
		cancel()
		if *got == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func waitTitle(ctx context.Context, want string, got *string) {
	for i := 0; i < 50; i++ {
		c, cancel := context.WithTimeout(ctx, time.Second)
		chromedp.Run(c, chromedp.Title(got))
		cancel()
		if *got == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func mustRun(dir, name string, args ...string) string {
	o, err := run(dir, name, args...)
	if err != nil {
		panic(fmt.Sprintf("%s: %v\n%s", name, err, o))
	}
	return o
}
