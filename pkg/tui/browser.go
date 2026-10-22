package tui

import (
	"os/exec"
	"runtime"
)

// browserCommand is the command that opens url in the default browser.
func browserCommand(goos, url string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	}
	return "xdg-open", []string{url}
}

// openBrowser starts the browser without waiting for it.
func openBrowser(url string) error {
	name, args := browserCommand(runtime.GOOS, url)
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
