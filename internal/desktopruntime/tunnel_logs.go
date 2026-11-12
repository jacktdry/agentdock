package desktopruntime

import (
	"io"
	"strings"
)

// Buffer lines so a token split across cloudflared writes cannot enter logs.
// Oversized or unfinished lines are discarded rather than exposing fragments.
type tunnelSafeLogWriter struct {
	output   io.Writer
	token    string
	pending  []byte
	dropping bool
}

func (w *tunnelSafeLogWriter) Write(data []byte) (int, error) {
	for _, b := range data {
		if b == '\n' {
			if !w.dropping {
				line := strings.ReplaceAll(string(w.pending), w.token, "[redacted]")
				if _, err := io.WriteString(w.output, line+"\n"); err != nil {
					return 0, err
				}
			}
			w.pending = w.pending[:0]
			w.dropping = false
		} else if !w.dropping {
			w.pending = append(w.pending, b)
			if len(w.pending) > 64*1024 {
				w.pending = nil
				w.dropping = true
			}
		}
	}
	return len(data), nil
}

var _ io.Writer = (*tunnelSafeLogWriter)(nil)
