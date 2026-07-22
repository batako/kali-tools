package xscp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"req/internal/ctxapi"
	"req/internal/ctxexec"
	"req/internal/onlinehelp"
)

var (
	Version = "1.1.1"
)

const usageText = `usage: xscp <upload|download> <source> [destination] [credential-id|username]

Transfer files between the current ctx target and the local machine over SSH.

commands:
  upload    copy a local file to the target
  download  copy a remote file to the local machine

arguments:
  credential-id  use a credential by ID
  username       use a credential by username

options:
  -p, --port      override the SSH port
  --service       select a discovered SSH service by number
  -h, --help     show this help
  -V, --version  show version
  --online-help  show the versioned online help URL`

type parsedOptions struct {
	Action      string
	Source      string
	Destination string
	Credential  string
	Port        string
	Service     string
}

type ExitCodeError struct{ CodeValue int }

func (err ExitCodeError) Error() string { return fmt.Sprintf("exit code %d", err.CodeValue) }
func (err ExitCodeError) Code() int     { return err.CodeValue }
func (err ExitCodeError) ExitCode() int { return err.CodeValue }

type commandRunner interface {
	LookPath(string) (string, error)
	Run(string, []string, []string, io.Reader, io.Writer, io.Writer) error
}

type realRunner struct{}

func (realRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }
func (realRunner) Run(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command(name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ExitCodeError{CodeValue: exitErr.ExitCode()}
		}
		return err
	}
	return nil
}

type App struct {
	runner commandRunner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	state  credentialState
	logger commandLogger
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	app := New(realRunner{}, stdin, stdout, stderr)
	app.state = fileCredentialState{}
	app.logger = ctxCommandLogger{runner: app.runner}
	return app.run(args)
}

func New(runner commandRunner, stdin io.Reader, stdout, stderr io.Writer) *App {
	return &App{runner: runner, stdin: stdin, stdout: stdout, stderr: stderr, state: noopCredentialState{}, logger: noopCommandLogger{}}
}

func (app *App) run(args []string) error {
	options, err := parseOptions(args[1:])
	if err != nil {
		return app.errorf("%s", err)
	}
	if options.Action == "help" {
		_, err := fmt.Fprintln(app.stdout, usageText)
		return err
	}
	if options.Action == "online-help" {
		return onlinehelp.Print(app.stdout, "xscp", Version)
	}
	if options.Action == "version" {
		_, err := fmt.Fprintf(app.stdout, "xscp %s\n", Version)
		return err
	}
	if _, err := ctxexec.LookPath(app.runner); err != nil {
		return app.errorf("ctx is required")
	}
	if err := app.requireCommands("scp"); err != nil {
		return err
	}
	prompt, err := ctxJSON[PromptData](app.runner, "prompt")
	if err != nil {
		return app.errorf("%s", err)
	}
	if !prompt.Active {
		return app.errorf("no active workspace")
	}
	if prompt.TargetIP == nil || strings.TrimSpace(*prompt.TargetIP) == "" {
		return app.errorf("no primary target")
	}
	targetIP := strings.TrimSpace(*prompt.TargetIP)
	credentialData, err := ctxJSON[CredentialData](app.runner, "credential", "ls", "ssh")
	if err != nil {
		return app.errorf("%s", err)
	}
	credential, err := app.resolveCredential(options.Credential, credentialData.Credentials, targetIP)
	if err != nil {
		return err
	}
	serviceData, err := ctxJSON[ServiceData](app.runner, "service", "ls")
	if err != nil {
		return app.errorf("%s", err)
	}
	port, err := app.resolveSSHPort(serviceData.Services, options.Port, options.Service)
	if err != nil {
		return err
	}
	if credential != nil && credential.Password != nil {
		if err := app.requireCommands("sshpass"); err != nil {
			return err
		}
	}
	local, remote := options.Source, options.Destination
	if options.Action == "download" {
		local, remote = options.Destination, options.Source
	}
	start := time.Now().UTC()
	expanded := scpLogCommand(options.Action, local, remote, credential, targetIP, port)
	logID, err := app.logger.Start("xscp "+strings.Join(args[1:], " "), expanded, start.Format(time.RFC3339Nano))
	if err != nil {
		return app.errorf("failed to start SCP log: %s", err)
	}
	_, _ = fmt.Fprintf(app.stdout, "Transferring %s %s...\n", options.Action, remote)
	var commandStdout, commandStderr bytes.Buffer
	scpArgs := app.scpArgs(options.Action, local, remote, credential, targetIP, port)
	commandName, commandArgsForRun := app.connectCommand(credential, scpArgs)
	runErr := app.runner.Run(commandName, commandArgsForRun, app.scpEnv(credential), app.stdin, io.MultiWriter(app.stdout, &commandStdout), io.MultiWriter(app.stderr, &commandStderr))
	status, exitCode := "success", 0
	if runErr != nil {
		status, exitCode = "failed", commandExitCode(runErr)
	}
	if finishErr := app.logger.Finish(logID, status, exitCode, commandStdout.String(), commandStderr.String(), time.Now().UTC().Format(time.RFC3339Nano)); finishErr != nil {
		return app.errorf("failed to finish SCP log: %s", finishErr)
	}
	if runErr == nil && credential != nil && credential.ID != 0 {
		_ = app.state.Save(credential.ID)
	}
	return runErr
}

