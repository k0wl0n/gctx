// Package debuglog is the opt-in, redacted debug log shared by the TUI and the
// providers. Nothing is written unless GCTX_DEBUG=1 or --debug is set.
package debuglog

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/k0wl0n/gctx/pkg/config"
)

const maxSize = 1 << 20 // 1 MiB

// Path is where the debug log is written when debug logging is on.
func Path() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cacheDir, config.AppName, "tui-debug.log"), nil
}

// EnabledByEnv reports whether GCTX_DEBUG asks for debug logging.
func EnabledByEnv() bool { return os.Getenv("GCTX_DEBUG") == "1" }

// Open opens or creates the debug log file. When enabled is false it returns
// a discarding logger and a no-op closer.
func Open(enabled bool) (*slog.Logger, io.Closer, error) {
	if !enabled {
		return slog.New(slog.NewTextHandler(io.Discard, nil)), io.NopCloser(nil), nil
	}
	logPath, err := Path()
	if err != nil {
		return nil, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return nil, nil, err
	}
	if fi, err := os.Stat(logPath); err == nil && fi.Size() > maxSize {
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

var (
	secretQuery = regexp.MustCompile(`(?i)\b(code|state|code_challenge|code_verifier|access_token|refresh_token|id_token|client_secret)=[^&\s"']+`)
	secretJSON  = regexp.MustCompile(`(?i)"(code|state|code_challenge|code_verifier|access_token|refresh_token|id_token|client_secret)"\s*:\s*"[^"]*"`)
)

// Redact masks OAuth codes and tokens in query-string and JSON form.
func Redact(text string) string {
	text = secretQuery.ReplaceAllString(text, "${1}=XXX")
	return secretJSON.ReplaceAllString(text, `"${1}": "XXX"`)
}

// Tail is an io.Writer that keeps only the last Size bytes written to it.
type Tail struct {
	mu   sync.Mutex
	size int
	data []byte
}

// NewTail returns a Tail keeping the last size bytes.
func NewTail(size int) *Tail { return &Tail{size: size} }

func (t *Tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, p...)
	if extra := len(t.data) - t.size; extra > 0 {
		t.data = t.data[extra:]
	}
	return len(p), nil
}

func (t *Tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.data)
}
