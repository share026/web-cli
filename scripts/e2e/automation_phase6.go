package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// phase6Checks: iframes (same- and cross-origin), open shadow DOM, file
// upload (chunked over native messaging), WebSocket capture, throttling and
// load timing - all through the real extension (real mode only).
func (h *harness) phase6Checks(browserCtx context.Context, s *site, auditDir string) {
	const P = "Phase 6"
	eval := func(js string) string {
		var out string
		c, cancel := context.WithTimeout(browserCtx, 5*time.Second)
		defer cancel()
		_ = chromedp.Run(c, chromedp.Evaluate(js, &out))
		return out
	}
	frameBodies := func() []string {
		var out []string
		for _, r := range s.requests("/api/frame") {
			out = append(out, r.Body)
		}
		return out
	}
	hasBody := func(want string) bool {
		for i := 0; i < 30; i++ {
			for _, b := range frameBodies() {
				if b == want {
					return true
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		return false
	}

	// --- iframes + shadow DOM ------------------------------------------------
	_, lines, ok := h.cmd("open "+s.URL+"/frames", `^opened https://\S+/frames "Frames"`)
	h.check(P, "open page with iframes", ok, "%s", first(grep(lines, "opened|error")))
	time.Sleep(800 * time.Millisecond) // let both iframes load

	_, lines, ok = h.cmd("list", `^\d+ element\(s\) on `)
	xoURL := s.XO.URL + "/xo-inner"
	sameFrame := grep(lines, `Frame Submit.*\(frame https://\S+/frame-inner\)`)
	xoFrame := grep(lines, `XO Submit.*\(frame `+regexp.QuoteMeta(xoURL)+`\)`)
	shadow := grep(lines, `\[button\]\s+Shadow Save`)
	h.check(P, "list includes same-origin iframe, cross-origin iframe and shadow DOM controls",
		ok && len(sameFrame) == 1 && len(xoFrame) == 1 && len(shadow) == 1,
		"same=%q xo=%q shadow=%q", first(sameFrame), first(xoFrame), first(shadow))

	_, lines, ok = h.cmd(`type "Frame Name" Bob`, `^typed 3 char\(s\) into <input:text> "Frame Name" in frame \d+`)
	_, lines2, ok2 := h.cmd(`click "Frame Submit"`, `^click <button> "Frame Submit" in frame \d+`)
	h.check(P, "type + click inside a same-origin iframe", ok && ok2 && hasBody("same:Bob"),
		"%s | %s | server got %q", first(grep(lines, "typed|error")), first(grep(lines2, "click|error")), frameBodies())

	// press without a target goes to the frame that has the focus.
	_, lines, ok = h.cmd(`type "XO Name" Carol`, `^typed 5 char\(s\) into <input:text> "XO Name" in frame \d+ \(`+regexp.QuoteMeta(xoURL)+`\)`)
	_, lines2, ok2 = h.cmd("press Enter", `^pressed Enter on <input:text> "XO Name" in frame`)
	h.check(P, "cross-origin iframe: type, then 'press Enter' submits in the focused frame", ok && ok2 && hasBody("xo:Carol"),
		"%s | %s | server got %q", first(grep(lines, "typed|error")), first(grep(lines2, "pressed|error")), frameBodies())

	_, lines, ok = h.cmd("click css=#xo-btn", `^click <button> "XO Submit" in frame`)
	h.check(P, "css= selector found in a cross-origin iframe", ok && hasBody("xo:Carol") && len(frameBodies()) >= 3,
		"%s | requests=%d", first(grep(lines, "click|error")), len(frameBodies()))

	_, lines, ok = h.cmd(`type "Shadow input" shadow-value`, `^typed 12 char\(s\) into <input:text> "Shadow input" selector=#shadow-in`)
	_, lines2, ok2 = h.cmd(`click "Shadow Save"`, `^click <button> "Shadow Save"`)
	time.Sleep(200 * time.Millisecond)
	out := eval(`document.querySelector('x-widget').shadowRoot.getElementById('out').textContent`)
	h.check(P, "type + click inside an open shadow root (label resolved in the shadow tree)", ok && ok2 && out == "saved: shadow-value",
		"%s | %s | page shows %q", first(grep(lines, "typed|error")), first(grep(lines2, "click|error")), out)
	_, lines, ok = h.cmd("type css=#shadow-in via-css", `^typed 7 char\(s\) into`)
	val := eval(`document.querySelector('x-widget').shadowRoot.getElementById('shadow-in').value`)
	h.check(P, "css= selector inside a shadow root", ok && val == "via-css", "%s | value=%q", first(grep(lines, "typed|error")), val)

	// --- file upload -------------------------------------------------------------
	dir := filepath.Join(auditDir, "upload")
	os.MkdirAll(dir, 0o755)
	big := make([]byte, 1_500_000) // > 1 MB: needs several native messages
	rand.Read(big)
	bigPath := filepath.Join(dir, "photo.png")
	os.WriteFile(bigPath, big, 0o644)
	txtPath := filepath.Join(dir, "notes.txt")
	os.WriteFile(txtPath, []byte("hello upload\n"), 0o644)
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

	h.cmd("open "+s.URL+"/upload", `^opened https://\S+/upload`)
	_, lines, ok = h.cmd(`upload "Choose files" `+bigPath+" "+txtPath, `^uploaded 2 file\(s\) \(1500013 bytes: photo.png, notes.txt\) into <input:file>`)
	picked := eval(`document.getElementById('picked').textContent`)
	h.check(P, "upload 1.5 MB + text file via the label of a hidden <input type=file multiple> (chunked)",
		ok && picked == "photo.png:1500000:image/png,notes.txt:13:text/plain",
		"%s | page change event saw %q", first(grep(lines, "uploaded|error")), picked)

	_, lines, ok = h.cmd(`click "Send files"`, `^click `)
	var title string
	waitTitle(browserCtx, "Uploaded", &title)
	ups := s.p6.lastUploads()
	good := len(ups) == 2 && ups[0].Name == "photo.png" && ups[0].SHA256 == sum(big) && ups[0].Type == "image/png" &&
		ups[1].Name == "notes.txt" && ups[1].Size == 13
	h.check(P, "server received both files byte-identical (multipart POST, sha256)", ok && good, "title=%q server got %+v", title, ups)

	h.cmd("open "+s.URL+"/upload", `^opened https://\S+/upload`)
	_, lines, ok = h.cmd("upload css=#drop "+txtPath, `^dropped 1 file\(s\) \(13 bytes: notes.txt\) into <div>`)
	dropped := eval(`document.getElementById('dropped').textContent`)
	h.check(P, "upload onto a drop zone (drag&drop events with the file)", ok && dropped == "notes.txt:hello upload",
		"%s | page saw %q", first(grep(lines, "dropped|error")), dropped)

	// --- WebSocket ---------------------------------------------------------------
	h.cmd("open "+s.URL+"/ws-page", `^opened https://\S+/ws-page`)
	wsText := ""
	for i := 0; i < 50 && !strings.Contains(wsText, "second message"); i++ {
		wsText = eval(`document.getElementById('ws').textContent`)
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(300 * time.Millisecond)
	m, lines, ok := h.cmd("ws", `^\s*#(\d+)\s+\S+\s+\d+ frame\(s\)\s+https://\S+/ws$`)
	h.check(P, "WebSocket works through the MITM proxy and is listed by 'ws'", ok && strings.Contains(wsText, "echo:hello-ws"),
		"page=%q | %s", wsText, first(grep(lines, "frame|error|no WebSocket")))
	if ok {
		_, lines, ok = h.cmd("ws "+m[1], `^#\d+ \S+ — \d+ frame`)
		sent := grep(lines, `→ text .*"hello-ws"`)
		recv := grep(lines, `← text .*\[deflate\].*"echo:hello-ws"`)
		json2 := grep(lines, `← text .*"echo:\{\\"n\\":2,\\"text\\":\\"second message\\"\}"`)
		closeF := grep(lines, `→ close .*code=1000.*"bye"`)
		h.check(P, "ws <id>: frames in both directions, permessage-deflate inflated, close code",
			ok && len(sent) == 1 && len(recv) == 1 && len(json2) == 1 && len(closeF) == 1,
			"sent=%q recv=%q second=%q close=%q", first(sent), first(recv), first(json2), first(closeF))
	}

	// --- throttling + timing -------------------------------------------------------
	_, lines, ok = h.cmd(`rule add throttle url=/slow latency=700ms kbps=2000`, `^added rule #(\d+)`)
	ruleLine := first(grep(lines, "added|error"))
	h.cmd("open "+s.URL+"/slow", `^opened https://\S+/slow`)
	_, lines, ok2 = h.cmd("timing", `^\s*resources: `)
	ttfb := -1
	if mm := regexp.MustCompile(`ttfb (\d+)ms`).FindStringSubmatch(strings.Join(lines, "\n")); mm != nil {
		ttfb, _ = strconv.Atoi(mm[1])
	}
	fcp := grep(lines, `first-contentful-paint \d+ms`)
	h.check(P, "rule throttle latency=700ms + 'timing' shows ttfb >= 700ms (DevTools-style throttling)",
		ok && ok2 && ttfb >= 700 && len(fcp) == 1,
		"%s | ttfb=%dms | %s", ruleLine, ttfb, first(grep(lines, "navigation|error")))
	_, lines, ok = h.cmd("log 5", `/slow -> 200`)
	slowLine := first(grep(lines, `/slow -> 200`))
	h.check(P, "throttled request logged with the rule as intercept", ok && strings.Contains(slowLine, "intercepts=1"), "%s", slowLine)
	if mm := regexp.MustCompile(`#(\d+)`).FindStringSubmatch(ruleLine); mm != nil {
		h.cmd("rule del "+mm[1], `^deleted`)
	}
	timingFile := filepath.Join(auditDir, "timing.json")
	_, lines, ok = h.cmd("timing "+timingFile, `^saved timing JSON`)
	h.check(P, "timing <file.json> saves the raw timing data", ok && exists(timingFile), "%s", first(grep(lines, "saved|error")))
}
