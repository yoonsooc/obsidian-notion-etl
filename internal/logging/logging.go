// Package logging provides a thin per-run file logger for item-level warnings
// from the CLI commands. Fatal errors go to stderr in main, not here.
package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// Logger writes the log file for one command run. Safe for concurrent use:
// log.Logger serializes writes and wrote is atomic.
type Logger struct {
	file  *os.File
	l     *log.Logger
	path  string
	wrote atomic.Bool
}

// New creates a run-timestamped log file under logs/<name>/,
// e.g. New("migration", now) -> logs/migration/2026-08-21-143000.log.
func New(name string, now time.Time) (*Logger, error) {
	dir := filepath.Join("logs", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	path := filepath.Join(dir, now.Format("2006-01-02-150405")+".log")
	// Append mode so a rerun within the same second keeps prior records.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	return &Logger{
		file: file,
		l:    log.New(file, "", log.LstdFlags),
		path: path,
	}, nil
}

// Warnf logs an item-level warning.
func (lg *Logger) Warnf(format string, args ...any) {
	lg.wrote.Store(true)
	lg.l.Printf("WARN "+format, args...)
}

// Infof logs item-level progress information.
func (lg *Logger) Infof(format string, args ...any) {
	lg.wrote.Store(true)
	lg.l.Printf("INFO "+format, args...)
}

// Path returns the log file path.
func (lg *Logger) Path() string {
	return lg.path
}

// Wrote reports whether anything was logged during this run.
func (lg *Logger) Wrote() bool {
	return lg.wrote.Load()
}

// Close closes the log file, removing it if nothing was logged so empty
// files do not accumulate (removal failure is ignored).
func (lg *Logger) Close() error {
	if err := lg.file.Close(); err != nil {
		return fmt.Errorf("close log file: %w", err)
	}
	if !lg.wrote.Load() {
		_ = os.Remove(lg.path)
	}
	return nil
}

// Write implements io.Writer so the logger can be injected as a warning
// sink; each call becomes one WARN record.
func (lg *Logger) Write(p []byte) (int, error) {
	lg.wrote.Store(true)
	lg.l.Print("WARN " + string(p))
	return len(p), nil
}
