package tui

import (
	"io"
	"log/slog"

	"github.com/k0wl0n/gctx/pkg/debuglog"
)

const debugLogMaxSize = 1 << 20 // 1 MiB; keep in step with debuglog

// debugLogPath is where the debug log is written when debug logging is on.
func debugLogPath() (string, error) { return debuglog.Path() }

// newDebugLogger opens or creates the debug log file.
// When enabled is false it returns a discarding logger and a no-op closer.
func newDebugLogger(enabled bool) (*slog.Logger, io.Closer, error) {
	return debuglog.Open(enabled)
}
