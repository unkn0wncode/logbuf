// Package logbuf provides a buffer for temporary storage of unfiltered log entries.
//
// The buffer is backed by a small SQLite database stored at a configurable path.
// Log entries can be appended using either Write/WriteString. LogBuf implements io.Writer.
// When maxLines > 0 only the newest maxLines entries are kept.
// When maxAge > 0 any entry older than maxAge is discarded the moment a new entry is written.
// Either constraint can be disabled by passing 0 for the respective argument, but at least one
// must be non-zero.
//
// All methods are safe for concurrent use by multiple goroutines.
// Close must be called when the buffer is no longer required to flush temporary data to disk.
// The Clear method removes all currently persisted entries and deletes the on-disk database.
// The buffer object stays usable and will recreate the database on the next operation.
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
	_ io.Writer = (LogBuf)(nil)
	_ io.Writer = (*sqliteBuf)(nil)
	_ LogBuf    = (*sqliteBuf)(nil)
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

// LogBuf interface describes temporary storage for unfiltered log entries.
type LogBuf interface {
	io.Writer // Write(p []byte) (int, error)

	// WriteString is a convenience wrapper around Write for UTF-8 strings.
	WriteString(entry string) error

	// Dump returns all currently buffered log entries ordered by their time of
	// insertion – oldest first.
	Dump() ([]string, error)

	// Close closes the underlying database connection.
	Close()

	// Clear deletes the on-disk database. The buffer becomes empty and the
	// next call to Write or Dump recreates a fresh database automatically.
	Clear() error
}

// sqliteBuf implements LogBuf.
type sqliteBuf struct {
	db       *sql.DB
	dbPath   string
	maxLines int
	maxAge   time.Duration
	mu       sync.RWMutex
}

// New returns an SQLite-backed LogBuf. At least one of maxLines or maxAge must
// be non-zero, otherwise an error is returned.
func New(maxLines int, maxAge time.Duration, dbPath string) (LogBuf, error) {
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
