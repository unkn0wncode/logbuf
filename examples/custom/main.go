package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wncode/logbuf"
)

var lb logbuf.LogBuf

func Debug(format string, a ...any) {
	msg := fmt.Sprintf("DEBUG: "+format, a...)
	_ = lb.WriteString(msg)
}

func Info(format string, a ...any) {
	msg := fmt.Sprintf("INFO: "+format, a...)
	_ = lb.WriteString(msg)
	fmt.Println(msg)
}

func main() {
	fp := filepath.Join(os.TempDir(), "custom-logbuf.db")
	var err error
	lb, err = logbuf.New(100, 0, fp)
	if err != nil {
		log.Fatalf("logbuf: %v", err)
	}
	defer lb.Clear()

	Debug("this is hidden from stdout")
	Info("visible and buffered – number=%d", 42)

	entries, _ := lb.Dump()
	fmt.Println("\nBuffered entries:\n" + strings.Join(entries, "\n"))
}
