package progress

import (
	"bytes"
	"io"
)

const maxDiagnosticLine = 64 * 1024

// Diagnostics frames complete lines before sanitization. The sanitizer receives
// false for a final EOF fragment, which may contain an interrupted URL credential.
// Oversized lines are omitted in full rather than streamed in unsafe fragments.
type Diagnostics struct {
	out      io.Writer
	sanitize func(string, bool) string
	pending  []byte
	dropping bool
	err      error
}

func (r *Reporter) Diagnostics(sanitize func(string, bool) string) *Diagnostics {
	return &Diagnostics{out: r.Writer(), sanitize: sanitize}
}

func (d *Diagnostics) Write(p []byte) (int, error) {
	consumed := 0
	for len(p) > 0 && d.err == nil {
		line, rest, complete := bytes.Cut(p, []byte{'\n'})
		consumed += len(line)
		if complete {
			consumed++
		}
		if !d.dropping {
			if len(d.pending)+len(line) > maxDiagnosticLine {
				d.pending = nil
				d.dropping = true
				_, d.err = io.WriteString(d.out, "gorn: omitted Go diagnostic line exceeding 64 KiB\n")
			} else {
				d.pending = append(d.pending, line...)
			}
		}
		if complete {
			if !d.dropping {
				d.flush(true)
			}
			d.dropping = false
		}
		p = rest
	}
	return consumed, d.err
}

func (d *Diagnostics) flush(newline bool) {
	text := d.sanitize(string(d.pending), newline)
	if newline {
		text += "\n"
	}
	_, d.err = io.WriteString(d.out, text)
	d.pending = d.pending[:0]
}

func (d *Diagnostics) Close() error {
	if d.err == nil && len(d.pending) > 0 && !d.dropping {
		d.flush(false)
	}
	return d.err
}
