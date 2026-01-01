package gcloud

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// CreateConfig creates a new gcloud configuration
func CreateConfig(configName string) error {
	cmd := exec.Command("gcloud", "config", "configurations",
		"create", configName)
	output, err := cmd.CombinedOutput()

	// Ignore "already exists" error
	if err != nil && !strings.Contains(string(output), "already exists") {
		return fmt.Errorf("failed to create config: %w\n%s", err, output)
	}

	return nil
}

// ActivateConfig activates a gcloud configuration
func ActivateConfig(configName string) error {
	cmd := exec.Command("gcloud", "config", "configurations",
		"activate", configName)
	return cmd.Run()
}

// SetProject sets the project for current configuration
func SetProject(projectID string) error {
	cmd := exec.Command("gcloud", "config", "set", "project", projectID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, string(output))
	}
	return nil
}

// AuthLogin runs gcloud auth login interactively
func AuthLogin() error {
	cmd := exec.Command("gcloud", "auth", "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// AuthADCLogin runs gcloud auth application-default login
func AuthADCLogin() ([]string, error) {
	cmd := exec.Command("gcloud", "auth", "application-default", "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout

	// Capture stderr for warnings
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Read stderr
	stderr, _ := io.ReadAll(stderrPipe)
	warnings := parseWarnings(string(stderr))

	err = cmd.Wait()
	return warnings, err
}

// RunCommand runs arbitrary gcloud command
func RunCommand(args ...string) error {
	cmd := exec.Command("gcloud", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ListConfigs returns all gcloud configurations
func ListConfigs() ([]string, error) {
	cmd := exec.Command("gcloud", "config", "configurations",
		"list", "--format=json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var configs []struct {
		Name string `json:"name"`
	}

	if err := json.Unmarshal(output, &configs); err != nil {
		return nil, err
	}

	names := make([]string, len(configs))
	for i, c := range configs {
		names[i] = c.Name
	}

	return names, nil
}

func parseWarnings(stderr string) []string {
	var warnings []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "WARNING") ||
			strings.Contains(line, "quota") {
			warnings = append(warnings, strings.TrimSpace(line))
		}
	}
	return warnings
}
