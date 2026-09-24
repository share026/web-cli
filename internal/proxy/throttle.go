package proxy

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type throttle struct {
	latency time.Duration
	kbps    int
	note    string
}

// throttleFor combines matching throttle rules: the largest latency and the
// smallest bandwidth win.
func throttleFor(rules []*Rule) throttle {
	var t throttle
	var notes []string
	for _, r := range rules {
		if r.Action != ActThrottle {
			continue
		}
		if d := time.Duration(r.LatencyMS) * time.Millisecond; d > t.latency {
			t.latency = d
		}
		if r.Kbps > 0 && (t.kbps == 0 || r.Kbps < t.kbps) {
			t.kbps = r.Kbps
		}
		notes = append(notes, r.String())
	}
	t.note = strings.Join(notes, "; ")
	return t
}

// parseMillis accepts "250", "250ms" or any time.ParseDuration string.
func parseMillis(v string) (int, error) {
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return n, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("expected milliseconds or a duration like 300ms, got %q", v)
	}
	return int(d / time.Millisecond), nil
}

// throttledBody streams at most bps bytes per second. Reads are cut into
// ~50ms slices so the pacing is smooth rather than bursty.
type throttledBody struct {
	rc    io.ReadCloser
	bps   int
	start time.Time
	sent  int64
}

func (t *throttledBody) Read(p []byte) (int, error) {
	if t.start.IsZero() {
		t.start = time.Now()
	}
	if chunk := max(t.bps/20, 1); len(p) > chunk {
		p = p[:chunk]
	}
	n, err := t.rc.Read(p)
	t.sent += int64(n)
	due := t.start.Add(time.Duration(float64(t.sent) / float64(t.bps) * float64(time.Second)))
	if wait := time.Until(due); wait > 0 {
		time.Sleep(wait)
	}
	return n, err
}

func (t *throttledBody) Close() error { return t.rc.Close() }