func (app *App) scpArgs(direction, local, remote string, credential *Credential, target string, port int) []string {
	remotePath := target + ":" + remote
	if credential != nil {
		remotePath = credential.Username + "@" + remotePath
	}
	if direction == "upload" {
		return []string{"-P", strconv.Itoa(port), local, remotePath}
	}
	return []string{"-P", strconv.Itoa(port), remotePath, local}
}

func (app *App) scpEnv(credential *Credential) []string {
	if credential == nil || credential.Password == nil {
		return nil
	}
	return []string{"SSHPASS=" + *credential.Password}
}

func (app *App) connectCommand(credential *Credential, args []string) (string, []string) {
	if credential != nil && credential.Password != nil {
		return "sshpass", append([]string{"-e", "scp"}, args...)
	}
	return "scp", args
}

func (app *App) requireCommands(commands ...string) error {
	for _, command := range commands {
		if _, err := app.runner.LookPath(command); err != nil {
			return app.errorf("%s is required", command)
		}
	}
	return nil
}

func (app *App) resolveCredential(filter string, credentials []Credential, target string) (*Credential, error) {
	credentials = sshCredentials(credentials)
	if filter == "" {
		if len(credentials) == 0 {
			return nil, nil
		}
		if len(credentials) == 1 {
			return &credentials[0], nil
		}
		return app.selectCredential(credentials, target)
	}
	if isDigits(filter) {
		id, _ := strconv.ParseInt(filter, 10, 64)
		for _, credential := range credentials {
			if credential.ID == id {
				return &credential, nil
			}
		}
		return nil, app.errorf("SSH credential not found: %s", filter)
	}
	var matches []Credential
	for _, credential := range credentials {
		if credential.Username == filter {
			matches = append(matches, credential)
		}
	}
	if len(matches) == 0 {
		return &Credential{Username: filter}, nil
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}
	return app.selectCredential(matches, target)
}

func (app *App) selectCredential(credentials []Credential, target string) (*Credential, error) {
	lastID, _ := app.state.Load()
	_, _ = fmt.Fprintln(app.stdout, "Select an SSH credential:")
	_, _ = fmt.Fprintln(app.stdout)
	defaultIndex := -1
	for i, credential := range credentials {
		marker := ""
		if credential.ID == lastID {
			marker, defaultIndex = " (default)", i
		}
		_, _ = fmt.Fprintf(app.stdout, "  %d) %s@%s%s\n", i+1, credential.Username, target, marker)
	}
	_, _ = fmt.Fprintln(app.stdout)
	index, err := app.selectIndex(len(credentials), defaultIndex)
	if err != nil {
		return nil, err
	}
	return &credentials[index], nil
}

func (app *App) resolveSSHPort(services []Service, explicit, selection string) (int, error) {
	if explicit != "" {
		port, err := strconv.Atoi(explicit)
		if err != nil || port < 1 || port > 65535 {
			return 0, app.errorf("invalid --port")
		}
		if selection != "" {
			return 0, app.errorf("--port cannot be combined with --service")
		}
		return port, nil
	}
	ports := sshPorts(services)
	if len(ports) == 0 {
		return 22, nil
	}
	if len(ports) == 1 {
		return ports[0].Port, nil
	}
	_, _ = fmt.Fprintln(app.stdout, "Select an SSH port:")
	_, _ = fmt.Fprintln(app.stdout)
	for i, port := range ports {
		_, _ = fmt.Fprintf(app.stdout, "  %d) %d/%s\n", i+1, port.Port, port.Protocol)
	}
	_, _ = fmt.Fprintf(app.stdout, "\nSelect [1-%d]: ", len(ports))
	index, err := app.selectIndex(len(ports))
	if err != nil {
		return 0, err
	}
	return ports[index].Port, nil
}

