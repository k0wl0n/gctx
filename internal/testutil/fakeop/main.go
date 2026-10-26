// Command fakeop is a test double for the 1Password op CLI. It records every
// invocation as a JSON line in $FAKE_OP_LOG.
//
// Behavior knobs (all optional):
//
//	FAKE_OP_VALUE=<text>  value printed by `op read` (default: a clearly fake key)
//	FAKE_OP_FAIL=1        print the value to stdout, then exit 1
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const defaultValue = "FAKEKEY-do-not-use-1234567890"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	logInvocation(args)
	if len(args) < 2 || args[0] != "read" {
		fmt.Fprintln(os.Stderr, "fake-op: unsupported command")
		return 2
	}
	value := os.Getenv("FAKE_OP_VALUE")
	if value == "" {
		value = defaultValue
	}
	fmt.Println(value)
	if os.Getenv("FAKE_OP_FAIL") != "" {
		return 1
	}
	return 0
}

func logInvocation(args []string) {
	path := os.Getenv("FAKE_OP_LOG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	line, _ := json.Marshal(map[string][]string{"args": args})
	fmt.Fprintln(f, string(line))
}
