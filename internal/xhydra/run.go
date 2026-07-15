package xhydra

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"req/internal/ctx"
	request "req/internal/req"
)

var (
	Version = "1.0.0"
)

const usageText = `usage: xhydra http [options]

Run Hydra against an HTTP form and save successful credentials to ctx.

options:
  -u, --username <name>       username to test
  -r, --request <file>        use a raw HTTP request template
  --url <url>                 URL to test without a request file
  --data <body>               additional form body to test without a request file
  --user-field <name>         username form field (or use ^USER^ in the body)
  --password-field <name>     password form field (or use ^PASS^ in the body)
  --fail-json <field=value>   JSON response value that means authentication failed
  --success-json <field=value> JSON response value that means authentication succeeded
  --fail-body <text>          response body text that means authentication failed
  --success-body <text>       response body text that means authentication succeeded
  --fail-status <code>        HTTP status that means authentication failed (401)
  --success-redirect           treat HTTP 302 responses as authentication success
  -P, --password-list <path>  use a password list instead of automatic selection
  -h, --help                  show this help
  -V, --version               show version`

type ExitCodeError struct{ Code int }

func (err ExitCodeError) Error() string { return fmt.Sprintf("exit code %d", err.Code) }

type parsedOptions struct {
	Mode            string
	Username        string
	RequestFile     string
	URL             string
	Data            string
	UserField       string
	PasswordField   string
	FailJSON        string
	SuccessJSON     string
	FailBody        string
	SuccessBody     string
	FailStatus      string
	SuccessRedirect bool
	PasswordList    string
}

type runner interface {
	LookPath(file string) (string, error)
	Run(name string, args []string, stdout, stderr io.Writer) error
}

type realRunner struct{}

func (realRunner) LookPath(file string) (string, error) { return exec.LookPath(file) }

func (realRunner) Run(name string, args []string, stdout, stderr io.Writer) error {
	command := exec.Command(name, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ExitCodeError{Code: exitErr.ExitCode()}
		}
		return err
	}
	return nil
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return newApp(realRunner{}, stdin, stdout, stderr).run(args)
}

