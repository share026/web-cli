package main

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/share026/web-cli/internal/audit"
)

// cmdWS lists WebSocket connections seen by the proxy, or the frames of one.
//
//	ws                 connections: id, state, frame count, URL
//	ws <id> [n]        last n (default 50) frames of connection <id>
func (a *App) cmdWS(args []string) error {
	if len(args) == 0 {
		exs := a.store.Exchanges(func(e *audit.Exchange) bool { return e.Status == 101 })
		if len(exs) == 0 {
			fmt.Println("no WebSocket connections captured (the page must connect through the proxy)")
			return nil
		}
		for _, ex := range exs {
			state := "open"
			if ex.Duration > 0 || ex.Error != "" {
				state = "closed"
			}
			fmt.Printf("  #%-4d %-6s %4d frame(s)  %s\n", ex.ID, state, ex.WSFrameCount, ex.URL)
		}
		return nil
	}
	ex, err := a.exchange(args[0])
	if err != nil {
		return err
	}
	if ex.Status != 101 {
		return fmt.Errorf("#%d is not a WebSocket connection (status %d)", ex.ID, ex.Status)
	}
	n := 50
	if len(args) > 1 {
		if n, err = strconv.Atoi(args[1]); err != nil || n <= 0 {
			return fmt.Errorf("frame count must be a positive number")
		}
	}
	frames := ex.WSFrames
	if len(frames) > n {
		frames = frames[len(frames)-n:]
	}
	fmt.Printf("#%d %s — %d frame(s)", ex.ID, ex.URL, ex.WSFrameCount)
	if ex.WSFrameCount > len(ex.WSFrames) {
		fmt.Printf(" (first %d retained)", len(ex.WSFrames))
	}
	fmt.Println()
	for _, f := range frames {
		fmt.Println(frameLine(f))
	}
	return nil
}

func frameLine(f audit.WSFrame) string {
	arrow := "→"
	if f.Dir == "recv" {
		arrow = "←"
	}
	var flags []string
	if f.Compressed {
		flags = append(flags, "deflate")
	}
	if f.Truncated {
		flags = append(flags, "truncated")
	}
	if f.Note != "" {
		flags = append(flags, f.Note)
	}
	extra := ""
	if len(flags) > 0 {
		extra = " [" + strings.Join(flags, ", ") + "]"
	}
	return fmt.Sprintf("  %s %s %-6s %6dB%s  %s", f.Time.Format("15:04:05.000"), arrow, f.Opcode, f.Len, extra, framePreview(f.Data))
}

func framePreview(b []byte) string {
	const max = 300
	if utf8.Valid(b) {
		s := string(b)
		if len(s) > max {
			s = s[:max] + "…"
		}
		return strconv.Quote(s)
	}
	if len(b) > 48 {
		return "base64:" + base64.StdEncoding.EncodeToString(b[:48]) + "…"
	}
	return "base64:" + base64.StdEncoding.EncodeToString(b)
}
