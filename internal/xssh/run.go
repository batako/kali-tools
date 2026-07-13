package xssh

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
)

var (
	Version = "0.1.0"
)

const usageText = `usage: xssh [credential-id|username]

Connect to the current ctx target using a stored SSH credential when available.

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
}

func RunWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	app := New(realRunner{}, stdin, stdout, stderr)
	app.state = fileCredentialState{}
	return app.Run(args)
}

func New(runner commandRunner, stdin io.Reader, stdout, stderr io.Writer) *App {
	return &App{
		runner: runner,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		state:  noopCredentialState{},
	}
}

func (app *App) Run(args []string) error {
	commandArgs := args[1:]
	if len(commandArgs) > 1 {
		return app.errorf("usage: xssh [credential-id|username]")
	}
	if len(commandArgs) == 1 {
		switch commandArgs[0] {
		case "-h", "--help":
			_, err := fmt.Fprintln(app.stdout, usageText)
			return err
		case "-V", "--version":
			_, err := fmt.Fprintf(app.stdout, "xssh %s\n", Version)
			return err
		}
	}

	if err := app.requireCommands("ctx", "ssh"); err != nil {
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

	credentialData, err := ctxJSON[CredentialData](app.runner, "credential", "ls", "ssh", "--format", "json", "--format-version", "1")
	if err != nil {
		return app.errorf("%s", err.Error())
	}
	credentials := sshCredentials(credentialData.Credentials)
	credential, err := app.resolveCredential(commandArgs, credentials, targetIP)
	if err != nil {
		return err
	}

	serviceData, err := ctxJSON[ServiceData](app.runner, "service", "ls", "--format", "json", "--format-version", "1")
	if err != nil {
		return app.errorf("%s", err.Error())
	}
	port, err := app.resolveSSHPort(serviceData.Services)
	if err != nil {
		return err
	}

	if credential != nil && credential.Password != nil {
		if err := app.requireCommands("sshpass"); err != nil {
			return err
		}
	}
	if credential == nil {
		_, _ = fmt.Fprintf(app.stdout, "Connecting to %s:%d...\n", targetIP, port)
	} else {
		_, _ = fmt.Fprintf(app.stdout, "Connecting to %s@%s:%d...\n", credential.Username, targetIP, port)
	}
	err = app.connect(credential, targetIP, port)
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
		return nil, app.errorf("SSH credential not found: %s", filter)
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
	_, _ = fmt.Fprintln(app.stdout, "Select an SSH credential:")
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

func (app *App) resolveSSHPort(services []Service) (int, error) {
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
	scanner := bufio.NewScanner(app.stdin)
	for {
		_, _ = fmt.Fprintf(app.stdout, "Select [1-%d]: ", count)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return 0, err
			}
			return 0, app.errorf("cancelled")
		}
		input := strings.TrimSpace(scanner.Text())
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

func (app *App) connect(credential *Credential, targetIP string, port int) error {
	if credential == nil {
		return app.runner.Run("ssh", []string{"-p", strconv.Itoa(port), targetIP}, nil, app.stdin, app.stdout, app.stderr)
	}
	destination := fmt.Sprintf("%s@%s", credential.Username, targetIP)
	args := []string{"-p", strconv.Itoa(port), destination}
	if credential.Password != nil {
		return app.runner.Run("sshpass", append([]string{"-e", "ssh"}, args...), []string{"SSHPASS=" + *credential.Password}, app.stdin, app.stdout, app.stderr)
	}
	return app.runner.Run("ssh", args, nil, app.stdin, app.stdout, app.stderr)
}

func (app *App) errorf(format string, args ...any) error {
	_, _ = fmt.Fprintf(app.stderr, "xssh: "+format+"\n", args...)
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

func sshCredentials(credentials []Credential) []Credential {
	filtered := make([]Credential, 0, len(credentials))
	for _, credential := range credentials {
		if credential.Scope == "ssh" {
			filtered = append(filtered, credential)
		}
	}
	return filtered
}

func sshPorts(services []Service) []Service {
	ports := make([]Service, 0, len(services))
	for _, service := range services {
		if !strings.EqualFold(service.Protocol, "tcp") {
			continue
		}
		if service.ServiceName == nil || !strings.EqualFold(strings.TrimSpace(*service.ServiceName), "ssh") {
			continue
		}
		ports = append(ports, service)
	}
	return ports
}
