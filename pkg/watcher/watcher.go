package watcher

import (
	"fmt"
	"os"
	"time"

	"github.com/k0wl0n/gctx/pkg/adc"
)

// WatchADC watches for ADC file changes with timeout
func WatchADC(timeout time.Duration) error {
	adcPath := adc.GetDefaultADCPath()

	// Get initial state
	initialState, err := getFileState(adcPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	fmt.Println("Watching for ADC file changes...")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			currentState, err := getFileState(adcPath)
			if err != nil {
				continue
			}

			// Check if file changed
			// If initialState is nil (file didn't exist), any existence is a change
			// If initialState exists, check if ModTime is after
			if (initialState == nil && currentState != nil) ||
				(initialState != nil && currentState.ModTime.After(initialState.ModTime)) {
				// Wait for write to complete
				time.Sleep(1 * time.Second)

				// Validate
				if err := adc.ValidateADC(adcPath); err == nil {
					return nil
				}
			} else if initialState != nil && currentState != nil && initialState.ModTime.Equal(currentState.ModTime) {
				// If the file exists and hasn't changed, but we are in a context where we expect it to be valid
				// (e.g. after a successful auth command), we can check validity.
				// However, the watcher's job is specifically to wait for a *change* or *creation*.
				// If the auth command didn't update the file (e.g. because credentials were already valid),
				// the timestamp won't change.

				// For now, let's keep the strict behavior: if no change detected, we timeout.
				// But we can add a check: if the file is valid right now, maybe we can return early?
				// But that defeats the purpose of "watching for new login".
			}

		case <-timeoutChan:
			// Check one last time if the file is valid before returning timeout error
			if err := adc.ValidateADC(adcPath); err == nil {
				// If the file is valid, we can consider it a success even if we didn't detect a modification timestamp change.
				// This handles cases where gcloud didn't update the file because it was already up-to-date.
				return nil
			}
			return fmt.Errorf("timeout waiting for ADC file")
		}
	}
}

type fileState struct {
	ModTime time.Time
	Size    int64
}

func getFileState(path string) (*fileState, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return &fileState{
		ModTime: info.ModTime(),
		Size:    info.Size(),
	}, nil
}
