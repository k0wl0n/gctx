package tui

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

const debugLogMaxSize = 1 << 20 // 1 MiB

// debugLogPath is where the debug log is written when debug logging is on.
func debugLogPath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, "gctx", "tui-debug.log"), nil
}

// newDebugLogger opens or creates the debug log file.
// When enabled is false it returns a discarding logger and a no-op closer.
func newDebugLogger(enabled bool) (*slog.Logger, io.Closer, error) {
	if !enabled {
		return slog.New(slog.NewTextHandler(io.Discard, nil)), io.NopCloser(nil), nil
	}
	logPath, err := debugLogPath()
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return nil, nil, err
	}

	// Truncate if over 1 MiB.
	if fi, err := os.Stat(logPath); err == nil && fi.Size() > debugLogMaxSize {
		if err := os.Truncate(logPath, 0); err != nil {
			return nil, nil, err
		}
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, err
	}
	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, f, nil
}
