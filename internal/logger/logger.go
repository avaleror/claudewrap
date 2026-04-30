// Package logger writes structured JSONL operation logs to ~/.claudewrap/logs/.
// One file per day. All writes are goroutine-safe. Call Init() at startup and
// Close() on exit. If Init() was never called, Log() is a silent no-op.
package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry is one logged operation.
type Entry struct {
	Timestamp  string  `json:"timestamp"`
	Operation  string  `json:"operation"`             // compress | filter | fallback | compact | token_update
	Engine     string  `json:"engine,omitempty"`
	TokensIn   int     `json:"tokens_in,omitempty"`
	TokensOut  int     `json:"tokens_out,omitempty"`
	Ratio      float64 `json:"ratio,omitempty"`       // compression ratio 0–1
	DurationMs int64   `json:"duration_ms,omitempty"`
	Success    bool    `json:"success"`
	Skipped    bool    `json:"skipped,omitempty"`
	CacheHit   bool    `json:"cache_hit,omitempty"`
	Redacted   bool    `json:"redacted,omitempty"` // true if secrets were stripped
	Error      string  `json:"error,omitempty"`
}

var (
	mu      sync.Mutex
	logFile *os.File
)

// Init opens today's log file under ~/.claudewrap/logs/. Safe to call multiple times.
func Init() error {
	dir := filepath.Join(os.Getenv("HOME"), ".claudewrap", "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	date := time.Now().Format("2006-01-02")
	path := filepath.Join(dir, "claudewrap-"+date+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
	}
	logFile = f
	return nil
}

// Log appends an entry to the current log file. No-op if Init was not called.
func Log(e Entry) {
	e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	mu.Lock()
	defer mu.Unlock()
	if logFile == nil {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	logFile.Write(append(data, '\n'))
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}
