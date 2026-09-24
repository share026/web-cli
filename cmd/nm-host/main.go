// Command nm-host is the Chrome Native Messaging host for web-cli.
//
// Chrome starts this process when the extension calls
// chrome.runtime.connectNative("com.share026.webcli"). Chrome writes
// length-prefixed JSON frames to our stdin and reads frames from our stdout.
// nm-host is a transparent bridge: every frame is relayed as one NDJSON line to
// the resident `app` over a Unix Domain Socket, and every line from `app` is
// framed back to Chrome. It never interprets application messages, except for
// answering with an error when `app` is unreachable.
//
// stdout is reserved for the protocol; diagnostics go to stderr (Chrome
// forwards it to its own log) and optionally to $WEBCLI_NMHOST_LOG.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/share026/web-cli/internal/ipc"
)

func main() {
	// Chrome hands its own stderr to native hosts. When that is a pipe nobody
	// reads any more (e.g. a browser launched by an automation tool), Go's
	// default SIGPIPE behaviour for fd 2 would kill the host on its first log
	// line. Ignoring SIGPIPE turns that into a harmless EPIPE on stderr; a
	// broken stdout still ends the bridge through the normal write error.
	signal.Ignore(syscall.SIGPIPE)
	logger := newLogger()
	origin := ""
	for _, a := range os.Args[1:] {
		if strings.HasPrefix(a, "chrome-extension://") {
			origin = a
		}
	}
	logger.Printf("nm-host start pid=%d origin=%q args=%q", os.Getpid(), origin, os.Args[1:])
	os.Exit(run(os.Stdin, os.Stdout, socketPath(), origin, logger))
}

func socketPath() string {
	if p := os.Getenv("WEBCLI_SOCKET"); p != "" {
		return p
	}
	return ipc.DefaultSocketPath
}

func newLogger() *log.Logger {
	var w io.Writer = os.Stderr
	if p := os.Getenv("WEBCLI_NMHOST_LOG"); p != "" {
		if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			w = io.MultiWriter(os.Stderr, f)
		}
	}
	return log.New(w, "[nm-host] ", log.LstdFlags|log.Lmicroseconds)
}

// browserWriter serialises frames written to Chrome.
type browserWriter struct {
	mu sync.Mutex
	w  *bufio.Writer
}

func (b *browserWriter) write(raw []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ipc.WriteNativeMessage(b.w, raw); err != nil {
		return err
	}
	return b.w.Flush()
}

func (b *browserWriter) writeError(id, msg string) {
	raw, _ := json.Marshal(ipc.Message{ID: id, Type: ipc.TypeError, Error: msg})
	_ = b.write(raw)
}

func run(stdin io.Reader, stdout io.Writer, sock, origin string, logger *log.Logger) int {
	bw := &browserWriter{w: bufio.NewWriter(stdout)}

	conn, err := ipc.Dial(sock, 5*time.Second)
	if err != nil {
		logger.Printf("cannot reach app: %v", err)
		// Tell the extension why, answering the first request if any.
		id := ""
		if raw, rerr := ipc.ReadNativeMessage(stdin); rerr == nil {
			var m ipc.Message
			_ = json.Unmarshal(raw, &m)
			id = m.ID
		}
		bw.writeError(id, fmt.Sprintf("web-cli app is not running (socket %s): %v", sock, err))
		return 1
	}
	defer conn.Close()
	hello, _ := ipc.NewMessage("", ipc.TypeHello, ipc.Hello{Origin: origin, PID: os.Getpid()})
	if err := conn.Send(hello); err != nil {
		logger.Printf("send hello: %v", err)
		return 1
	}
	logger.Printf("connected to app at %s", sock)

	errc := make(chan error, 2)

	// Chrome -> app
	go func() {
		for {
			raw, err := ipc.ReadNativeMessage(stdin)
			if err != nil {
				if errors.Is(err, io.EOF) {
					errc <- errors.New("browser closed stdin (port disconnected)")
				} else {
					errc <- fmt.Errorf("read from browser: %w", err)
				}
				return
			}
			if err := conn.SendRaw(raw); err != nil {
				errc <- fmt.Errorf("write to app: %w", err)
				return
			}
		}
	}()

	// app -> Chrome
	go func() {
		for {
			raw, err := conn.RecvRaw()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					errc <- errors.New("app closed the socket")
				} else {
					errc <- fmt.Errorf("read from app: %w", err)
				}
				return
			}
			if len(raw) > ipc.MaxToBrowser {
				var m ipc.Message
				_ = json.Unmarshal(raw, &m)
				bw.writeError(m.ID, fmt.Sprintf("message of %d bytes exceeds the 1MB native messaging limit", len(raw)))
				continue
			}
			if err := bw.write(raw); err != nil {
				errc <- fmt.Errorf("write to browser: %w", err)
				return
			}
		}
	}()

	err = <-errc
	logger.Printf("bridge stopped: %v", err)
	return 0
}
