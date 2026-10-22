package providerutil_test

import (
	"testing"

	"github.com/k0wl0n/gctx/pkg/provider/providerutil"
)

func TestIsAuthFailure(t *testing.T) {
	markers := []string{"ExpiredToken", "Unable to locate credentials"}
	for _, tt := range []struct {
		output string
		want   bool
	}{
		{"An error occurred (ExpiredToken) when calling GetCallerIdentity", true},
		{"an error occurred (expiredtoken)", true},
		{"Unable to locate credentials. You can configure credentials by running \"aws configure\".", true},
		{"Could not connect to the endpoint URL: \"https://sts.amazonaws.com/\"", false},
		{"dial tcp: lookup sts.amazonaws.com: no such host", false},
		{"", false},
	} {
		if got := providerutil.IsAuthFailure(tt.output, markers); got != tt.want {
			t.Errorf("%q: %v, want %v", tt.output, got, tt.want)
		}
	}
	if providerutil.IsAuthFailure("ExpiredToken", nil) {
		t.Error("no markers must never match")
	}
}
