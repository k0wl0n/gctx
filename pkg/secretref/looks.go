package secretref

import "strings"

const minTokenLength = 32

var keyPrefixes = []string{"sk-", "sk-or-", "ghp_", "AKIA"}

// LooksLikeKey reports whether s looks like a raw API key rather than a
// reference such as op://..., gcpsm://..., keychain:... or env:....
func LooksLikeKey(s string) bool {
	if s == "" || strings.ContainsAny(s, ":/") {
		return false
	}
	for _, prefix := range keyPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	if len(s) < minTokenLength {
		return false
	}
	for _, r := range s {
		isTokenChar := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		if !isTokenChar {
			return false
		}
	}
	return true
}
