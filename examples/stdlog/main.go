package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wncode/logbuf"
)

func main() {
	fp := filepath.Join(os.TempDir(), "stdlog-logbuf.db")
	lb, err := logbuf.New(50, 0, fp)
	if err != nil {
		log.Fatalf("logbuf: %v", err)
	}
	defer lb.Clear()

	// Forward logs to stdout *and* buffer.
	mw := io.MultiWriter(os.Stdout, lb)
	log.SetOutput(mw)

	log.Println("this line appears on stdout and is buffered as well")

	entries, _ := lb.Dump()
	log.Printf("Buffered entries:\n%s", strings.Join(entries, "\n"))
}
