package debuglog_test

import (
	"strings"
	"testing"

	"github.com/k0wl0n/gctx/pkg/debuglog"
)

func TestRedactReplacesSecretParameters(t *testing.T) {
	input := strings.Join([]string{
		"GET /?state=S1&code=C1&code_challenge=H1&scope=email",
		`{"access_token": "A1", "refresh_token":"R1", "type": "authorized_user"}`,
		"(missing_code) Missing code parameter in response.",
	}, "\n")
	got := debuglog.Redact(input)
	for _, secret := range []string{"S1", "C1", "H1", "A1", "R1"} {
		if strings.Contains(got, secret) {
			t.Errorf("secret %q survived redaction: %s", secret, got)
		}
	}
	for _, kept := range []string{"scope=email", "authorized_user", "(missing_code) Missing code parameter in response."} {
		if !strings.Contains(got, kept) {
			t.Errorf("%q was lost: %s", kept, got)
		}
	}
}

func TestTailWriterKeepsLastBytes(t *testing.T) {
	tail := debuglog.NewTail(8)
	tail.Write([]byte("0123456789"))
	tail.Write([]byte("abc"))
	if got := tail.String(); got != "56789abc" {
		t.Errorf("tail %q", got)
	}
}

func TestRedactFakeGcloudCallbackText(t *testing.T) {
	input := "Redirected to http://localhost:8085/?state=SECRETSTATE&code=SECRETVALUE&scope=email\n" +
		"https://accounts.google.com/o/oauth2/auth?code_challenge=SECRETCHALLENGE&client_secret=SECRETCLIENT\n" +
		"id_token=SECRETID access_token=SECRETACCESS refresh_token=SECRETREFRESH"
	got := debuglog.Redact(input)
	for _, secret := range []string{"SECRETSTATE", "SECRETVALUE", "SECRETCHALLENGE", "SECRETCLIENT", "SECRETID", "SECRETACCESS", "SECRETREFRESH"} {
		if strings.Contains(got, secret) {
			t.Errorf("%s survived: %s", secret, got)
		}
	}
}
