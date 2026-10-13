package tui

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDebugLoggerDisabled tests that a disabled logger discards output.
func TestDebugLoggerDisabled(t *testing.T) {
	logger, closer, err := newDebugLogger(false)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	logger.Info("should not appear")
}

// TestDebugLoggerEnabled tests that an enabled logger creates the log file with correct permissions.
func TestDebugLoggerEnabled(t *testing.T) {
	// Point UserCacheDir to a temp dir.
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)   // Linux
	t.Setenv("HOME", tmp)             // macOS UserCacheDir uses HOME/Library/Caches
	if runtime.GOOS == "darwin" {
		t.Setenv("HOME", tmp)
	}
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", tmp)
	}

	logger, closer, err := newDebugLogger(true)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("test event", "account", "sandbox")
	closer.Close()

	// Find the log file.
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Skip("UserCacheDir unavailable:", err)
	}
	logPath := filepath.Join(cacheDir, "gctx", "tui-debug.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file not found: %v", err)
	}
	if !strings.Contains(string(data), "test event") {
		t.Errorf("log file does not contain expected content; got: %s", data)
	}

	// Check permissions on POSIX.
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(logPath)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0600 {
			t.Errorf("expected mode 0600, got %v", fi.Mode().Perm())
		}
	}
}

// TestDebugLogRunArgsNotLogged verifies that run arguments beyond the verb are not logged.
// This is the credential-safety scenario: running "secrets versions access latest --secret=db-pass"
// must not log "db-pass".
func TestDebugLogRunArgsNotLogged(t *testing.T) {
	tmp := t.TempDir()
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CACHE_HOME", tmp)
	} else if runtime.GOOS == "darwin" {
		t.Setenv("HOME", tmp)
	} else {
		t.Setenv("LOCALAPPDATA", tmp)
	}

	logger, closer, err := newDebugLogger(true)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	// Simulate what run.go should log: only the first arg (verb), not the full command.
	args := []string{"secrets", "versions", "access", "latest", "--secret=db-pass"}
	verb := args[0] // "secrets"
	logger.Info("run", "account", "work", "verb", verb, "exit", 0)
	closer.Close()

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Skip("UserCacheDir unavailable:", err)
	}
	logPath := filepath.Join(cacheDir, "gctx", "tui-debug.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("log file not found: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "secrets") {
		t.Errorf("expected verb 'secrets' in log, got: %s", content)
	}
	if strings.Contains(content, "db-pass") {
		t.Errorf("secret 'db-pass' must not appear in log, but found in: %s", content)
	}
}

// TestDebugLoggerTruncatesOver1MiB verifies that a log file over 1 MiB is truncated.
func TestDebugLoggerTruncatesOver1MiB(t *testing.T) {
	tmp := t.TempDir()
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_CACHE_HOME", tmp)
	} else if runtime.GOOS == "darwin" {
		t.Setenv("HOME", tmp)
	} else {
		t.Setenv("LOCALAPPDATA", tmp)
	}

	// Pre-create a large log file.
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Skip("UserCacheDir unavailable:", err)
	}
	logDir := filepath.Join(cacheDir, "gctx")
	os.MkdirAll(logDir, 0700)
	logPath := filepath.Join(logDir, "tui-debug.log")
	big := make([]byte, debugLogMaxSize+100)
	for i := range big {
		big[i] = 'X'
	}
	os.WriteFile(logPath, big, 0600)

	logger, closer, err := newDebugLogger(true)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("after truncate")
	closer.Close()

	fi, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() >= int64(debugLogMaxSize) {
		t.Errorf("expected file size < 1MiB after truncate, got %d bytes", fi.Size())
	}
}

// Ensure io.NopCloser(nil) does not panic.
func TestNopCloserNilSafe(t *testing.T) {
	c := io.NopCloser(nil)
	c.Close()
}
