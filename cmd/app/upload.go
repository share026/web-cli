package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/share026/web-cli/internal/ipc"
)

const (
	// uploadChunk raw bytes per native message: a multiple of 3 so the base64
	// parts concatenate, and 512 KB of base64 keeps well under Chrome's 1 MB
	// host->browser limit.
	uploadChunk = 384 << 10
	// uploadMax total bytes: the extension hands the files to the page in one
	// runtime message (64 MB limit, base64 inflates by 4/3).
	uploadMax = 32 << 20
)

// cmdUpload sets the files of an <input type=file> (or drops them on a drop
// zone) without the OS file chooser.
func (a *App) cmdUpload(ctx context.Context, target string, paths []string) (DOMResult, error) {
	type file struct {
		name, mime string
		mod        int64
		data       []byte
	}
	var files []file
	total := 0
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return DOMResult{}, err
		}
		if st.IsDir() {
			return DOMResult{}, fmt.Errorf("%s is a directory", p)
		}
		if total += int(st.Size()); total > uploadMax {
			return DOMResult{}, fmt.Errorf("files total %d bytes; the limit is %d MB", total, uploadMax>>20)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return DOMResult{}, err
		}
		// File.type is a bare MIME type ("text/plain", not "...; charset=utf-8")
		mt, _, _ := strings.Cut(mime.TypeByExtension(strings.ToLower(filepath.Ext(p))), ";")
		if mt == "" {
			mt = "application/octet-stream"
		}
		files = append(files, file{filepath.Base(p), mt, st.ModTime().UnixMilli(), b})
	}
	id := ipc.NewID()
	for fi, f := range files {
		for idx := 0; ; idx++ { // an empty file still sends one (empty) chunk
			off := idx * uploadChunk
			end := min(off+uploadChunk, len(f.data))
			_, err := a.hub.Request(ctx, ipc.TypeUploadChunk, map[string]any{
				"upload_id": id, "file": fi, "name": f.name, "mime": f.mime, "last_modified": f.mod,
				"index": idx, "data": base64.StdEncoding.EncodeToString(f.data[off:end]),
			})
			if err != nil {
				return DOMResult{}, fmt.Errorf("sending %s: %w", f.name, err)
			}
			if end >= len(f.data) {
				break
			}
		}
	}
	return a.domOnTarget(ctx, "upload", target, map[string]any{"upload_id": id})
}

// Timing is the extension's reply to a timing request.
type Timing struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	Navigation *struct {
		Type             string  `json:"type"`
		Protocol         string  `json:"protocol"`
		Redirect         float64 `json:"redirect"`
		DNS              float64 `json:"dns"`
		Connect          float64 `json:"connect"`
		TLS              float64 `json:"tls"`
		TTFB             float64 `json:"ttfb"`
		Response         float64 `json:"response"`
		DOMInteractive   float64 `json:"dom_interactive"`
		DOMContentLoaded float64 `json:"dom_content_loaded"`
		Load             float64 `json:"load"`
		TransferSize     int64   `json:"transfer_size"`
		DecodedSize      int64   `json:"decoded_size"`
	} `json:"navigation"`
	FirstPaint float64 `json:"first_paint"`
	FCP        float64 `json:"fcp"`
	LCP        float64 `json:"lcp"`
	Resources  struct {
		Count        int   `json:"count"`
		TransferSize int64 `json:"transfer_size"`
		Slowest      []struct {
			URL          string  `json:"url"`
			Type         string  `json:"type"`
			Duration     float64 `json:"duration"`
			TransferSize int64   `json:"transfer_size"`
		} `json:"slowest"`
	} `json:"resources"`
}

func (a *App) cmdTiming(ctx context.Context, file string) error {
	r, err := a.hub.Request(ctx, ipc.TypeTiming, nil)
	if err != nil {
		return err
	}
	var t Timing
	if err := json.Unmarshal(r.Payload, &t); err != nil {
		return fmt.Errorf("decode timing: %w", err)
	}
	ms := func(v float64) string { return fmt.Sprintf("%.0fms", v) }
	fmt.Printf("timing of %s  %q\n", t.URL, t.Title)
	if n := t.Navigation; n != nil {
		fmt.Printf("  navigation %s over %s: redirect %s  dns %s  connect %s  tls %s  ttfb %s  download %s\n",
			n.Type, orDash(n.Protocol), ms(n.Redirect), ms(n.DNS), ms(n.Connect), ms(n.TLS), ms(n.TTFB), ms(n.Response))
		fmt.Printf("  dom-interactive %s  DOMContentLoaded %s  load %s  document %d B transferred (%d B decoded)\n",
			ms(n.DOMInteractive), ms(n.DOMContentLoaded), ms(n.Load), n.TransferSize, n.DecodedSize)
	}
	fmt.Printf("  first-paint %s  first-contentful-paint %s  largest-contentful-paint %s\n", ms(t.FirstPaint), ms(t.FCP), ms(t.LCP))
	fmt.Printf("  resources: %d (%d B transferred)\n", t.Resources.Count, t.Resources.TransferSize)
	for _, s := range t.Resources.Slowest {
		fmt.Printf("    %7s  %-14s %8d B  %s\n", ms(s.Duration), s.Type, s.TransferSize, clip(s.URL, 120))
	}
	if file != "" {
		js, _ := json.MarshalIndent(json.RawMessage(r.Payload), "", "  ")
		return writeFile(file, append(js, '\n'), "timing JSON")
	}
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
