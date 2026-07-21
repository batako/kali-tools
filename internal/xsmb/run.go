package xsmb

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"req/internal/ctxapi"
	"req/internal/ctxexec"
	"req/internal/onlinehelp"
)

var (
	Version = "1.1.0"
)

const usageText = `usage: xsmb [credential-id|username]

Connect to the current ctx target using a stored SMB credential when available.

arguments:
  credential-id  use a credential by ID
  username       use a credential by username

options:
  -h, --help     show this help
  -V, --version  show version
  --online-help  show the versioned online help URL`

type ExitCodeError struct {
	Code int
}

func (err ExitCodeError) Error() string {
	return fmt.Sprintf("exit code %d", err.Code)
}

func (err ExitCodeError) ExitCode() int { return err.Code }

type commandRunner interface {
	LookPath(file string) (string, error)
	Run(name string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) error
}

type realRunner struct{}

func (realRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
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
		return app.errorf("usage: xsmb [credential-id|username]")
	}
	if len(commandArgs) == 1 {
		switch commandArgs[0] {
		case "-h", "--help":
			_, err := fmt.Fprintln(app.stdout, usageText)
			return err
		case "--online-help":
			return onlinehelp.Print(app.stdout, "xsmb", Version)
		case "-V", "--version":
			_, err := fmt.Fprintf(app.stdout, "xsmb %s\n", Version)
			return err
		}
	}

	if _, err := ctxexec.LookPath(app.runner); err != nil {
		return app.errorf("ctx is required")
	}
	if err := app.requireCommands("smbclient"); err != nil {
		return err
	}

	prompt, err := ctxJSON[PromptData](app.runner, "prompt")
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

	credentialData, err := ctxJSON[CredentialData](app.runner, "credential", "ls", "smb")
	if err != nil {
		return app.errorf("%s", err.Error())
	}
	credentials := smbCredentials(credentialData.Credentials)
	credential, err := app.resolveCredential(commandArgs, credentials, targetIP)
	if err != nil {
		return err
	}

	serviceData, err := ctxJSON[ServiceData](app.runner, "service", "ls")
	if err != nil {
		return app.errorf("%s", err.Error())
	}
	port, err := app.resolveSMBPort(serviceData.Services)
	if err != nil {
		return err
	}

	var sharesOutput, sharesError bytes.Buffer
	if err := app.listShares(targetIP, port, &sharesOutput, &sharesError); err != nil {
		if sharesError.Len() > 0 {
			_, _ = io.Copy(app.stderr, &sharesError)
		}
		return err
	}
	shares := parseSMBShares(sharesOutput.String())
	if len(shares) == 0 {
		return app.errorf("no SMB shares found")
	}
	share, err := app.selectShare(shares)
	if err != nil {
		return err
	}
	startedAt := time.Now().UTC()
	logID, err := app.logger.Start("xsmb", smbLogCommand(credential, targetIP, port, share.Name), startedAt.Format(time.RFC3339Nano))
	if err != nil {
		return app.errorf("failed to start SMB log: %s", err.Error())
	}
	if credential == nil {
		_, _ = fmt.Fprintf(app.stdout, "Connecting to //%s/%s:%d...\n", targetIP, share.Name, port)
	} else {
		_, _ = fmt.Fprintf(app.stdout, "Connecting to %s@//%s/%s:%d...\n", credential.Username, targetIP, share.Name, port)
	}
	var commandStdout, commandStderr bytes.Buffer
	streamStdout := io.MultiWriter(app.stdout, &commandStdout)
	streamStderr := io.MultiWriter(app.stderr, &commandStderr)
	err = app.connect(credential, targetIP, port, share.Name, streamStdout, streamStderr)
	status := "success"
	exitCode := 0
	if err != nil {
		status = "failed"
		exitCode = commandExitCode(err)
	}
	finishErr := app.logger.Finish(logID, status, exitCode, commandStdout.String(), commandStderr.String(), time.Now().UTC().Format(time.RFC3339Nano))
	if finishErr != nil {
		return app.errorf("failed to finish SMB log: %s", finishErr.Error())
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
		return nil, app.errorf("SMB credential not found: %s", filter)
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
	_, _ = fmt.Fprintln(app.stdout, "Select an SMB credential:")
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

func (app *App) resolveSMBPort(services []Service) (int, error) {
	ports := smbPorts(services)
	if len(ports) == 0 {
		return 445, nil
	}
	if len(ports) == 1 {
		return ports[0].Port, nil
	}

	_, _ = fmt.Fprintln(app.stdout, "Select an SMB port:")
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

type SMBShare struct {
	Name    string
	Type    string
	Comment string
}

func (app *App) listShares(targetIP string, port int, stdout, stderr io.Writer) error {
	target := fmt.Sprintf("//%s", targetIP)
	args := []string{"-L", target, "-p", strconv.Itoa(port), "-g", "-N"}
	// Share discovery must not consume the user's password or share-selection input.
	return app.runner.Run("smbclient", args, nil, nil, stdout, stderr)
}

func (app *App) selectShare(shares []SMBShare) (SMBShare, error) {
	if len(shares) == 1 {
		return shares[0], nil
	}
	_, _ = fmt.Fprintln(app.stdout, "Select an SMB share:")
	_, _ = fmt.Fprintln(app.stdout)
	for i, share := range shares {
		_, _ = fmt.Fprintf(app.stdout, "  %d) %s\n", i+1, share.Name)
	}
	_, _ = fmt.Fprintln(app.stdout)
	index, err := app.selectIndex(len(shares))
	if err != nil {
		return SMBShare{}, err
	}
	return shares[index], nil
}

func parseSMBShares(output string) []SMBShare {
	shares := make([]SMBShare, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, "|")
		if len(fields) < 2 || !strings.EqualFold(strings.TrimSpace(fields[0]), "disk") {
			continue
		}
		name := strings.TrimSpace(fields[1])
		if name == "" || strings.EqualFold(name, "IPC$") {
			continue
		}
		comment := ""
		if len(fields) > 2 {
			comment = strings.TrimSpace(fields[2])
		}
		shares = append(shares, SMBShare{Name: name, Type: "Disk", Comment: comment})
	}
	return shares
}

func (app *App) connect(credential *Credential, targetIP string, port int, share string, stdout, stderr io.Writer) error {
	target := fmt.Sprintf("//%s/%s", targetIP, share)
	args := []string{target, "-p", strconv.Itoa(port)}
	if credential == nil {
		args = append(args, "-N")
		return app.runner.Run("smbclient", args, nil, app.stdin, stdout, stderr)
	}
	args = append(args, "-U", credential.Username)
	if credential.Password != nil {
		return app.runner.Run("smbclient", args, []string{"PASSWD=" + *credential.Password}, app.stdin, stdout, stderr)
	}
	return app.runner.Run("smbclient", args, nil, app.stdin, stdout, stderr)
}

func smbLogCommand(credential *Credential, targetIP string, port int, share string) string {
	destination := fmt.Sprintf("//%s/%s", targetIP, share)
	args := []string{"smbclient", destination, "-p", strconv.Itoa(port)}
	if credential == nil {
		args = append(args, "-N")
	} else if credential.Username != "" {
		args = append(args, "-U", credential.Username)
	}
	return strings.Join(args, " ")
}

func (app *App) errorf(format string, args ...any) error {
	_, _ = fmt.Fprintf(app.stderr, "xsmb: "+format+"\n", args...)
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
	result, err := ctxapi.Call[T](ctxapi.NewV1(runner), args...)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

func smbCredentials(credentials []Credential) []Credential {
	filtered := make([]Credential, 0, len(credentials))
	for _, credential := range credentials {
		if credential.Scope == "smb" {
			filtered = append(filtered, credential)
		}
	}
	return filtered
}

func smbPorts(services []Service) []Service {
	ports := make([]Service, 0, len(services))
	for _, service := range services {
		if !strings.EqualFold(service.Protocol, "tcp") {
			continue
		}
		if service.ServiceName == nil || !isSMBServiceName(*service.ServiceName) {
			continue
		}
		ports = append(ports, service)
	}
	return ports
}

func isSMBServiceName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "smb", "microsoft-ds", "netbios-ssn":
		return true
	default:
		return false
	}
}
