// Package logbuf provides a buffer for temporary storage of unfiltered log entries.
//
// The buffer implementation is backed by a small SQLite database stored at a configurable path.
// Log entries can be appended using either Write/WriteString. Buffer implements io.Writer.
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
	"io"
)

// Interface checks.
var (
	_ io.Writer = (Buffer)(nil)
)

// Buffer interface describes a temporary storage for unfiltered log entries.
type Buffer interface {
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
