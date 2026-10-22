package azure

import "time"

var ParseDeviceCode = parseDeviceCode

func NewDeviceCodeScanner() *deviceCodeScanner { return &deviceCodeScanner{} }

func (s *deviceCodeScanner) Scan(line string) (url, code string, ok bool) { return s.scan(line) }

// SetLogoutTimeout shortens the logout bound for a test and returns the restore function.
func SetLogoutTimeout(d time.Duration) (restore func()) {
	previous := logoutTimeout
	logoutTimeout = d
	return func() { logoutTimeout = previous }
}
