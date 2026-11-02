// Command fakegcloud is a test double for the gcloud CLI. It emulates the
// subset of gcloud that gctx uses by manipulating files under the gcloud
// configuration directory, and records every invocation as a JSON line in
// $FAKE_GCLOUD_LOG.
//
// When the executable's base name starts with "fakeshell" it instead acts as a
// shell: it prints the gctx isolation variables and exits with $FAKE_SHELL_EXIT.
//
// Behavior knobs (all optional):
//
//	FAKE_GCLOUD_FAIL="verb:exit[:stderr];..."  fail a verb with an exit code
//	FAKE_GCLOUD_SLEEP="verb:ms;..."            delay a verb
//	FAKE_GCLOUD_ADC_UNCHANGED=1                ADC login leaves the file untouched
//	FAKE_GCLOUD_ACCOUNT=<email>                account used by "auth login"
//	FAKE_GCLOUD_ADC_MISSING_CODE=<n>           the first n ADC logins fail with gcloud's
//	                                           "(missing_code)" error and a URL carrying secrets
//
// Verbs: configurations-create, configurations-activate, configurations-delete,
// configurations-list, config-set, config-get-value, auth-login, auth-adc-login,
// auth-list, other.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	if strings.HasPrefix(strings.ToLower(filepath.Base(os.Args[0])), "fakeshell") {
		os.Exit(runShell())
	}
	os.Exit(run(os.Args[1:]))
}

func runShell() int {
	for _, name := range []string{"GCTX_ACCOUNT", "CLOUDSDK_ACTIVE_CONFIG_NAME", "GOOGLE_APPLICATION_CREDENTIALS"} {
		fmt.Printf("%s=%s\n", name, os.Getenv(name))
	}
	code, _ := strconv.Atoi(os.Getenv("FAKE_SHELL_EXIT"))
	return code
}

type invocation struct {
	Args           []string `json:"args"`
	Verb           string   `json:"verb"`
	ActiveConfig   string   `json:"active_config_env"`
	DisablePrompts string   `json:"disable_prompts"`
	ADCEnv         string   `json:"google_application_credentials"`
	StdinDevNull   bool     `json:"stdin_devnull"`
}

func run(args []string) int {
	verb := classify(args)
	logInvocation(args, verb)

	if ms, ok := knob("FAKE_GCLOUD_SLEEP", verb); ok {
		delay, _ := strconv.Atoi(ms)
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
	if spec, ok := knob("FAKE_GCLOUD_FAIL", verb); ok {
		code, msg, _ := strings.Cut(spec, ":")
		exit, _ := strconv.Atoi(code)
		if msg != "" {
			fmt.Fprintln(os.Stderr, msg)
		}
		if exit == 0 {
			exit = 1
		}
		return exit
	}

	g := newGcloud()
	switch verb {
	case "configurations-create":
		return g.createConfig(arg(args, 3))
	case "configurations-activate":
		return g.activateConfig(arg(args, 3))
	case "configurations-delete":
		return g.deleteConfig(arg(args, 3))
	case "configurations-list":
		return g.listConfigs()
	case "config-set":
		return g.setProperty(arg(args, 2), arg(args, 3))
	case "config-get-value":
		fmt.Println(g.property(g.currentConfig(), arg(args, 2)))
		return 0
	case "auth-login":
		return g.authLogin()
	case "auth-adc-login":
		return g.adcLogin()
	case "auth-list":
		return g.authList()
	default:
		fmt.Printf("fake-gcloud: %s\n", strings.Join(args, " "))
		return 0
	}
}

func classify(args []string) string {
	joined := strings.Join(args, " ")
	switch {
	case strings.HasPrefix(joined, "config configurations create"):
		return "configurations-create"
	case strings.HasPrefix(joined, "config configurations activate"):
		return "configurations-activate"
	case strings.HasPrefix(joined, "config configurations delete"):
		return "configurations-delete"
	case strings.HasPrefix(joined, "config configurations list"):
		return "configurations-list"
	case strings.HasPrefix(joined, "config set "):
		return "config-set"
	case strings.HasPrefix(joined, "config get-value "):
		return "config-get-value"
	case strings.HasPrefix(joined, "auth application-default login"):
		return "auth-adc-login"
	case strings.HasPrefix(joined, "auth login"):
		return "auth-login"
	case strings.HasPrefix(joined, "auth list"):
		return "auth-list"
	}
	return "other"
}

func arg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func knob(env, verb string) (string, bool) {
	for _, entry := range strings.Split(os.Getenv(env), ";") {
		name, value, found := strings.Cut(entry, ":")
		if found && name == verb {
			return value, true
		}
	}
	return "", false
}

func logInvocation(args []string, verb string) {
	path := os.Getenv("FAKE_GCLOUD_LOG")
	if path == "" {
		return
	}
	record := invocation{
		Args:           args,
		Verb:           verb,
		ActiveConfig:   os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME"),
		DisablePrompts: os.Getenv("CLOUDSDK_CORE_DISABLE_PROMPTS"),
		ADCEnv:         os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		StdinDevNull:   stdinIsDevNull(),
	}
	line, _ := json.Marshal(record)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}

func stdinIsDevNull() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return stdinInfo.Mode()&os.ModeCharDevice != 0 && stdinInfo.Mode()&os.ModeNamedPipe == 0
	}
	nullInfo, err := os.Stat(os.DevNull)
	return err == nil && os.SameFile(stdinInfo, nullInfo)
}

