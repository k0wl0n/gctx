// Command fakeclaude is a test double for the Claude Code CLI. It records
// every invocation as a JSON line in $FAKE_CLAUDE_LOG and emulates the subset
// gctx uses: `auth login` and `auth status` inside $CLAUDE_CONFIG_DIR.
//
// Behavior knobs (all optional):
//
//	FAKE_CLAUDE_LOGIN_FAIL=1  auth login exits 1
//	FAKE_CLAUDE_STATUS_FAIL=1 auth status exits 2 with an unexpected error
//	FAKE_CLAUDE_DEFAULT_DIR   directory used when CLAUDE_CONFIG_DIR is unset,
//	                          like the real CLI's ~/.claude
//
// auth login writes a credentials file holding fakeToken; auth status reports
// logged in when that file exists. Status never prints the token, so tests
// can prove gctx never surfaces it.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	fakeToken       = "FAKE-CLAUDE-SECRET-TOKEN"
	credentialsFile = ".credentials.json"
)

type invocation struct {
	Args      []string `json:"args"`
	ConfigDir string   `json:"claude_config_dir"`
	Env       []string `json:"env"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	logInvocation(args)
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = os.Getenv("FAKE_CLAUDE_DEFAULT_DIR")
	}
	switch strings.Join(args, " ") {
	case "auth login":
		if os.Getenv("FAKE_CLAUDE_LOGIN_FAIL") != "" {
			fmt.Fprintln(os.Stderr, "fake-claude: login failed")
			return 1
		}
		if dir == "" {
			fmt.Fprintln(os.Stderr, "fake-claude: CLAUDE_CONFIG_DIR not set")
			return 2
		}
		data := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q}}`, fakeToken)
		if err := os.WriteFile(filepath.Join(dir, credentialsFile), []byte(data), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("Login successful.")
		return 0
	case "auth status":
		if os.Getenv("FAKE_CLAUDE_STATUS_FAIL") != "" {
			fmt.Fprintln(os.Stderr, "fake-claude: unexpected failure")
			return 2
		}
		if _, err := os.Stat(filepath.Join(dir, credentialsFile)); dir == "" || err != nil {
			fmt.Println(`{"loggedIn":false,"authMethod":"none","apiProvider":"firstParty"}`)
			return 1
		}
		fmt.Println(`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","email":"dev@example.com","orgId":"org-FAKE-ORG-ID","orgName":"Example Org","subscriptionType":"max"}`)
		return 0
	}
	fmt.Printf("fake-claude: %s\n", strings.Join(args, " "))
	return 0
}

func logInvocation(args []string) {
	path := os.Getenv("FAKE_CLAUDE_LOG")
	if path == "" {
		return
	}
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "CLAUDE_") || strings.HasPrefix(kv, "ANTHROPIC_") || strings.HasPrefix(kv, "CLOUD_ML_") {
			env = append(env, kv)
		}
	}
	line, _ := json.Marshal(invocation{Args: args, ConfigDir: os.Getenv("CLAUDE_CONFIG_DIR"), Env: env})
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, string(line))
}
