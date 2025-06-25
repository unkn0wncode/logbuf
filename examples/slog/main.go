package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wncode/logbuf"
)

// bufHandler writes every record to LogBuf and forwards INFO+ to stdout.
type bufHandler struct {
	buf logbuf.Buffer
}

func (h bufHandler) Enabled(_ context.Context, level slog.Level) bool { return true }

func (h bufHandler) Handle(_ context.Context, r slog.Record) error {
	// Write to buffer
	_ = h.buf.WriteString(r.Message)
	if r.Level >= slog.LevelInfo {
		fmt.Println(r.Message)
	}
	return nil
}

func (h bufHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h bufHandler) WithGroup(name string) slog.Handler       { return h }

func main() {
	fp := filepath.Join(os.TempDir(), "slog-logbuf.db")
	lbIface, err := logbuf.NewSQliteBuffer(500, 48*time.Hour, fp)
	if err != nil {
		log.Fatalf("logbuf: %v", err)
	}
	defer lbIface.Clear()

	slog.SetDefault(slog.New(bufHandler{buf: lbIface}))

	slog.Debug("hidden in buffer only")
	slog.Info("hello world")

	entries, _ := lbIface.Dump()
	fmt.Println("Buffered entries:\n" + strings.Join(entries, "\n"))
}
