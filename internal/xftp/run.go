package xftp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var (
	Version = "1.0.0"
)

const usageText = `usage: xftp [credential-id|username]

Connect to the current ctx target using a stored FTP credential when available.

arguments:
  credential-id  use a credential by ID
  username       use a credential by username

options:
  -h, --help     show this help
  -V, --version  show version`

type ExitCodeError struct {
	Code int
}

func (err ExitCodeError) Error() string {
	return fmt.Sprintf("exit code %d", err.Code)
}

type commandRunner interface {
	LookPath(file string) (string, error)
	Output(name string, args ...string) ([]byte, []byte, error)
	Run(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error
}

type realRunner struct{}

func (realRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (realRunner) Output(name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (realRunner) Run(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command(name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ExitCodeError{Code: exitErr.ExitCode()}
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

func RunWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	app := New(realRunner{}, stdin, stdout, stderr)
	app.state = fileCredentialState{}
	app.logger = ctxCommandLogger{runner: app.runner}
	return app.Run(args)
}

func New(runner commandRunner, stdin io.Reader, stdout, stderr io.Writer) *App {
	return &App{
		runner: runner,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		state:  noopCredentialState{},
		logger: noopCommandLogger{},
	}
}

func (app *App) Run(args []string) error {
	commandArgs := args[1:]
	if len(commandArgs) > 1 {
		return app.errorf("usage: xftp [credential-id|username]")
	}
	if len(commandArgs) == 1 {
		switch commandArgs[0] {
		case "-h", "--help":
			_, err := fmt.Fprintln(app.stdout, usageText)
			return err
		case "-V", "--version":
			_, err := fmt.Fprintf(app.stdout, "xftp %s\n", Version)
			return err
		}
	}

	if err := app.requireCommands("ctx", "lftp"); err != nil {
		return err
	}

	prompt, err := ctxJSON[PromptData](app.runner, "prompt", "--format", "json", "--format-version", "1")
	if err != nil {
		return app.errorf("%s", err.Error())
	}
	if !prompt.Active {
		return app.errorf("no active workspace")
	}
	if prompt.TargetIP == nil || strings.TrimSpace(*prompt.TargetIP) == "" {
		return app.errorf("no primary target")
	}
	targetIP := strings.TrimSpace(*prompt.TargetIP)

	credentialData, err := ctxJSON[CredentialData](app.runner, "credential", "ls", "ftp", "--format", "json", "--format-version", "1")
	if err != nil {
		return app.errorf("%s", err.Error())
	}
	credentials := ftpCredentials(credentialData.Credentials)
	credential, err := app.resolveCredential(commandArgs, credentials, targetIP)
	if err != nil {
		return err
	}

	serviceData, err := ctxJSON[ServiceData](app.runner, "service", "ls", "--format", "json", "--format-version", "1")
	if err != nil {
		return app.errorf("%s", err.Error())
	}
	port, err := app.resolveFTPPort(serviceData.Services)
	if err != nil {
		return err
	}

	if credential == nil {
		_, _ = fmt.Fprintf(app.stdout, "Connecting to %s:%d...\n", targetIP, port)
	} else {
		_, _ = fmt.Fprintf(app.stdout, "Connecting to %s@%s:%d...\n", credential.Username, targetIP, port)
	}
	startedAt := time.Now().UTC()
	logID, err := app.logger.Start("xftp", ftpLogCommand(credential, targetIP, port), startedAt.Format(time.RFC3339Nano))
	if err != nil {
		return app.errorf("failed to start FTP log: %s", err.Error())
	}
	var commandStdout, commandStderr bytes.Buffer
	streamStdout := io.MultiWriter(app.stdout, &commandStdout)
	streamStderr := io.MultiWriter(app.stderr, &commandStderr)
	err = app.connect(credential, targetIP, port, streamStdout, streamStderr)
	status := "success"
	exitCode := 0
	if err != nil {
		status = "failed"
		exitCode = commandExitCode(err)
	}
	finishErr := app.logger.Finish(logID, status, exitCode, commandStdout.String(), commandStderr.String(), time.Now().UTC().Format(time.RFC3339Nano))
	if finishErr != nil {
		return app.errorf("failed to finish FTP log: %s", finishErr.Error())
	}
	if err == nil && credential != nil && credential.ID != 0 {
		_ = app.state.Save(credential.ID)
	}
	return err
}

func (app *App) requireCommands(commands ...string) error {
	for _, command := range commands {
		if _, err := app.runner.LookPath(command); err != nil {
			return app.errorf("%s is required", command)
		}
	}
	return nil
}

func (app *App) resolveCredential(args []string, credentials []Credential, targetIP string) (*Credential, error) {
	if len(args) == 0 {
		if len(credentials) == 0 {
			return nil, nil
		}
		if len(credentials) == 1 {
			return &credentials[0], nil
		}
		return app.selectCredential(credentials, targetIP)
	}

	filter := args[0]
	if isDigits(filter) {
		id, _ := strconv.ParseInt(filter, 10, 64)
		for _, credential := range credentials {
			if credential.ID == id {
				return &credential, nil
			}
		}
		return nil, app.errorf("FTP credential not found: %s", filter)
	}

	matches := make([]Credential, 0)
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
	return app.selectCredential(matches, targetIP)
}

func (app *App) selectCredential(credentials []Credential, targetIP string) (*Credential, error) {
	lastID, _ := app.state.Load()
	defaultIndex := -1
	_, _ = fmt.Fprintln(app.stdout, "Select an FTP credential:")
	_, _ = fmt.Fprintln(app.stdout)
	for i, credential := range credentials {
		marker := ""
		if credential.ID == lastID {
			defaultIndex = i
			marker = " (default)"
		}
		_, _ = fmt.Fprintf(app.stdout, "  %d) %s@%s%s\n", i+1, credential.Username, targetIP, marker)
	}
	_, _ = fmt.Fprintln(app.stdout)
	index, err := app.selectIndex(len(credentials), defaultIndex)
	if err != nil {
		return nil, err
	}
	return &credentials[index], nil
}

func (app *App) resolveFTPPort(services []Service) (int, error) {
	ports := ftpPorts(services)
	if len(ports) == 0 {
		return 21, nil
	}
	if len(ports) == 1 {
		return ports[0].Port, nil
	}

	_, _ = fmt.Fprintln(app.stdout, "Select an FTP port:")
	_, _ = fmt.Fprintln(app.stdout)
	for i, port := range ports {
		_, _ = fmt.Fprintf(app.stdout, "  %d) %d/%s\n", i+1, port.Port, port.Protocol)
	}
	_, _ = fmt.Fprintln(app.stdout)
	index, err := app.selectIndex(len(ports))
	if err != nil {
		return 0, err
	}
	return ports[index].Port, nil
}

func (app *App) selectIndex(count int, defaults ...int) (int, error) {
	defaultIndex := -1
	if len(defaults) > 0 {
		defaultIndex = defaults[0]
	}
	reader := bufio.NewReader(app.stdin)
	for {
		_, _ = fmt.Fprintf(app.stdout, "Select [1-%d]: ", count)
		input, err := readSelectionLine(reader)
		if err != nil {
			if err == io.EOF {
				return 0, app.errorf("cancelled")
			}
			return 0, err
		}
		input = strings.TrimSpace(input)
		if input == "" {
			if defaultIndex >= 0 && defaultIndex < count {
				return defaultIndex, nil
			}
			return 0, app.errorf("cancelled")
		}
		selection, err := strconv.Atoi(input)
		if err != nil || selection < 1 || selection > count {
			continue
		}
		return selection - 1, nil
	}
}

func readSelectionLine(reader *bufio.Reader) (string, error) {
	var input strings.Builder
	for {
		character, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF && input.Len() > 0 {
				return input.String(), nil
			}
			return "", err
		}
		if character == '\r' || character == '\n' {
			if character == '\r' {
				next, nextErr := reader.ReadByte()
				if nextErr == nil && next != '\n' {
					_ = reader.UnreadByte()
				}
			}
			return input.String(), nil
		}
		input.WriteByte(character)
	}
}

func (app *App) connect(credential *Credential, targetIP string, port int, stdout, stderr io.Writer) error {
	if credential == nil {
		args := []string{"-p", strconv.Itoa(port), targetIP, "-e", lftpAnonymousCommand}
		return app.runner.Run("lftp", args, nil, app.stdin, stdout, stderr)
	}
	args := []string{"-u", credential.Username, "-p", strconv.Itoa(port), targetIP}
	if credential.Password != nil {
		args = append([]string{"--env-password", "-e", lftpAuthenticationCommand}, args...)
		return app.runner.Run("lftp", args, []string{"LFTP_PASSWORD=" + *credential.Password}, app.stdin, stdout, stderr)
	}
	args = append(args, "-e", lftpAuthenticationCommand)
	return app.runner.Run("lftp", args, nil, app.stdin, stdout, stderr)
}

const (
	lftpAuthenticationCommand = "set net:max-retries 0; set cmd:fail-exit yes; quote NOOP"
	lftpAnonymousCommand      = "set net:max-retries 0"
)

func ftpLogCommand(credential *Credential, targetIP string, port int) string {
	destination := targetIP
	if credential != nil && credential.Username != "" {
		destination = credential.Username + "@" + targetIP
	}
	return fmt.Sprintf("lftp -p %d %s", port, destination)
}

func (app *App) errorf(format string, args ...any) error {
	_, _ = fmt.Fprintf(app.stderr, "xftp: "+format+"\n", args...)
	return ExitCodeError{Code: 1}
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

type APIResponse[T any] struct {
	Success       bool      `json:"success"`
	FormatVersion *string   `json:"format_version"`
	Data          *T        `json:"data"`
	Error         *APIError `json:"error"`
}

type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

type PromptData struct {
	Active         bool    `json:"active"`
	TargetIP       *string `json:"target_ip"`
	TargetName     *string `json:"target_name"`
	WorkspaceID    *string `json:"workspace_id"`
	WorkspaceName  *string `json:"workspace_name"`
	WorkspacePath  *string `json:"workspace_path"`
	LocalIP        *string `json:"local_ip"`
	LocalInterface *string `json:"local_interface"`
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
	ID          int64   `json:"id"`
	Port        int     `json:"port"`
	Protocol    string  `json:"protocol"`
	ServiceName *string `json:"service_name"`
}

func ctxJSON[T any](runner commandRunner, args ...string) (*T, error) {
	stdout, stderr, runErr := runner.Output("ctx", args...)

	var response APIResponse[T]
	if err := json.Unmarshal(stdout, &response); err != nil {
		message := "invalid JSON from ctx"
		if runErr != nil && len(stderr) > 0 {
			message = fmt.Sprintf("%s: %s", message, strings.TrimSpace(string(stderr)))
		}
		return nil, errors.New(message)
	}

	if !response.Success {
		if response.Error != nil && strings.TrimSpace(response.Error.Message) != "" {
			return nil, errors.New(response.Error.Message)
		}
		return nil, errors.New("ctx command failed")
	}
	if response.FormatVersion == nil || !strings.HasPrefix(*response.FormatVersion, "1.") {
		return nil, errors.New("unsupported ctx JSON format version")
	}
	if response.Data == nil {
		return nil, errors.New("ctx response missing data")
	}
	return response.Data, nil
}

func ftpCredentials(credentials []Credential) []Credential {
	filtered := make([]Credential, 0, len(credentials))
	for _, credential := range credentials {
		if credential.Scope == "ftp" {
			filtered = append(filtered, credential)
		}
	}
	return filtered
}

func ftpPorts(services []Service) []Service {
	ports := make([]Service, 0, len(services))
	for _, service := range services {
		if !strings.EqualFold(service.Protocol, "tcp") {
			continue
		}
		if service.ServiceName == nil || !strings.EqualFold(strings.TrimSpace(*service.ServiceName), "ftp") {
			continue
		}
		ports = append(ports, service)
	}
	return ports
}