func parseOptions(args []string) (parsedOptions, error) {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return parsedOptions{Action: "help"}, nil
	}
	if len(args) == 1 && args[0] == "--online-help" {
		return parsedOptions{Action: "online-help"}, nil
	}
	if len(args) == 1 && (args[0] == "-V" || args[0] == "--version") {
		return parsedOptions{Action: "version"}, nil
	}
	if len(args) == 0 || (args[0] != "upload" && args[0] != "download") {
		return parsedOptions{}, errors.New("usage: xscp <upload|download> <source> [destination] [credential-id|username]")
	}
	options := parsedOptions{Action: args[0]}
	var positional []string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-p", "--port", "--service":
			option := args[i]
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, fmt.Errorf("%s requires a value", args[i])
			}
			i++
			if option == "--service" {
				options.Service = args[i]
			} else {
				options.Port = args[i]
			}
		case "-h", "--help", "-V", "--version":
			return parsedOptions{}, errors.New("help and version must be used alone")
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) < 1 || len(positional) > 3 {
		return parsedOptions{}, errors.New("usage: xscp <upload|download> <source> [destination] [credential-id|username]")
	}
	options.Source = positional[0]
	options.Destination = filepath.Base(options.Source)
	if len(positional) >= 2 {
		options.Destination = positional[1]
	}
	if len(positional) == 3 {
		options.Credential = positional[2]
	}
	return options, nil
}

func (app *App) selectIndex(count int, defaults ...int) (int, error) {
	defaultIndex := -1
	if len(defaults) > 0 {
		defaultIndex = defaults[0]
	}
	reader := bufio.NewReader(app.stdin)
	for {
		_, _ = fmt.Fprintf(app.stdout, "Select [1-%d]: ", count)
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return 0, app.errorf("cancelled")
		}
		line = strings.TrimSpace(line)
		if line == "" && defaultIndex >= 0 {
			return defaultIndex, nil
		}
		choice, convErr := strconv.Atoi(line)
		if convErr == nil && choice >= 1 && choice <= count {
			return choice - 1, nil
		}
	}
}

func (app *App) errorf(format string, args ...any) error {
	_, _ = fmt.Fprintf(app.stderr, "xscp: "+format+"\n", args...)
	return ExitCodeError{CodeValue: 1}
}

func commandExitCode(err error) int {
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.CodeValue
	}
	return 1
}

func scpLogCommand(direction, local, remote string, credential *Credential, target string, port int) string {
	remotePath := target + ":" + remote
	if credential != nil {
		remotePath = credential.Username + "@" + remotePath
	}
	if direction == "upload" {
		return fmt.Sprintf("scp -P %d %s %s", port, local, remotePath)
	}
	return fmt.Sprintf("scp -P %d %s %s", port, remotePath, local)
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

type PromptData struct {
	Active   bool    `json:"active"`
	TargetIP *string `json:"target_ip"`
}
type CredentialData struct {
	Credentials []Credential `json:"credentials"`
}
type Credential struct {
	ID       int64   `json:"id"`
	Scope    string  `json:"scope"`
	Username string  `json:"username"`
	Password *string `json:"password"`
}
type ServiceData struct {
	Services []Service `json:"services"`
}
type Service struct {
	Port        int     `json:"port"`
	Protocol    string  `json:"protocol"`
	ServiceName *string `json:"service_name"`
}

func ctxJSON[T any](runner commandRunner, args ...string) (*T, error) {
	result, err := ctxapi.Call[T](ctxapi.NewV1(runner), args...)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

func sshCredentials(credentials []Credential) []Credential {
	result := make([]Credential, 0)
	for _, credential := range credentials {
		if credential.Scope == "ssh" {
			result = append(result, credential)
		}
	}
	return result
}
func sshPorts(services []Service) []Service {
	result := make([]Service, 0)
	for _, service := range services {
		if strings.EqualFold(service.Protocol, "tcp") && service.ServiceName != nil && strings.EqualFold(strings.TrimSpace(*service.ServiceName), "ssh") {
			result = append(result, service)
		}
	}
	return result
}