type gcloud struct {
	dir string
}

func newGcloud() *gcloud {
	dir := os.Getenv("CLOUDSDK_CONFIG")
	if dir == "" {
		if runtime.GOOS == "windows" {
			dir = filepath.Join(os.Getenv("APPDATA"), "gcloud")
		} else {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, ".config", "gcloud")
		}
	}
	return &gcloud{dir: dir}
}

func (g *gcloud) configPath(name string) string {
	return filepath.Join(g.dir, "configurations", "config_"+name)
}

func (g *gcloud) globalActive() string {
	data, err := os.ReadFile(filepath.Join(g.dir, "active_config"))
	if err != nil {
		return "default"
	}
	return strings.TrimSpace(string(data))
}

func (g *gcloud) currentConfig() string {
	if env := os.Getenv("CLOUDSDK_ACTIVE_CONFIG_NAME"); env != "" {
		return env
	}
	return g.globalActive()
}

func (g *gcloud) exists(name string) bool {
	_, err := os.Stat(g.configPath(name))
	return err == nil
}

func (g *gcloud) createConfig(name string) int {
	if g.exists(name) {
		fmt.Fprintf(os.Stderr, "ERROR: (gcloud.config.configurations.create) Cannot create configuration [%s], it already exists.\n", name)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(g.configPath(name)), 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	os.WriteFile(g.configPath(name), nil, 0644)
	fmt.Fprintf(os.Stderr, "Created [%s].\n", name)
	return g.activateConfig(name)
}

func (g *gcloud) activateConfig(name string) int {
	if !g.exists(name) {
		fmt.Fprintf(os.Stderr, "ERROR: (gcloud.config.configurations.activate) Cannot activate configuration [%s], it does not exist.\n", name)
		return 1
	}
	os.WriteFile(filepath.Join(g.dir, "active_config"), []byte(name), 0644)
	fmt.Fprintf(os.Stderr, "Activated [%s].\n", name)
	return 0
}

func (g *gcloud) deleteConfig(name string) int {
	if name == g.globalActive() {
		fmt.Fprintf(os.Stderr, "ERROR: (gcloud.config.configurations.delete) Deleting configuration [%s], which is currently active, is not permitted.\n", name)
		return 1
	}
	if err := os.Remove(g.configPath(name)); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: (gcloud.config.configurations.delete) Cannot delete configuration [%s], it does not exist.\n", name)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Deleted [%s].\n", name)
	return 0
}

func (g *gcloud) listConfigs() int {
	entries, _ := os.ReadDir(filepath.Join(g.dir, "configurations"))
	type item struct {
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	}
	items := []item{}
	for _, e := range entries {
		name := strings.TrimPrefix(e.Name(), "config_")
		items = append(items, item{Name: name, IsActive: name == g.globalActive()})
	}
	out, _ := json.Marshal(items)
	fmt.Println(string(out))
	return 0
}

