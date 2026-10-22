package aws

import "time"

// SetLogoutTimeout shortens the logout bound for a test and returns the restore function.
func SetLogoutTimeout(d time.Duration) (restore func()) {
	previous := logoutTimeout
	logoutTimeout = d
	return func() { logoutTimeout = previous }
}
