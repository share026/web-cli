// Package ipc implements the two wire formats used between the browser and the
// resident CLI:
//
//  1. Chrome Native Messaging framing (stdin/stdout of nm-host):
//     a 4-byte message length in *native* byte order followed by UTF-8 JSON.
//  2. The app <-> nm-host Unix Domain Socket protocol: newline-delimited JSON
//     (NDJSON) carrying Message envelopes.
package ipc

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	// MaxToBrowser is the maximum size of a single message sent from the
	// native host to Chrome (1 MB, per Chrome documentation).
	MaxToBrowser = 1 << 20
	// MaxFromBrowser is the maximum size of a single message Chrome sends to
	// the native host (64 MiB, per Chrome documentation).
	MaxFromBrowser = 64 << 20
)

// ErrTooLarge is returned when a frame exceeds the protocol limits.
var ErrTooLarge = errors.New("native message exceeds size limit")

// ReadNativeMessage reads one length-prefixed native messaging frame.
// It returns io.EOF when the stream is closed cleanly between frames
// (which is how Chrome signals that the port was disconnected).
func ReadNativeMessage(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("truncated native message header: %w", err)
		}
		return nil, err
	}
	n := binary.NativeEndian.Uint32(hdr[:])
	if n > MaxFromBrowser {
		return nil, fmt.Errorf("%w: %d bytes", ErrTooLarge, n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("truncated native message body (%d bytes expected): %w", n, err)
	}
	if !json.Valid(buf) {
		return nil, fmt.Errorf("native message is not valid JSON")
	}
	return buf, nil
}

// WriteNativeMessage writes one length-prefixed native messaging frame.
// The header and body are written with a single Write call so that
// concurrent writers guarded by a mutex never interleave partial frames.
func WriteNativeMessage(w io.Writer, msg []byte) error {
	if len(msg) > MaxToBrowser {
		return fmt.Errorf("%w: %d bytes (max %d to browser)", ErrTooLarge, len(msg), MaxToBrowser)
	}
	frame := make([]byte, 4+len(msg))
	binary.NativeEndian.PutUint32(frame[:4], uint32(len(msg)))
	copy(frame[4:], msg)
	_, err := w.Write(frame)
	return err
}
