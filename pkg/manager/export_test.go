package manager

import "time"

// SetCLIVersionTimeout replaces cliVersionTimeout and returns a restore func.
func SetCLIVersionTimeout(d time.Duration) (restore func()) {
	previous := cliVersionTimeout
	cliVersionTimeout = d
	return func() { cliVersionTimeout = previous }
}
