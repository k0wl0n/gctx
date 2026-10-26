// Command fakecodex is a test double for the Codex CLI. It records every
// invocation as a JSON line in $FAKE_CODEX_LOG and emulates the subset gctx
// uses: `login` and `login status` inside $CODEX_HOME. Like the real CLI it
// fails when CODEX_HOME does not exist.
//
// Behavior knobs (all optional):
//
//	FAKE_CODEX_LOGIN_FAIL=1 login exits 1
//
// login writes auth.json holding fakeToken; login status reports logged in
// when that file exists and never prints the token.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const fakeToken = "FAKE-CODEX-SECRET-TOKEN"

type invocation struct {
	Args      []string `json:"args"`
	CodexHome string   `json:"codex_home"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	logInvocation(args)
	home := os.Getenv("CODEX_HOME")
	if home != "" {
		if _, err := os.Stat(home); err != nil {
			fmt.Fprintf(os.Stderr, "Error: CODEX_HOME points to %q, but that path does not exist\n", home)
			return 1
		}
	}
	switch strings.Join(args, " ") {
	case "login":
		if os.Getenv("FAKE_CODEX_LOGIN_FAIL") != "" {
			fmt.Fprintln(os.Stderr, "fake-codex: login failed")
			return 1
		}
		data := fmt.Sprintf(`{"tokens":{"access_token":%q}}`, fakeToken)
		if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(data), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Successfully logged in")
		return 0
	case "login status":
		if _, err := os.Stat(filepath.Join(home, "auth.json")); home == "" || err != nil {
			fmt.Fprintln(os.Stderr, "Not logged in")
			return 1
		}
		fmt.Fprintln(os.Stderr, "Logged in using ChatGPT")
		return 0
	}
	fmt.Printf("fake-codex: %s\n", strings.Join(args, " "))
	return 0
}

func logInvocation(args []string) {
	path := os.Getenv("FAKE_CODEX_LOG")
	if path == "" {
		return
	}
	line, _ := json.Marshal(invocation{Args: args, CodexHome: os.Getenv("CODEX_HOME")})
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, string(line))
}
