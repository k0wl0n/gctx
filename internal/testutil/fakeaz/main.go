// Command fakeaz is a test double for the az CLI. It records every invocation
// as a JSON line in $FAKE_AZ_LOG and emulates the subset gctx uses.
//
// Behavior knobs (all optional):
//
//	FAKE_AZ_LOGIN_FAIL=1   login exits 1
//	FAKE_AZ_TOKEN_FAIL=1   account get-access-token exits 1 with an expiry error
//	FAKE_AZ_TOKEN_HANG=1   account get-access-token blocks until killed
//	FAKE_AZ_TOKEN_NETFAIL=1 account get-access-token exits 1 with a network error
//
// get-access-token prints a token-looking value only when --query is absent,
// so tests can prove gctx never asks for the token itself.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// TokenMarker is the secret-looking value emitted without --query.
const tokenMarker = "FAKE-AZ-SECRET-ACCESS-TOKEN"

type invocation struct {
	Args         []string `json:"args"`
	ConfigDir    string   `json:"azure_config_dir"`
	TokenEmitted bool     `json:"token_emitted"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	joined := strings.Join(args, " ")
	tokenEmitted := false
	code := 0
	switch {
	case strings.HasPrefix(joined, "login"):
		code = login()
	case strings.HasPrefix(joined, "account get-access-token"):
		code, tokenEmitted = accessToken(args)
	default:
		fmt.Printf("fake-az: %s\n", joined)
	}
	logInvocation(args, tokenEmitted)
	return code
}

func login() int {
	if os.Getenv("FAKE_AZ_LOGIN_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "login failed")
		return 1
	}
	dir := os.Getenv("AZURE_CONFIG_DIR")
	if dir == "" {
		fmt.Fprintln(os.Stderr, "fake-az: AZURE_CONFIG_DIR not set")
		return 2
	}
	profile := `{"subscriptions":[{"id":"fake","isDefault":true}]}`
	if err := os.WriteFile(filepath.Join(dir, "azureProfile.json"), []byte(profile), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("fake-az: To sign in, use a web browser to open https://microsoft.com/devicelogin and enter the code FAKECODE")
	return 0
}

func accessToken(args []string) (code int, tokenEmitted bool) {
	if os.Getenv("FAKE_AZ_TOKEN_HANG") != "" {
		time.Sleep(time.Minute)
	}
	if os.Getenv("FAKE_AZ_TOKEN_NETFAIL") != "" {
		fmt.Fprintln(os.Stderr, "ERROR: HTTPSConnectionPool(host='login.microsoftonline.com', port=443): Max retries exceeded (Failed to establish a new connection: [Errno 8] nodename nor servname provided, or not known)")
		return 1, false
	}
	if os.Getenv("FAKE_AZ_TOKEN_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "ERROR: AADSTS70043: The refresh token has expired. Run 'az login' to refresh.")
		return 1, false
	}
	if slices.Contains(args, "--query") {
		fmt.Println("2030-01-01 00:00:00.000000")
		return 0, false
	}
	out, _ := json.Marshal(map[string]string{"accessToken": tokenMarker, "expiresOn": "2030-01-01 00:00:00.000000"})
	fmt.Println(string(out))
	return 0, true
}

func logInvocation(args []string, tokenEmitted bool) {
	path := os.Getenv("FAKE_AZ_LOG")
	if path == "" {
		return
	}
	line, _ := json.Marshal(invocation{Args: args, ConfigDir: os.Getenv("AZURE_CONFIG_DIR"), TokenEmitted: tokenEmitted})
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, string(line))
}
