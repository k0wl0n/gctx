package watcher

import (
	"fmt"
	"github.com/k0wl0n/gctx/pkg/adc"
	"os"
	"time"
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
			}

		case <-timeoutChan:
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