// properties reads a config file's [core] section.
func (g *gcloud) properties(name string) map[string]string {
	props := map[string]string{}
	data, err := os.ReadFile(g.configPath(name))
	if err != nil {
		return props
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, found := strings.Cut(line, "=")
		if found {
			props[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return props
}

func (g *gcloud) writeProperties(name string, props map[string]string) {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("[core]\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %s\n", k, props[k])
	}
	os.WriteFile(g.configPath(name), []byte(b.String()), 0644)
}

func (g *gcloud) property(config, key string) string {
	return g.properties(config)[strings.TrimPrefix(key, "core/")]
}

func (g *gcloud) setProperty(key, value string) int {
	config := g.currentConfig()
	if !g.exists(config) {
		fmt.Fprintf(os.Stderr, "ERROR: configuration [%s] does not exist.\n", config)
		return 1
	}
	props := g.properties(config)
	props[strings.TrimPrefix(key, "core/")] = value
	g.writeProperties(config, props)
	fmt.Fprintf(os.Stderr, "Updated property [core/%s].\n", strings.TrimPrefix(key, "core/"))
	return 0
}

func (g *gcloud) loginAccount() string {
	if account := os.Getenv("FAKE_GCLOUD_ACCOUNT"); account != "" {
		return account
	}
	return strings.TrimSuffix(g.currentConfig(), "-config") + "@example.com"
}

func (g *gcloud) credentialsPath() string {
	return filepath.Join(g.dir, "fake_credentialed_accounts.json")
}

func (g *gcloud) credentialed() []string {
	var accounts []string
	data, err := os.ReadFile(g.credentialsPath())
	if err == nil {
		json.Unmarshal(data, &accounts)
	}
	return accounts
}

func (g *gcloud) authLogin() int {
	config := g.currentConfig()
	if !g.exists(config) {
		fmt.Fprintf(os.Stderr, "ERROR: configuration [%s] does not exist.\n", config)
		return 1
	}
	account := g.loginAccount()
	props := g.properties(config)
	props["account"] = account
	g.writeProperties(config, props)

	accounts := g.credentialed()
	found := false
	for _, a := range accounts {
		found = found || a == account
	}
	if !found {
		accounts = append(accounts, account)
	}
	data, _ := json.Marshal(accounts)
	os.MkdirAll(g.dir, 0755)
	os.WriteFile(g.credentialsPath(), data, 0600)
	fmt.Fprintf(os.Stderr, "You are now logged in as [%s].\n", account)
	return 0
}

// missingCodeFailure reports whether this ADC login is one of the first
// FAKE_GCLOUD_ADC_MISSING_CODE attempts, counting attempts in a file.
func (g *gcloud) missingCodeFailure() bool {
	limit, _ := strconv.Atoi(os.Getenv("FAKE_GCLOUD_ADC_MISSING_CODE"))
	if limit <= 0 {
		return false
	}
	counter := filepath.Join(g.dir, "fake_adc_attempts")
	data, _ := os.ReadFile(counter)
	attempts, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	attempts++
	os.MkdirAll(g.dir, 0755)
	os.WriteFile(counter, []byte(strconv.Itoa(attempts)), 0600)
	return attempts <= limit
}

func (g *gcloud) adcLogin() int {
	if g.missingCodeFailure() {
		fmt.Fprintln(os.Stderr, "Your browser has been opened to visit:")
		fmt.Fprintln(os.Stderr, "    https://accounts.google.com/o/oauth2/auth?response_type=code&state=SECRETSTATE&code_challenge=SECRETCHALLENGE")
		fmt.Fprintln(os.Stderr, "Redirected to http://localhost:8085/?state=SECRETSTATE&code=SECRETVALUE&scope=email")
		fmt.Fprintln(os.Stderr, "ERROR: (gcloud.auth.application-default.login) There was a problem with web authentication.")
		fmt.Fprintln(os.Stderr, "(missing_code) Missing code parameter in response.")
		return 1
	}
	path := filepath.Join(g.dir, "application_default_credentials.json")
	if os.Getenv("FAKE_GCLOUD_ADC_UNCHANGED") == "1" {
		fmt.Fprintf(os.Stderr, "Credentials saved to file: [%s]\n", path)
		return 0
	}
	adc := map[string]string{
		"client_id":     "fake-client-id",
		"client_secret": "fake-client-secret",
		"refresh_token": fmt.Sprintf("fake-refresh-token-%s-%d", g.loginAccount(), time.Now().UnixNano()),
		"type":          "authorized_user",
	}
	data, _ := json.MarshalIndent(adc, "", "  ")
	os.MkdirAll(g.dir, 0755)
	if err := os.WriteFile(path, data, 0600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "Credentials saved to file: [%s]\n", path)
	return 0
}

func (g *gcloud) authList() int {
	type item struct {
		Account string `json:"account"`
		Status  string `json:"status"`
	}
	active := g.property(g.currentConfig(), "account")
	items := []item{}
	for _, account := range g.credentialed() {
		status := "CREDENTIALED"
		if account == active {
			status = "ACTIVE"
		}
		items = append(items, item{Account: account, Status: status})
	}
	out, _ := json.Marshal(items)
	fmt.Println(string(out))
	return 0
}
