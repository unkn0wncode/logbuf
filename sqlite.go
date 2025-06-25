// Package logbuf / sqlite.go contains an SQLite-based implementation of the Buffer interface.
package logbuf

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	writeStmt = `INSERT INTO log(entry) VALUES (?);`
	dumpStmt  = `SELECT entry FROM log ORDER BY timestamp;`
)

// Interface checks.
var (
	_ io.Writer = (*sqliteBuf)(nil)
	_ Buffer    = (*sqliteBuf)(nil)
)

var setupSQL = []string{
	`PRAGMA journal_mode = WAL;`,

	`CREATE TABLE IF NOT EXISTS log (
        timestamp INTEGER NOT NULL DEFAULT (
            CAST((julianday('now') - 2440587.5) * 86400000000000 AS INTEGER)
        ),
        entry BLOB NOT NULL
    );`,

	`CREATE TABLE IF NOT EXISTS log_settings (
        id        INTEGER PRIMARY KEY CHECK(id = 1),
        maxAge_ns INTEGER NOT NULL,
        maxLines  INTEGER NOT NULL
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
               SELECT rowid FROM log ORDER BY timestamp DESC LIMIT (SELECT maxLines FROM log_settings WHERE id = 1)
           );
     END;`,
}

// sqliteBuf implements LogBuf.
type sqliteBuf struct {
	db       *sql.DB
	dbPath   string
	maxLines int
	maxAge   time.Duration
	mu       sync.RWMutex
}

// NewSQliteBuffer returns an SQLite-backed logbuf.Buffer. At least one of maxLines or
// maxAge must be non-zero, otherwise an error is returned.
func NewSQliteBuffer(maxLines int, maxAge time.Duration, dbPath string) (Buffer, error) {
	if maxLines == 0 && maxAge == 0 {
		return nil, fmt.Errorf("logbuf: at least one of maxLines or maxAge must be non-zero")
	}

	lb := &sqliteBuf{
		dbPath:   dbPath,
		maxLines: maxLines,
		maxAge:   maxAge,
	}
	if err := lb.open(); err != nil {
		return nil, err
	}
	return lb, nil
}

// open (re)creates the underlying database connection and executes all setup
// SQL including the parameterisation of the retention trigger.
func (b *sqliteBuf) open() error {
	db, err := sql.Open("sqlite3", b.dbPath)
	if err != nil {
		return err
	}

	// In case an early statement fails ensure the handle is closed to avoid
	// connection leaks.
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	for _, stmt := range setupSQL {
		if _, err = db.Exec(stmt); err != nil {
			return err
		}
	}

	if _, err = db.Exec(
		`INSERT OR REPLACE INTO log_settings(id, maxAge_ns, maxLines) VALUES (1, ?, ?);`,
		b.maxAge.Nanoseconds(),
		b.maxLines,
	); err != nil {
		return err
	}

	b.db = db
	return nil
}

// Write implements LogBuf and io.Writer.
// Inserts the provided bytes as a single entry into the buffer.
func (b *sqliteBuf) Write(p []byte) (int, error) {
	// First lock DB to prevent Close and try to write.
	// If DB is missing run ensureOpen and try again.
	for {
		b.mu.RLock()
		db := b.db
		if db != nil {
			_, err := db.Exec(writeStmt, p)
			b.mu.RUnlock()
			if err != nil {
				return 0, err
			}
			return len(p), nil
		}
		b.mu.RUnlock()

		if err := b.ensureOpen(); err != nil {
			return 0, err
		}
	}
}

// WriteString implements LogBuf.
// Uses Write by converting the string to bytes.
func (b *sqliteBuf) WriteString(entry string) error {
	_, err := b.Write([]byte(entry))
	return err
}

// Dump implements LogBuf.
// Returns all currently buffered log entries ordered by their time of insertion, oldest first.
func (b *sqliteBuf) Dump() ([]string, error) {
	// First lock DB to prevent Close and try to dump.
	// If DB is missing run ensureOpen and try again.
	var rows *sql.Rows
	var err error
	for {
		b.mu.RLock()
		db := b.db
		if db != nil {
			rows, err = db.Query(dumpStmt)
			if err != nil {
				b.mu.RUnlock()
				return nil, err
			}
			b.mu.RUnlock()

			defer rows.Close()

			break
		}
		b.mu.RUnlock()

		if err := b.ensureOpen(); err != nil {
			return nil, err
		}
	}

	if rows == nil {
		return nil, nil
	}

	var entries []string
	for rows.Next() {
		var e []byte
		if err = rows.Scan(&e); err != nil {
			return nil, err
		}
		entries = append(entries, strings.TrimSpace(string(e)))
	}

	return entries, rows.Err()
}

// Close closes the underlying database. It is safe to call multiple times.
func (b *sqliteBuf) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.db != nil {
		_ = b.db.Close()
		b.db = nil
	}
}

// Clear removes the on-disk database and recreates an empty buffer so that the
// object can continue to be used.
func (b *sqliteBuf) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.db != nil {
		_ = b.db.Close()
		b.db = nil
	}

	if b.dbPath != ":memory:" {
		_ = os.Remove(b.dbPath)
	}

	return nil
}

// ensureOpen lazily opens the database if it is not already open.
func (b *sqliteBuf) ensureOpen() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.db != nil {
		return nil
	}
	return b.open()
}
