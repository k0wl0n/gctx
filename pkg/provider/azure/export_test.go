package azure

var ParseDeviceCode = parseDeviceCode

func NewDeviceCodeScanner() *deviceCodeScanner { return &deviceCodeScanner{} }

func (s *deviceCodeScanner) Scan(line string) (url, code string, ok bool) { return s.scan(line) }
