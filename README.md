# logbuf – SQLite-backed transient log buffer for Go

`logbuf` is a small Go library that provides an in-process buffer for raw log
lines.  The buffer is persisted in a light-weight SQLite database (optionally
in memory) and automatically trims itself according to either of two
constraints you choose:

* **maxLines** – keep only the newest *n* entries, or
* **maxAge**   – keep entries younger than *d* (e.g. 5 minutes).

You can enable one constraint, the other, or both at the same time.

---

## Why?

When investigating problems it is often handy to keep a short history of *all*
log messages – including debug-level noise – while still forwarding only higher
levels to stdout/stderr or another logging sink. A transient SQLite database
fits that purpose:

* zero external dependencies (uses CGO-enabled
  [`github.com/mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3)),
* automatically trimmed through SQL triggers (no extra Go code runs),
* crash-safe thanks to WAL mode,
* cheap to query on demand.

---

## Installation

```bash
go get github.com/unkn0wncode/logbuf
```

The package requires Go ≥ 1.21 and a CGO-enabled build environment because the
SQLite driver depends on `libsqlite3`.

---

## Quick start

Example programs illustrating integrations with `slog`, the standard `log`
package, and a custom `fmt.Printf`-based logger can be found under
[`examples/`](./examples). Run them with:

```bash
go run ./examples/slog
go run ./examples/stdlog
go run ./examples/custom
```

Each example forwards only a subset of messages to standard output while all
records are kept in the buffer and can be dumped on demand.

---

## API overview

```go
func NewSQliteBuffer(maxLines int, maxAge time.Duration, dbPath string) (logbuf.Buffer, error)
```

* If **both** `maxLines` and `maxAge` are 0, `New` returns an error.
* Pass `":memory:"` for `dbPath` if you want the database to live only in RAM.

The returned `logbuf.Buffer` interface provides:

* `Write(entry []byte) (int, error)` – append a single entry (binary-safe).
* `WriteString(entry string) error` – convenience wrapper accepting UTF-8 strings.
* `Dump() ([]string, error)` – fetch *all* entries (oldest first).
* `Close()` – close the underlying `*sql.DB`.
* `Clear() error` – wipe the database file and start fresh.

All methods are safe for concurrent use.

---

## Testing

The package ships with unit tests:

```bash
go test ./...
```

They cover constructor validation, round-tripping, retention logic and `Clear`.

---

## Contributing

Issues and pull requests are welcome! Feel free to open tickets for bugs or
feature suggestions.

---

## License

Distributed under the MIT license – see `LICENSE` file for details. 