type app struct {
	runner runner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func newApp(commandRunner runner, stdin io.Reader, stdout, stderr io.Writer) *app {
	return &app{runner: commandRunner, stdin: stdin, stdout: stdout, stderr: stderr}
}

func (app *app) run(args []string) error {
	options, err := parseOptions(args[1:])
	if err != nil {
		return err
	}
	if options.Mode == "help" {
		_, err := fmt.Fprintln(app.stdout, usageText)
		return err
	}
	if options.Mode == "version" {
		_, err := fmt.Fprintf(app.stdout, "xhydra %s\n", Version)
		return err
	}
	if options.Mode != "http" {
		return errors.New("usage: xhydra http [options]")
	}
	if _, err := app.runner.LookPath("hydra"); err != nil {
		return errors.New("hydra is required; install the hydra package")
	}
	if options.Username == "" {
		return errors.New("username is required: use -u <name>")
	}
	if options.RequestFile != "" && (options.URL != "" || options.Data != "") {
		return errors.New("usage: --request cannot be combined with --url or --data")
	}
	if options.RequestFile == "" && options.URL == "" {
		return errors.New("request input is required: use --request or --url")
	}
	template, err := loadTemplate(options)
	if err != nil {
		return err
	}
	if template.Method != "POST" {
		return fmt.Errorf("unsupported request method: %s; xhydra http supports POST only", template.Method)
	}
	form, err := formSpec(template.Body, options.UserField, options.PasswordField)
	if err != nil {
		return err
	}
	condition, optional, err := responseCondition(options)
	if err != nil {
		return err
	}
	passwordList, cleanup, err := resolvePasswordList(options.PasswordList)
	if err != nil {
		return err
	}
	defer cleanup()

	workspace, err := loadWorkspace()
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	hydraArgs := buildHydraArgs(template, form, condition, optional, passwordList, options.Username)
	logID, err := ctx.StartCommandLog(workspace, ctx.CommandLog{
		Command:         commandString(append([]string{"xhydra", "http"}, args[1:]...)),
		ExpandedCommand: commandString(append([]string{"hydra"}, hydraArgs...)),
		StartedAt:       startedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("failed to start command log: %w", err)
	}

	var commandStdout, commandStderr bytes.Buffer
	runErr := app.runner.Run("hydra", hydraArgs, io.MultiWriter(app.stdout, &commandStdout), io.MultiWriter(app.stderr, &commandStderr))
	endedAt := time.Now().UTC()
	exitCode := 0
	if runErr != nil {
		var exitErr ExitCodeError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.Code
		} else {
			exitCode = 1
		}
	}
	status := "success"
	if exitCode != 0 {
		status = "failed"
	}
	if finishErr := ctx.FinishCommandLog(workspace, logID, ctx.CommandLog{
		Status:   status,
		ExitCode: exitCode,
		Stdout:   commandStdout.String(),
		Stderr:   commandStderr.String(),
		EndedAt:  endedAt.Format(time.RFC3339Nano),
	}); finishErr != nil {
		return fmt.Errorf("failed to finish command log: %w", finishErr)
	}
	if runErr != nil {
		return runErr
	}

	password, ok := hydraSuccessPassword(commandStdout.String())
	if !ok {
		return errors.New("hydra completed without a successful credential")
	}
	if _, err := ctx.SetCredential(workspace, "http", options.Username, password); err != nil {
		return fmt.Errorf("failed to save HTTP credential: %w", err)
	}
	_, err = fmt.Fprintf(app.stdout, "Saved HTTP credential for %s (log: ctx log %d)\n", options.Username, logID)
	return err
}

type requestTemplate struct {
	Method string
	URL    *url.URL
	Header map[string][]string
	Body   string
}

func loadTemplate(options parsedOptions) (requestTemplate, error) {
	if options.RequestFile != "" {
		parsed, err := request.ParseFile(options.RequestFile)
		if err != nil {
			return requestTemplate{}, err
		}
		return requestTemplate{Method: parsed.Method, URL: parsed.URL, Header: parsed.Header, Body: string(parsed.Body)}, nil
	}
	parsedURL, err := url.Parse(options.URL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return requestTemplate{}, errors.New("invalid --url")
	}
	return requestTemplate{Method: "POST", URL: parsedURL, Header: map[string][]string{}, Body: options.Data}, nil
}

type formTemplate struct {
	Body string
}

const (
	userMarker     = "^USER^"
	passwordMarker = "^PASS^"
)

func formSpec(body, userField, passwordField string) (formTemplate, error) {
	if body == "" {
		if userField == "" || passwordField == "" {
			return formTemplate{}, errors.New("empty form body requires --user-field and --password-field")
		}
		body = url.Values{
			userField:     {userMarker},
			passwordField: {passwordMarker},
		}.Encode()
	}
	values, err := url.ParseQuery(body)
	if err != nil {
		return formTemplate{}, fmt.Errorf("invalid form body: %w", err)
	}
	if userField == "" {
		userField = fieldForMarker(values, userMarker)
		if userField == "" {
			return formTemplate{}, errors.New("username field not found: use --user-field or add ^USER^ to the request body")
		}
	} else if _, ok := values[userField]; !ok {
		return formTemplate{}, fmt.Errorf("username field not found: %s", userField)
	}
	if passwordField == "" {
		passwordField = fieldForMarker(values, passwordMarker)
		if passwordField == "" {
			return formTemplate{}, errors.New("password field not found: use --password-field or add ^PASS^ to the request body")
		}
	} else if _, ok := values[passwordField]; !ok {
		return formTemplate{}, fmt.Errorf("password field not found: %s", passwordField)
	}
	values.Set(userField, userMarker)
	values.Set(passwordField, passwordMarker)
	encodedBody := values.Encode()
	encodedBody = strings.ReplaceAll(encodedBody, "%5EUSER%5E", userMarker)
	encodedBody = strings.ReplaceAll(encodedBody, "%5EPASS%5E", passwordMarker)
	return formTemplate{Body: encodedBody}, nil
}

func fieldForMarker(values url.Values, marker string) string {
	for field, entries := range values {
		for _, entry := range entries {
			if entry == marker {
				return field
			}
		}
	}
	return ""
}

func jsonCondition(prefix, value string) (string, error) {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("invalid --fail-json; expected field=value")
	}
	field := strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])
	if strings.ContainsAny(field+value, "\r\n:") {
		return "", errors.New("invalid --fail-json value")
	}
	return fmt.Sprintf(`%s="%s"\:"%s"`, prefix, field, value), nil
}

func jsonFailureMarker(value string) (string, error) {
	return jsonCondition("F", value)
}

