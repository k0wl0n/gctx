// Command fakeaws is a test double for the aws CLI. It records every
// invocation as a JSON line in $FAKE_AWS_LOG and emulates the subset gctx uses.
//
// Behavior knobs (all optional):
//
//	FAKE_AWS_STS_FAIL=1       sts get-caller-identity exits 255 with ExpiredToken
//	FAKE_AWS_STS_HANG=1       sts get-caller-identity blocks until killed
//	FAKE_AWS_STS_NETFAIL=1    sts get-caller-identity exits 255 with a network error
//	FAKE_AWS_ARN=<arn>        ARN returned by sts (default arn:aws:iam::123456789012:user/tester)
//	FAKE_AWS_LOGIN_FAIL=1     sso login exits 1
//
// "configure set profile.<name>.<key> <value>" edits $AWS_CONFIG_FILE.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type invocation struct {
	Args       []string `json:"args"`
	ConfigFile string   `json:"aws_config_file"`
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	logInvocation(args)
	joined := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(joined, "sts get-caller-identity"):
		return sts()
	case strings.HasPrefix(joined, "sso login"):
		if os.Getenv("FAKE_AWS_LOGIN_FAIL") != "" {
			fmt.Fprintln(os.Stderr, "login failed")
			return 1
		}
		fmt.Println("fake-aws: sso login complete")
		return 0
	case strings.HasPrefix(joined, "configure set") && len(args) == 4:
		return configureSet(args[2], args[3])
	default:
		fmt.Printf("fake-aws: %s\n", joined)
		return 0
	}
}

func sts() int {
	if os.Getenv("FAKE_AWS_STS_HANG") != "" {
		time.Sleep(time.Minute)
	}
	if os.Getenv("FAKE_AWS_STS_NETFAIL") != "" {
		fmt.Fprintln(os.Stderr, `Could not connect to the endpoint URL: "https://sts.amazonaws.com/"`)
		return 255
	}
	if os.Getenv("FAKE_AWS_STS_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "An error occurred (ExpiredToken) when calling the GetCallerIdentity operation: The security token included in the request is expired")
		return 255
	}
	arn := os.Getenv("FAKE_AWS_ARN")
	if arn == "" {
		arn = "arn:aws:iam::123456789012:user/tester"
	}
	out, _ := json.Marshal(map[string]string{"UserId": "AIDAFAKE", "Account": "123456789012", "Arn": arn})
	fmt.Println(string(out))
	return 0
}

// configureSet handles "profile.<name>.<key>" by editing the INI file.
func configureSet(name, value string) int {
	parts := strings.SplitN(name, ".", 3)
	path := os.Getenv("AWS_CONFIG_FILE")
	if len(parts) != 3 || parts[0] != "profile" || path == "" {
		fmt.Fprintln(os.Stderr, "fake-aws: unsupported configure set")
		return 2
	}
	data, _ := os.ReadFile(path)
	header := "[profile " + parts[1] + "]"
	var out []string
	inSection, found := false, false
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			if inSection && !found {
				out = append(out, parts[2]+" = "+value)
				found = true
			}
			inSection = strings.TrimSpace(line) == header
		}
		out = append(out, line)
	}
	if !found {
		if !hasHeader(out, header) {
			out = append(out, header)
		}
		out = append(out, parts[2]+" = "+value)
	}
	if err := os.WriteFile(path, []byte(strings.TrimLeft(strings.Join(out, "\n"), "\n")+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func hasHeader(lines []string, header string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == header {
			return true
		}
	}
	return false
}

func logInvocation(args []string) {
	path := os.Getenv("FAKE_AWS_LOG")
	if path == "" {
		return
	}
	line, _ := json.Marshal(invocation{Args: args, ConfigFile: os.Getenv("AWS_CONFIG_FILE")})
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, string(line))
}
