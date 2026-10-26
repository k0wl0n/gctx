package provider

import "strings"

// An env entry is NAME=value to set a variable, or a bare NAME to remove it
// from the process environment (and `unset NAME` in a shell).

// MergeEnv returns base with extra applied in order; base is not modified.
func MergeEnv(base, extra []string) []string {
	merged := append([]string(nil), base...)
	for _, entry := range extra {
		name, _, isSet := strings.Cut(entry, "=")
		if isSet {
			merged = append(merged, entry)
			continue
		}
		kept := merged[:0]
		for _, existing := range merged {
			if existingName, _, _ := strings.Cut(existing, "="); existingName != name {
				kept = append(kept, existing)
			}
		}
		merged = kept
	}
	return merged
}
