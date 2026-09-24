package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/share026/web-cli/internal/ipc"
)

// recorder writes a replayable web-cli script (one command per line) from
//   - real user input in the browser (trusted click/change/Enter events,
//     reported by content.js as record_event), and
//   - page commands typed into web-cli itself.
//
// Password fields are written as $WEBCLI_PASSWORD unless recording was
// started with "secrets", so recordings can be shared and committed.
type recorder struct {
	mu      sync.Mutex
	f       *os.File
	path    string
	steps   int
	secrets bool
}

func (r *recorder) active() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.f != nil
}

func (r *recorder) start(path string, secrets bool, url string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		return fmt.Errorf("already recording to %s", r.path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	r.f, r.path, r.steps, r.secrets = f, path, 0, secrets
	fmt.Fprintf(f, "# web-cli recording %s\n# replay: bin/app -f %s   (or 'run %s' in the REPL)\n", time.Now().Format(time.RFC3339), filepath.Base(path), filepath.Base(path))
	if !secrets {
		fmt.Fprintln(f, "# password fields are recorded as $WEBCLI_PASSWORD: export it before replaying")
	}
	if url != "" && !strings.HasPrefix(url, "chrome") && !strings.HasPrefix(url, "about:") {
		fmt.Fprintf(f, "open %s\n", quoteArg(url))
		r.steps++
	}
	return nil
}

func (r *recorder) stop() (string, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return "", 0, fmt.Errorf("not recording")
	}
	err := r.f.Close()
	path, n := r.path, r.steps
	r.f = nil
	return path, n, err
}

func (r *recorder) writeCommand(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return
	}
	fmt.Fprintln(r.f, line)
	r.steps++
}

// RecordEvent is one user action reported by content.js.
type RecordEvent struct {
	Op       string `json:"op"`
	Selector string `json:"selector"`
	Tag      string `json:"tag"`
	Type     string `json:"type"`
	Text     string `json:"text"`
	URL      string `json:"url"`
	Value    string `json:"value"`
	Secret   bool   `json:"secret"`
	Key      string `json:"key"`
}

// lines converts a browser event into script lines (a comment + a command).
func (e RecordEvent) lines(secrets bool) []string {
	target := quoteArg("css=" + e.Selector)
	el := e.Tag
	if e.Type != "" {
		el += ":" + e.Type
	}
	comment := fmt.Sprintf("# %s <%s> %q on %s", e.Op, el, e.Text, e.URL)
	var cmd string
	switch e.Op {
	case "click":
		cmd = "click " + target
	case "check", "uncheck":
		cmd = e.Op + " " + target
	case "select":
		cmd = "select " + target + " " + quoteArg(e.Value)
	case "type":
		if e.Secret && (!secrets || e.Value == "") {
			cmd = "type " + target + " $WEBCLI_PASSWORD"
		} else {
			cmd = "type " + target + " " + quoteArg(e.Value)
		}
	case "press":
		cmd = "press " + e.Key + " " + target
	default:
		return nil
	}
	return []string{comment, cmd}
}

func (r *recorder) event(e RecordEvent) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return "", false
	}
	ls := e.lines(r.secrets)
	if ls == nil {
		return "", false
	}
	for _, l := range ls {
		fmt.Fprintln(r.f, l)
	}
	r.steps++
	return ls[len(ls)-1], true
}

// onRecordEvent is called from the IPC goroutine for record_event messages.
func (a *App) onRecordEvent(m ipc.Message) {
	var e RecordEvent
	if err := json.Unmarshal(m.Payload, &e); err != nil {
		a.log.Printf("[record] bad event: %v", err)
		return
	}
	if line, ok := a.rec.event(e); ok {
		a.log.Printf("[record] %s", line)
	}
}

func (a *App) cmdRecord(ctx context.Context, args []string) error {
	if len(args) == 0 {
		args = []string{"status"}
	}
	switch args[0] {
	case "start":
		file := filepath.Join(a.cfg.outDir, "recording-"+time.Now().Format("20060102-150405")+".webcli")
		secrets := false
		for _, arg := range args[1:] {
			if arg == "secrets" {
				secrets = true
			} else {
				file = arg
			}
		}
		r, err := a.hub.Request(ctx, ipc.TypeRecord, map[string]any{"on": true, "secrets": secrets})
		if err != nil {
			return err
		}
		var res struct {
			URL string `json:"url"`
		}
		json.Unmarshal(r.Payload, &res)
		if err := a.rec.start(file, secrets, res.URL); err != nil {
			a.hub.Request(ctx, ipc.TypeRecord, map[string]any{"on": false})
			return err
		}
		fmt.Printf("recording to %s (starting at %s): use the browser normally and/or web-cli commands; 'record stop' ends it\n", file, res.URL)
	case "stop":
		_, err := a.hub.Request(ctx, ipc.TypeRecord, map[string]any{"on": false})
		path, n, serr := a.rec.stop()
		if serr != nil {
			return serr
		}
		fmt.Printf("recording stopped: %d step(s) written to %s\n", n, path)
		if err != nil {
			fmt.Printf("  warning: could not tell the extension to stop: %v\n", err)
		}
	case "status":
		if a.rec.active() {
			a.rec.mu.Lock()
			fmt.Printf("recording to %s (%d step(s) so far)\n", a.rec.path, a.rec.steps)
			a.rec.mu.Unlock()
		} else {
			fmt.Println("not recording")
		}
	default:
		return fmt.Errorf("usage: record start [file] [secrets] | record stop | record status")
	}
	return nil
}

// runScript executes a web-cli script: one command per line, '#' comments,
// $VAR expansion, stopping at the first failing command.
func (a *App) runScript(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n, steps := 0, 0
	start := time.Now()
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fmt.Printf("> %s\n", line)
		quit, err := a.exec(line)
		if err != nil {
			return fmt.Errorf("%s:%d: %s: %w", path, n, line, err)
		}
		steps++
		if quit {
			break
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	fmt.Printf("run %s: %d step(s) OK in %s\n", path, steps, time.Since(start).Round(time.Millisecond))
	return nil
}
