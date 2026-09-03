// Package checkpoint persists per-record submission outcomes to a CSV file
// so a run can be resumed or audited later.
package checkpoint

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one row of the checkpoint CSV.
type Entry struct {
	Index     int
	Sheet     string
	Row       int
	UHID      string
	Status    string // "success" or "failed"
	Timestamp string
	Error     string
}

var header = []string{"index", "sheet", "row", "uhid", "status", "timestamp", "error"}

// Load reads an existing checkpoint CSV, if present, returning the entries
// keyed by record index. A missing file is not an error.
func Load(path string) (map[int]Entry, error) {
	entries := make(map[int]Entry)

	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening checkpoint %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading checkpoint %q: %w", path, err)
	}
	for i, row := range rows {
		if i == 0 || len(row) < 7 {
			continue // header or malformed row
		}
		var idx, rowNum int
		fmt.Sscanf(row[0], "%d", &idx)
		fmt.Sscanf(row[2], "%d", &rowNum)
		entries[idx] = Entry{
			Index:     idx,
			Sheet:     row[1],
			Row:       rowNum,
			UHID:      row[3],
			Status:    row[4],
			Timestamp: row[5],
			Error:     row[6],
		}
	}
	return entries, nil
}

// LatestSuccessfulIndex returns the highest record index marked "success" in
// the checkpoint, or 0 if none succeeded yet.
func LatestSuccessfulIndex(entries map[int]Entry) int {
	latest := 0
	for _, e := range entries {
		if e.Status == "success" && e.Index > latest {
			latest = e.Index
		}
	}
	return latest
}

// Writer appends result rows to the checkpoint CSV as they complete. Safe
// for concurrent use by multiple worker goroutines.
type Writer struct {
	mu     sync.Mutex
	file   *os.File
	writer *csv.Writer
}

// NewWriter opens (or creates) the checkpoint file for appending, creating
// parent directories and writing the header row if the file is new.
func NewWriter(path string) (*Writer, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("creating checkpoint directory %q: %w", dir, err)
		}
	}

	writeHeader := false
	if stat, err := os.Stat(path); os.IsNotExist(err) || (err == nil && stat.Size() == 0) {
		writeHeader = true
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening checkpoint %q: %w", path, err)
	}

	w := csv.NewWriter(f)
	if writeHeader {
		if err := w.Write(header); err != nil {
			f.Close()
			return nil, fmt.Errorf("writing checkpoint header: %w", err)
		}
		w.Flush()
	}

	return &Writer{file: f, writer: w}, nil
}

// Write appends a single checkpoint row, flushing immediately so progress is
// never lost if the process is interrupted.
func (c *Writer) Write(e Entry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e.Timestamp == "" {
		e.Timestamp = time.Now().Format(time.RFC3339)
	}
	row := []string{
		fmt.Sprintf("%d", e.Index),
		e.Sheet,
		fmt.Sprintf("%d", e.Row),
		e.UHID,
		e.Status,
		e.Timestamp,
		e.Error,
	}
	if err := c.writer.Write(row); err != nil {
		return err
	}
	c.writer.Flush()
	return c.writer.Error()
}

// Close flushes and closes the underlying checkpoint file.
func (c *Writer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writer.Flush()
	return c.file.Close()
}
