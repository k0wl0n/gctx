package adc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store manages the per-account ADC copies kept by gctx and the default ADC
// file that gcloud and Google client libraries read.
type Store struct {
	Dir         string // ~/.config/gctx/adc
	DefaultPath string // <gcloud dir>/application_default_credentials.json
}

// StoragePath returns the canonical stored-ADC path for an account.
func (s Store) StoragePath(accountName string) string {
	return filepath.Join(s.Dir, accountName+"_adc.json")
}

// Contains reports whether path lies inside the store directory.
func (s Store) Contains(path string) bool {
	if path == "" {
		return false
	}
	absDir, err := filepath.Abs(s.Dir)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

// Validate checks that path holds a JSON object. Error messages never include
// file content, because the file holds credentials.
func Validate(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %s", path, describeJSONError(err))
	}
	return nil
}

// describeJSONError avoids encoding/json messages that quote input characters.
func describeJSONError(err error) string {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntaxErr):
		if strings.Contains(syntaxErr.Error(), "unexpected end of JSON input") {
			return "unexpected end of JSON input"
		}
		return fmt.Sprintf("invalid JSON at byte offset %d", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		return "not a JSON object"
	}
	return "invalid JSON"
}

// Save copies the default ADC file into the account's stored location.
func (s Store) Save(accountName string) (string, error) {
	if !regularFileExists(s.DefaultPath) {
		return "", fmt.Errorf("no ADC found at %s", s.DefaultPath)
	}
	if err := Validate(s.DefaultPath); err != nil {
		return "", fmt.Errorf("invalid ADC file: %w", err)
	}
	storagePath := s.StoragePath(accountName)
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return "", err
	}
	if err := copyAtomic(s.DefaultPath, storagePath); err != nil {
		return "", err
	}
	return storagePath, nil
}

// Restore replaces the default ADC file with the account's stored copy.
func (s Store) Restore(accountName string) error {
	storagePath := s.StoragePath(accountName)
	if !regularFileExists(storagePath) {
		return fmt.Errorf("no saved ADC for account: %s", accountName)
	}
	if err := os.MkdirAll(filepath.Dir(s.DefaultPath), 0700); err != nil {
		return err
	}
	return copyAtomic(storagePath, s.DefaultPath)
}

// Remove deletes the account's stored ADC files: the canonical path and the
// recorded path when it lies inside the store. A recorded path outside the
// store is never deleted and is returned as outside.
func (s Store) Remove(accountName, recordedPath string) (outside string, err error) {
	paths := []string{s.StoragePath(accountName)}
	if recordedPath != "" && filepath.Clean(recordedPath) != filepath.Clean(paths[0]) {
		if s.Contains(recordedPath) {
			paths = append(paths, recordedPath)
		} else {
			outside = recordedPath
		}
	}
	var errs []error
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return outside, errors.Join(errs...)
}

// FileState is a cheap fingerprint used to detect whether a file was rewritten.
type FileState struct {
	Exists  bool
	ModTime time.Time
	Size    int64
}

// Snapshot captures the current state of path.
func Snapshot(path string) FileState {
	info, err := os.Stat(path)
	if err != nil {
		return FileState{}
	}
	return FileState{Exists: true, ModTime: info.ModTime(), Size: info.Size()}
}

// Changed reports whether the file differs from an earlier snapshot.
func (before FileState) Changed(after FileState) bool {
	return before.Exists != after.Exists || !before.ModTime.Equal(after.ModTime) || before.Size != after.Size
}

// FilesEqual reports whether both files exist with identical content.
func FilesEqual(a, b string) (bool, error) {
	dataA, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	dataB, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(dataA, dataB), nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// copyAtomic copies src to dst through a temp file in dst's directory, with
// owner-only permissions.
func copyAtomic(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+"-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op after a successful rename

	if err := tmp.Chmod(0600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		tmp.Close()
		return err
	}
	if _, err := io.Copy(tmp, source); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, dst)
}
