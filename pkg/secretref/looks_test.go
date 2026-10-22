package secretref

import "testing"

func TestLooksLikeKey(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"sk-abc123", true},
		{"sk-or-v1-abcdef", true},
		{"ghp_abcdefghijkl", true},
		{"AKIAIOSFODNN7EXAMPLE", true},
		{"abcdefghijklmnopqrstuvwxyzABCDEF0123", true},
		{"abcdefghijklmnopqrstuvwxyz_ABCDEF-0123456789", true},
		{"op://Private/sumopod-dev/credential", false},
		{"gcpsm://aliz-ai/sumopod-prod", false},
		{"keychain:openrouter", false},
		{"env:OPENAI_API_KEY", false},
		{"env:ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", false},
		{"short", false},
		{"", false},
		{"has/slash/abcdefghijklmnopqrstuvwxyz0123456789", false},
	}
	for _, c := range cases {
		if got := LooksLikeKey(c.in); got != c.want {
			t.Errorf("LooksLikeKey(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
