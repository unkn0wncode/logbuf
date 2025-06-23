package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	maxLines = 10
	maxAge   = 5 * time.Second

	writeSmt = `INSERT INTO log(entry) VALUES (?);`
	dumpSmt  = `SELECT entry FROM log ORDER BY timestamp;`
)

var (
	db *sql.DB
)

var setupStatements = []string{
	`PRAGMA journal_mode = WAL;`,

	`CREATE TABLE IF NOT EXISTS log (
		timestamp INTEGER NOT NULL DEFAULT (
			CAST( (julianday('now') - 2440587.5) * 86400000000000 AS INTEGER)
		),
		entry      BLOB    NOT NULL
	);`,

	`CREATE TABLE IF NOT EXISTS log_settings (
		id         INTEGER PRIMARY KEY CHECK(id = 1),
		maxAge_ns  INTEGER NOT NULL,
		maxLines   INTEGER NOT NULL
	);`,

	`CREATE INDEX IF NOT EXISTS idx_log_ts ON log(timestamp);`,

	`CREATE TRIGGER IF NOT EXISTS log_trim
	AFTER INSERT ON log
	BEGIN
	    DELETE FROM log
	    WHERE (SELECT maxAge_ns FROM log_settings WHERE id = 1) > 0
	      AND timestamp < NEW.timestamp - (SELECT maxAge_ns FROM log_settings WHERE id = 1);

	    DELETE FROM log
	    WHERE (SELECT maxLines FROM log_settings WHERE id = 1) > 0
	      AND rowid NOT IN (
	          SELECT rowid
	          FROM   log
	          ORDER  BY timestamp DESC
	          LIMIT  (SELECT maxLines FROM log_settings WHERE id = 1)
	      );
	END;`,
}

func setup() error {
	var err error
	db, err = sql.Open("sqlite3", "./logs.db")
	if err != nil {
		return err
	}

	for _, stmt := range setupStatements {
		_, err = db.Exec(stmt)
		if err != nil {
			return err
		}
	}

	_, err = db.Exec(`INSERT OR REPLACE INTO log_settings (id, maxAge_ns, maxLines) VALUES (1, ?, ?)`, maxAge.Nanoseconds(), maxLines)
	if err != nil {
		return err
	}

	return nil
}

func write(entry string) error {
	_, err := db.Exec(writeSmt, entry)
	return err
}

func dump() ([]string, error) {
	rows, err := db.Query(dumpSmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []string{}
	for rows.Next() {
		var entry string
		err = rows.Scan(&entry)
		if err != nil {
			return nil, err
		}
		entries = append(entries, strings.TrimSpace(entry))
	}

	return entries, nil
}

// sqliteStdHandler is a slog.Handler that writes every record to the SQLite
// buffer via the write function and forwards records at or above stdoutLevel
// to a wrapped stdoutHandler.
type sqliteStdHandler struct {
	stdoutLevel slog.Level
}

func (h *sqliteStdHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Always return true so that Handle receives every record.
	return true
}

func (h *sqliteStdHandler) Handle(ctx context.Context, r slog.Record) error {
	// Format timestamp with millisecond precision.
	ts := r.Time.Format("2006-01-02 15:04:05.000000")

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s", ts, r.Level, r.Message)

	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})

	line := b.String()
	// Always attempt to write to SQLite; ignore error to avoid log loop.
	_ = write(line)

	// Forward to stdout if level is high enough.
	if r.Level >= h.stdoutLevel {
		fmt.Println(line)
	}
	return nil
}

func (h *sqliteStdHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Attribute attachment is handled by slog; we can return same handler.
	return h
}

func (h *sqliteStdHandler) WithGroup(name string) slog.Handler {
	return h
}

func main() {
	log.Default().Printf("Starting...")
	if err := setup(); err != nil {
		panic(err)
	}
	defer db.Close()
	log.Default().Printf("Setup complete")

	// Root handler writes every log entry to SQLite and prints entries whose
	// level is INFO or higher to stdout, using identical plain-text format.
	rootHandler := &sqliteStdHandler{
		stdoutLevel: slog.LevelInfo,
	}
	logger := slog.New(rootHandler)
	slog.SetDefault(logger)

	// Emit log records of mixed levels.
	slog.Debug("this debug entry should NOT appear on stdout, but IS saved to DB")
	slog.Info("user logged in", "user", "alice")
	slog.Debug("request received", "method", "GET", "path", "/api/v1/users")
	slog.Warn("disk space running low", "path", "/var")
	slog.Debug("request executed", "method", "GET", "path", "/api/v1/users", "status", 200)
	slog.Error("failed to send email", "id", 42)

	log.Default().Printf("Wrote slog entries")

	log.Default().Printf("Dumping entries from SQLite buffer")
	entries, err := dump()
	if err != nil {
		panic(err)
	}
	fmt.Printf("\nUnfiltered log entries:\n%v\n\n", strings.Join(entries, "\n"))

	log.Default().Printf("Done")
}
