package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/share026/web-cli/internal/ipc"
)

// buildHost compiles the real nm-host binary so tests exercise it exactly the
// way Chrome does: as a child process speaking length-prefixed JSON on stdio.
func buildHost(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "nm-host")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func TestBridgeLikeChrome(t *testing.T) {
	bin := buildHost(t)
	sock := filepath.Join(t.TempDir(), "ipc.sock")
	hub, err := ipc.Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	go hub.Serve()

	cmd := exec.Command(bin, "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/")
	cmd.Env = append(os.Environ(), "WEBCLI_SOCKET="+sock)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Chrome -> nm-host -> app: ping is answered by app with pong.
	if err := ipc.WriteNativeMessage(stdin, []byte(`{"id":"x1","type":"ping","payload":{"from":"extension"}}`)); err != nil {
		t.Fatal(err)
	}
	raw, err := ipc.ReadNativeMessage(stdout)
	if err != nil {
		t.Fatalf("read pong: %v (stderr: %s)", err, stderr.String())
	}
	var m ipc.Message
	json.Unmarshal(raw, &m)
	if m.Type != "pong" || m.ID != "x1" {
		t.Fatalf("got %s", raw)
	}
	t.Logf("browser received: %s", raw)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s, err := hub.WaitSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Hello.Origin != "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/" {
		t.Fatalf("hello origin %q", s.Hello.Origin)
	}

	// app -> nm-host -> Chrome: request/response correlation.
	go func() {
		raw, err := ipc.ReadNativeMessage(stdout)
		if err != nil {
			return
		}
		var req ipc.Message
		json.Unmarshal(raw, &req)
		reply, _ := json.Marshal(ipc.Message{ID: req.ID, Type: "elements", Payload: json.RawMessage(`{"elements":[{"hint":1}]}`)})
		ipc.WriteNativeMessage(stdin, reply)
	}()
	r, err := hub.Request(ctx, ipc.TypeCollect, nil)
	if err != nil || r.Type != "elements" {
		t.Fatalf("collect via bridge: %+v %v", r, err)
	}

	// A >1 MB message must never reach Chrome (it would drop the port): the
	// host answers the app with an error for that request ID instead, and the
	// bridge keeps working.
	events := make(chan ipc.Message, 1)
	hub.OnEvent = func(_ *ipc.Session, m ipc.Message) { events <- m }
	big, _ := ipc.NewMessage("big1", ipc.TypeDOM, map[string]string{"data": strings.Repeat("A", ipc.MaxToBrowser)})
	if err := s.Send(big); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-events:
		if m.ID != "big1" || m.Type != ipc.TypeError || !strings.Contains(m.Error, "1MB native messaging limit") {
			t.Fatalf("oversized reply: %+v", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no error reply for the oversized message")
	}
	if _, err := hub.Request(ctx, ipc.TypeCollect, map[string]string{"data": strings.Repeat("A", ipc.MaxToBrowser)}); !errors.Is(err, ipc.ErrTooLarge) {
		t.Fatalf("hub must refuse oversized requests locally, got %v", err)
	}
	go func() {
		raw, err := ipc.ReadNativeMessage(stdout)
		if err != nil {
			return
		}
		var req ipc.Message
		json.Unmarshal(raw, &req)
		reply, _ := json.Marshal(ipc.Message{ID: req.ID, Type: "pong"})
		ipc.WriteNativeMessage(stdin, reply)
	}()
	if _, err := hub.Request(ctx, ipc.TypePing, nil); err != nil {
		t.Fatalf("bridge broken after refusing a large message: %v", err)
	}

	// Chrome closing the port (stdin EOF) terminates the host cleanly.
	stdin.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("exit: %v", err)
		}
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		t.Fatal("nm-host did not exit after stdin EOF")
	}
	t.Logf("nm-host stderr:\n%s", stderr.String())
}

func TestAppNotRunning(t *testing.T) {
	bin := buildHost(t)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "WEBCLI_SOCKET="+filepath.Join(t.TempDir(), "absent.sock"))
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Start()
	ipc.WriteNativeMessage(stdin, []byte(`{"id":"p","type":"ping"}`))
	raw, err := ipc.ReadNativeMessage(stdout)
	if err != nil {
		t.Fatal(err)
	}
	var m ipc.Message
	json.Unmarshal(raw, &m)
	if m.Type != "error" || m.ID != "p" || !strings.Contains(m.Error, "not running") {
		t.Fatalf("got %s", raw)
	}
	io.Copy(io.Discard, stdout)
	cmd.Wait()
}

// TestSurvivesClosedStderr: Chrome passes its stderr to the host; if nobody
// reads it any more the host must keep working (found with CloakBrowser under
// chromedp, where the host was killed by SIGPIPE on its first log line).
func TestSurvivesClosedStderr(t *testing.T) {
	bin := buildHost(t)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	cmd := exec.Command(bin, "chrome-extension://aghlljmggamhjpkcngogikkiohnaaiha/")
	logFile := filepath.Join(t.TempDir(), "nm-host.log")
	cmd.Env = append(os.Environ(), "WEBCLI_SOCKET="+filepath.Join(t.TempDir(), "absent.sock"), "WEBCLI_NMHOST_LOG="+logFile)
	cmd.Stderr = w
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	w.Close()
	ipc.WriteNativeMessage(stdin, []byte(`{"id":"p","type":"ping"}`))
	raw, err := ipc.ReadNativeMessage(stdout)
	if err != nil {
		cmd.Wait()
		t.Fatalf("no reply (host died: %v): %v", cmd.ProcessState, err)
	}
	var m ipc.Message
	json.Unmarshal(raw, &m)
	if m.Type != "error" || m.ID != "p" {
		t.Fatalf("got %s", raw)
	}
	stdin.Close()
	io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil && strings.Contains(err.Error(), "broken pipe") {
		t.Fatalf("host killed by SIGPIPE: %v", err)
	}
	// the log file must still receive everything
	if b, _ := os.ReadFile(logFile); !strings.Contains(string(b), "nm-host start") {
		t.Fatalf("WEBCLI_NMHOST_LOG missing lines with dead stderr: %q", b)
	}
}
