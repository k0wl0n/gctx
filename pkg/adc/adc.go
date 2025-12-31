package adc

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ADCCredential struct {
	ClientID       string `json:"client_id"`
	ClientSecret   string `json:"client_secret"`
	QuotaProjectID string `json:"quota_project_id"`
	RefreshToken   string `json:"refresh_token"`
	Type           string `json:"type"`
}

// GetDefaultADCPath returns the default ADC location
func GetDefaultADCPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gcloud",
		"application_default_credentials.json")
}

// GetStoragePath returns the storage path for an account's ADC
func GetStoragePath(accountName string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gctx", "adc",
		fmt.Sprintf("%s_adc.json", accountName))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}

	destFile, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// SaveADC copies current ADC to storage for an account
func SaveADC(accountName string) (string, error) {
	defaultPath := GetDefaultADCPath()
	if !fileExists(defaultPath) {
		return "", fmt.Errorf("no ADC found at %s", defaultPath)
	}

	// Validate JSON
	if err := ValidateADC(defaultPath); err != nil {
		return "", fmt.Errorf("invalid ADC file: %w", err)
	}

	// Copy to storage
	storagePath := GetStoragePath(accountName)
	if err := copyFile(defaultPath, storagePath); err != nil {
		return "", err
	}

	return storagePath, nil
}

// RestoreADC copies saved ADC back to default location
func RestoreADC(accountName string) error {
	storagePath := GetStoragePath(accountName)
	if !fileExists(storagePath) {
		return fmt.Errorf("no saved ADC for account: %s", accountName)
	}

	defaultPath := GetDefaultADCPath()

	// Atomic write: temp file -> rename
	tempPath := defaultPath + ".tmp"
	if err := copyFile(storagePath, tempPath); err != nil {
		return err
	}

	if err := os.Rename(tempPath, defaultPath); err != nil {
		os.Remove(tempPath)
		return err
	}

	return nil
}

// ValidateADC checks if ADC JSON is valid
func ValidateADC(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var adc ADCCredential
	return json.Unmarshal(data, &adc)
}

// GetADCEmail extracts email from ADC file
func GetADCEmail(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var adc struct {
		QuotaProjectID string `json:"quota_project_id"`
		Type           string `json:"type"`
	}

	if err := json.Unmarshal(data, &adc); err != nil {
		return "", err
	}

	// Try to get email from gcloud
	cmd := exec.Command("gcloud", "config", "get-value", "account")
	output, err := cmd.Output()
	if err != nil {
		return "", nil
	}

	return strings.TrimSpace(string(output)), nil
}