func responseCondition(options parsedOptions) (string, []string, error) {
	conditions := make([]string, 0, 1)
	if options.FailJSON != "" {
		condition, err := jsonCondition("F", options.FailJSON)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, condition)
	}
	if options.SuccessJSON != "" {
		condition, err := jsonCondition("S", options.SuccessJSON)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, condition)
	}
	if options.FailBody != "" {
		conditions = append(conditions, "F="+escapeHydraValue(options.FailBody))
	}
	if options.SuccessBody != "" {
		conditions = append(conditions, "S="+escapeHydraValue(options.SuccessBody))
	}
	if len(conditions) > 1 {
		return "", nil, errors.New("only one body or JSON response condition may be specified")
	}
	optional := make([]string, 0, 2)
	if options.FailStatus != "" {
		if options.FailStatus != "401" {
			return "", nil, errors.New("--fail-status currently supports only 401")
		}
		optional = append(optional, "1=")
	}
	if options.SuccessRedirect {
		optional = append(optional, "2=")
	}
	if len(conditions) == 0 && len(optional) == 0 {
		return "", nil, errors.New("a response condition is required")
	}
	condition := ""
	if len(conditions) == 1 {
		condition = conditions[0]
	}
	return condition, optional, nil
}

func escapeHydraValue(value string) string {
	return strings.ReplaceAll(value, ":", "\\:")
}

func buildHydraArgs(template requestTemplate, form formTemplate, condition string, optional []string, passwordList, username string) []string {
	module := "http-post-form"
	if template.URL.Scheme == "https" {
		module = "https-post-form"
	}
	port := template.URL.Port()
	args := []string{"-l", username, "-P", passwordList, "-f"}
	if port != "" {
		args = append(args, "-s", port)
	}
	formSpec := template.URL.RequestURI() + ":" + form.Body
	for name, values := range template.Header {
		if strings.EqualFold(name, "Cookie") || strings.EqualFold(name, "User-Agent") {
			for _, value := range values {
				formSpec += ":H=" + name + "\\: " + strings.ReplaceAll(value, ":", "\\:")
			}
		}
	}
	for _, value := range optional {
		formSpec += ":" + value
	}
	if condition != "" {
		formSpec += ":" + condition
	}
	return append(args, template.URL.Hostname(), module, formSpec)
}

func resolvePasswordList(explicit string) (string, func(), error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", func() {}, fmt.Errorf("password list not found: %s", explicit)
		}
		return explicit, func() {}, nil
	}
	root := ctx.DiscoverWordlistsRoot()
	candidates := []string{
		filepath.Join(root, "rockyou.txt"),
		filepath.Join(root, "fasttrack.txt"),
		"/usr/share/seclists/Passwords/Common-Credentials/10-million-password-list-top-1000000.txt",
		"/usr/share/seclists/Passwords/Common-Credentials/best1050.txt",
	}
	for _, candidate := range candidates {
		if candidate != "" {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, func() {}, nil
			}
		}
	}
	return "", func() {}, errors.New("no password wordlist found; install wordlists or seclists, or use --password-list")
}

func loadWorkspace() (*ctx.Workspace, error) {
	data, err := ctx.LoadPromptData(".")
	if err != nil {
		return nil, err
	}
	if !data.Active || data.WorkspacePath == "" {
		return nil, errors.New("no active workspace")
	}
	return ctx.InitWorkspace(data.WorkspacePath)
}

func hydraSuccessPassword(output string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "login:") || !strings.Contains(line, "password:") {
			continue
		}
		password := strings.TrimSpace(line[strings.Index(line, "password:")+len("password:"):])
		if password != "" {
			return password, true
		}
	}
	return "", false
}

func parseOptions(args []string) (parsedOptions, error) {
	var options parsedOptions
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return parsedOptions{Mode: "help"}, nil
	}
	if len(args) == 1 && (args[0] == "-V" || args[0] == "--version") {
		return parsedOptions{Mode: "version"}, nil
	}
	if len(args) == 0 || args[0] != "http" {
		return parsedOptions{}, errors.New("usage: xhydra http [options]")
	}
	options.Mode = "http"
	for i := 1; i < len(args); i++ {
		value := func(name string) (string, error) {
			if i+1 >= len(args) || args[i+1] == "" {
				return "", fmt.Errorf("%s requires a value", name)
			}
			i++
			return args[i], nil
		}
		switch args[i] {
		case "-u", "--username":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.Username = v
		case "-r", "--request":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.RequestFile = v
		case "--url":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.URL = v
		case "--data":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.Data = v
		case "--user-field":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.UserField = v
		case "--password-field":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.PasswordField = v
		case "--fail-json":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.FailJSON = v
		case "--success-json":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.SuccessJSON = v
		case "--fail-body":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.FailBody = v
		case "--success-body":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.SuccessBody = v
		case "--fail-status":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.FailStatus = v
		case "--success-redirect":
			options.SuccessRedirect = true
		case "-P", "--password-list":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.PasswordList = v
		default:
			return parsedOptions{}, fmt.Errorf("unknown option: %s", args[i])
		}
	}
	return options, nil
}

func commandString(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\n") {
			parts = append(parts, strconv.Quote(arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}
