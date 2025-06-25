package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wncode/logbuf"
)

// Define log levels because log package does not provide them.
const (
	LevelDebug = "DEBUG"
	LevelInfo  = " INFO"
	LevelWarn  = " WARN"
	LevelError = "ERROR"
)

// Create separate loggers with the same flags but with different writers:
//  - stdBufLogger writes to both stdout and the buffer using a multi-writer;
//  - bufOnlyLogger writes only to the buffer.
var (
	bufPath       = filepath.Join(os.TempDir(), "stdlog-logbuf.db")
	lb, _         = logbuf.New(50, 0, bufPath)
	multiWriter   = io.MultiWriter(os.Stdout, lb)
	stdBufLogger  = log.New(multiWriter, "", log.LstdFlags)
	bufOnlyLogger = log.New(lb, "", log.LstdFlags)
)

// logMsg decides where to send the log message based on the level.
// If the level is DEBUG, the message is only sent to the buffer.
// If the level is INFO or higher, the message is sent to both stdout and the buffer.
func logMsg(level, msg string, args ...any) {
	line := fmt.Sprintf(level+": "+msg, args...)
	if level != LevelDebug {
		stdBufLogger.Print(line)
		return
	}
	bufOnlyLogger.Print(line)
}

func main() {
	defer lb.Clear()

	logMsg(LevelDebug, "hidden in buffer only")
	logMsg(LevelInfo, "this line appears on stderr/stdout and is buffered as well")

	entries, _ := lb.Dump()
	logMsg(LevelInfo, "Buffered entries:\n%s", strings.Join(entries, "\n"))
}
