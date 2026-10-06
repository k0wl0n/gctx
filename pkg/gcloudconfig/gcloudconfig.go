// Package gcloudconfig reads gcloud's on-disk configuration state without
// running gcloud. It is read-only: all changes go through the gcloud CLI.
package gcloudconfig

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dir returns gcloud's configuration directory: $CLOUDSDK_CONFIG when set,
// otherwise %APPDATA%\gcloud on Windows and ~/.config/gcloud elsewhere.
func Dir(getenv func(string) string, goos, home string) string {
	if dir := getenv("CLOUDSDK_CONFIG"); dir != "" {
		return dir
	}
	if goos == "windows" {
		if appData := getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "gcloud")
		}
		return filepath.Join(home, "AppData", "Roaming", "gcloud")
	}
	return filepath.Join(home, ".config", "gcloud")
}

// ActiveConfig returns the globally active configuration recorded in the
// active_config file ("default" when absent). It deliberately ignores
// CLOUDSDK_ACTIVE_CONFIG_NAME, which only pins the current process tree.
func ActiveConfig(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "active_config"))
	if err != nil {
		return "default"
	}
	if name := strings.TrimSpace(string(data)); name != "" {
		return name
	}
	return "default"
}

// Config is the subset of a gcloud configuration gctx cares about.
type Config struct {
	Name    string
	Exists  bool
	Account string // [core] account
	Project string // [core] project
}

// Path returns the file backing a named configuration.
func Path(dir, name string) string {
	return filepath.Join(dir, "configurations", "config_"+name)
}

// Read loads a configuration. A missing file is not an error; it yields
// Exists=false. Unreadable files return an error.
func Read(dir, name string) (Config, error) {
	cfg := Config{Name: name}
	data, err := os.ReadFile(Path(dir, name))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading gcloud configuration %s: %w", name, err)
	}
	cfg.Exists = true

	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || section != "core" {
			continue
		}
		switch strings.TrimSpace(key) {
		case "account":
			cfg.Account = strings.TrimSpace(value)
		case "project":
			cfg.Project = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return cfg, fmt.Errorf("parsing gcloud configuration %s: %w", name, err)
	}
	return cfg, nil
}
