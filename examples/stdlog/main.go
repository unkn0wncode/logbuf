package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wncode/logbuf"
)

// levelFilterWriter writes every log line to buf. If the line does *not* start
// with "DEBUG" it is forwarded to out as well. This keeps debug-level noise
// hidden from the console while still being persisted in the buffer.
type levelFilterWriter struct {
	buf io.Writer // receives every message
	out io.Writer // receives INFO+ messages
}

func (w levelFilterWriter) Write(p []byte) (int, error) {
	// Always write to buffer first.
	if _, err := w.buf.Write(p); err != nil {
		return 0, err
	}

	// Forward anything that is not a DEBUG line to the additional writer.
	trimmed := bytes.TrimSpace(p)
	if !bytes.HasPrefix(trimmed, []byte("DEBUG")) {
		if _, err := w.out.Write(p); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func main() {
	fp := filepath.Join(os.TempDir(), "stdlog-logbuf.db")
	lb, err := logbuf.New(50, 0, fp)
	if err != nil {
		log.Fatalf("logbuf: %v", err)
	}
	defer lb.Clear()

	// Disable default timestamp/date prefixes so log lines start directly with
	// the log level keyword. This makes filtering straightforward.
	log.SetFlags(0)

	// Use a custom writer that always appends to the buffer while filtering
	// DEBUG-level lines from the console.
	log.SetOutput(levelFilterWriter{buf: lb, out: log.Writer()})

	log.Println("DEBUG: hidden in buffer only")
	log.Println("INFO: this line appears on stderr/stdout and is buffered as well")

	entries, _ := lb.Dump()
	log.Printf("Buffered entries:\n%s", strings.Join(entries, "\n"))
}
