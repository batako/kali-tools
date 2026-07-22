package xhydra

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	"req/internal/onlinehelp"
	request "req/internal/req"
)

var (
	Version = "1.1.1"
)

const usageText = `usage: xhydra <mode> [options]

Run Hydra against a service and save successful credentials to ctx.

Password and username wordlists are selected from ctx recommendations.
For SSH, FTP, and SMB, wordlists are tried in ctx recommendation order without
repeating searched passwords. The automatic request limit is configured with:
ctx config set password.max-requests <count>.
For SSH, FTP, and SMB, use --password without -u to search usernames with an
automatically selected username wordlist, or provide one with -L/--user-list.

modes:
  http                         attack an HTTP POST form
  ssh                          attack an SSH service
  ftp                          attack an FTP service
  smb                          attack an SMB service

options:
  -u, --username <name>       username to test
  --password <password>       fixed password for username search
  -L, --user-list <path>      use a username list for username search
  --host <host>               override the target host for ssh, ftp, or smb
  -p, --port <port>           override the target port for ssh, ftp, or smb
  --service <number>          select a discovered service for ssh, ftp, or smb
  -t, --tasks <number>        override SSH/FTP/SMB parallel tasks (default: 4)
  --status                    show password wordlist progress (requires -u)
  --clear-cache               clear scoped password search progress (requires -u)
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
  -V, --version               show version
  --online-help               show the versioned online help URL`

type ExitCodeError struct{ Code int }

func (err ExitCodeError) Error() string { return fmt.Sprintf("exit code %d", err.Code) }

type parsedOptions struct {
	Mode            string
	Username        string
	Password        string
	UserList        string
	Host            string
	Port            string
	Service         string
	Tasks           int
	Status          bool
	ClearCache      bool
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
	if options.Mode == "online-help" {
		return onlinehelp.Print(app.stdout, "xhydra", Version)
	}
	if options.Mode == "version" {
		_, err := fmt.Fprintf(app.stdout, "xhydra %s\n", Version)
		return err
	}
	if options.Tasks > 0 && options.Mode != "ssh" && options.Mode != "ftp" && options.Mode != "smb" {
		return errors.New("--tasks is supported for ssh, ftp, and smb only")
	}
	if options.Mode != "http" {
		if options.Mode == "ssh" || options.Mode == "ftp" || options.Mode == "smb" {
			return app.runServiceMode(options, args[1:])
		}
		return errors.New("usage: xhydra <mode> [options]")
	}
	if options.Status {
		return errors.New("--status is supported for ssh, ftp, and smb")
	}
	if options.ClearCache {
		return errors.New("--clear-cache is supported for ssh, ftp, and smb")
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

func (app *app) runServiceMode(options parsedOptions, originalArgs []string) error {
	if options.RequestFile != "" || options.URL != "" || options.Data != "" || options.UserField != "" || options.PasswordField != "" || options.FailJSON != "" || options.SuccessJSON != "" || options.FailBody != "" || options.SuccessBody != "" || options.FailStatus != "" || options.SuccessRedirect {
		return errors.New("HTTP form options cannot be used with ssh, ftp, or smb")
	}
	if options.Tasks > 0 && (options.Status || options.ClearCache) {
		return errors.New("--tasks cannot be combined with --status or --clear-cache")
	}
	host, err := app.serviceHost(options.Host)
	if err != nil {
		return err
	}
	workspace, err := loadWorkspace()
	if err != nil {
		return err
	}
	target, err := ctx.GetPrimaryTarget(workspace)
	if err != nil {
		return fmt.Errorf("failed to load primary target: %w", err)
	}
	port, err := app.resolveServicePort(workspace, options.Mode, options.Port, options.Service)
	if err != nil {
		return err
	}
	usernameSearch := options.Password != "" && options.Username == ""
	if options.Password != "" && options.Username != "" {
		return errors.New("--password cannot be combined with --username")
	}
	if options.UserList != "" && !usernameSearch {
		return errors.New("--user-list requires --password without --username")
	}
	if options.Username == "" && !usernameSearch {
		return errors.New("username is required: use -u <name>")
	}
	if usernameSearch {
		if options.Status {
			return app.showUsernameStatus(workspace, target.ID, options, host, port)
		}
		if options.ClearCache {
			return app.clearUsernameCache(workspace, target.ID, options, host, port)
		}
		return app.runUsernameMode(workspace, target.ID, options, host, port, originalArgs)
	}
	if options.ClearCache {
		if options.Status || options.PasswordList != "" {
			return errors.New("--clear-cache cannot be combined with --status or --password-list")
		}
		statePath, err := passwordStatePath(workspace, target.ID, options.Mode, host, port, options.Username)
		if err != nil {
			return err
		}
		cleared, err := clearPasswordCache(statePath)
		if err != nil {
			return fmt.Errorf("failed to clear password wordlist state: %w", err)
		}
		if cleared {
			_, _ = fmt.Fprintf(app.stdout, "Cleared password search cache for %s@%s:%d (%s)\n", options.Username, host, port, options.Mode)
		} else {
			_, _ = fmt.Fprintf(app.stdout, "No password search cache found for %s@%s:%d (%s)\n", options.Username, host, port, options.Mode)
		}
		return nil
	}
	if options.Status {
		return app.showPasswordStatus(workspace, target.ID, options, host, port)
	}
	if _, err := app.runner.LookPath("hydra"); err != nil {
		return errors.New("hydra is required; install the hydra package")
	}
	batches, cleanup, err := app.preparePasswordBatches(workspace, target.ID, options, host, port)
	if err != nil {
		return err
	}
	defer cleanup()
	for _, batch := range batches {
		hydraArgs := buildServiceHydraArgs(options.Mode, host, port, batch.TempPath, options.Username, options.Tasks)
		expandedArgs := buildServiceHydraArgs(options.Mode, host, port, batch.OriginalPath, options.Username, options.Tasks)
		logID, err := ctx.StartCommandLog(workspace, ctx.CommandLog{
			Command:         commandString(append([]string{"xhydra"}, originalArgs...)),
			ExpandedCommand: commandString(append([]string{"hydra"}, expandedArgs...)),
			StartedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return fmt.Errorf("failed to start command log: %w", err)
		}
		var commandStdout, commandStderr bytes.Buffer
		runErr := app.runner.Run("hydra", hydraArgs, io.MultiWriter(app.stdout, &commandStdout), io.MultiWriter(app.stderr, &commandStderr))
		exitCode := commandExitCode(runErr)
		status := "success"
		if exitCode != 0 {
			status = "failed"
		}
		finishErr := ctx.FinishCommandLog(workspace, logID, ctx.CommandLog{
			Status: status, ExitCode: exitCode, Stdout: commandStdout.String(), Stderr: commandStderr.String(),
			EndedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		if finishErr != nil {
			return fmt.Errorf("failed to finish command log: %w", finishErr)
		}
		if runErr != nil {
			return runErr
		}
		password, ok := hydraSuccessPassword(commandStdout.String())
		wordsToSave := batch.Words
		if ok && len(batch.Words) > 0 {
			for i, word := range batch.Words {
				if word == password {
					wordsToSave = batch.Words[:i+1]
					break
				}
			}
		}
		if batch.StatePath != "" {
			if err := appendSearchedPasswords(batch.StatePath, wordsToSave); err != nil {
				return fmt.Errorf("failed to save password wordlist state: %w", err)
			}
		}
		if ok {
			scope := options.Mode
			if _, err := ctx.SetCredential(workspace, scope, options.Username, password); err != nil {
				return fmt.Errorf("failed to save %s credential: %w", scope, err)
			}
			_, err = fmt.Fprintf(app.stdout, "Saved %s credential for %s (log: ctx log %d)\n", scope, options.Username, logID)
			return err
		}
	}
	return errors.New("hydra completed without a successful credential")
}

func (app *app) showPasswordStatus(workspace *ctx.Workspace, targetID int64, options parsedOptions, host string, port int) error {
	candidates := []ctx.WordlistSelection{}
	if options.PasswordList != "" {
		candidates = append(candidates, ctx.WordlistSelection{Provider: "manual", Type: ctx.WordlistTypePassword, Path: options.PasswordList})
	} else {
		var err error
		candidates, err = recommendWordlists(ctx.WordlistKindPassword)
		if err != nil {
			return fmt.Errorf("failed to select password wordlist: %w", err)
		}
	}
	statePath, err := passwordStatePath(workspace, targetID, options.Mode, host, port, options.Username)
	if err != nil {
		return err
	}
	seen, err := loadSearchedPasswords(statePath)
	if err != nil {
		return fmt.Errorf("failed to load password wordlist state: %w", err)
	}
	var total, covered int
	type statusEntry struct {
		selection ctx.WordlistSelection
		total     int
		covered   int
	}
	entries := make([]statusEntry, 0, len(candidates))
	for _, candidate := range candidates {
		words, err := loadUniquePasswordWords(candidate.Path)
		if err != nil {
			return fmt.Errorf("failed to inspect wordlist %s: %w", candidate.Path, err)
		}
		wordCovered := 0
		for word := range words {
			if _, ok := seen[word]; ok {
				wordCovered++
			}
		}
		total += len(words)
		covered += wordCovered
		entries = append(entries, statusEntry{selection: candidate, total: len(words), covered: wordCovered})
	}
	_, _ = fmt.Fprintf(app.stdout, "Password wordlist status for %s:%d (%s, user: %s)\n", host, port, options.Mode, options.Username)
	_, _ = fmt.Fprintf(app.stdout, "Entries: %d total, %d covered, %d remaining\n\n", total, covered, total-covered)
	for _, entry := range entries {
		state := "pending"
		if entry.covered > 0 && entry.covered < entry.total {
			state = "partial"
		} else if entry.total > 0 && entry.covered >= entry.total {
			state = "completed"
		}
		_, _ = fmt.Fprintf(app.stdout, "[%s] %-16s %7d/%d words  %s\n", state, entry.selection.Profile, entry.covered, entry.total, entry.selection.Path)
	}
	return nil
}

func (app *app) showUsernameStatus(workspace *ctx.Workspace, targetID int64, options parsedOptions, host string, port int) error {
	candidates, err := usernameWordlistCandidates(options.UserList)
	if err != nil {
		return err
	}
	statePath, err := usernameStatePath(workspace, targetID, options.Mode, host, port, options.Password)
	if err != nil {
		return err
	}
	seen, err := loadSearchedPasswords(statePath)
	if err != nil {
		return fmt.Errorf("failed to load username wordlist state: %w", err)
	}
	var total, covered int
	_, _ = fmt.Fprintf(app.stdout, "Username wordlist status for %s:%d (%s, password: <redacted>)\n", host, port, options.Mode)
	for _, candidate := range candidates {
		words, err := loadUniquePasswordWords(candidate.Path)
		if err != nil {
			return fmt.Errorf("failed to inspect wordlist %s: %w", candidate.Path, err)
		}
		wordCovered := 0
		for word := range words {
			if _, ok := seen[word]; ok {
				wordCovered++
			}
		}
		total += len(words)
		covered += wordCovered
		state := "pending"
		if wordCovered > 0 && wordCovered < len(words) {
			state = "partial"
		} else if len(words) > 0 && wordCovered == len(words) {
			state = "completed"
		}
		_, _ = fmt.Fprintf(app.stdout, "[%s] %-17s %7d/%d words  %s\n", state, candidate.Profile, wordCovered, len(words), candidate.Path)
	}
	_, _ = fmt.Fprintf(app.stdout, "\nEntries: %d total, %d covered, %d remaining\n", total, covered, total-covered)
	return nil
}

func usernameWordlistCandidates(explicit string) ([]ctx.WordlistSelection, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return nil, fmt.Errorf("user list not found: %s", explicit)
		}
		return []ctx.WordlistSelection{{Provider: "manual", Profile: ctx.WordlistProfileUsernameQuick, Path: explicit}}, nil
	}
	candidates, err := recommendWordlists(ctx.WordlistKindUsername)
	if err != nil {
		return nil, fmt.Errorf("failed to select username wordlist: %w; use --user-list to override", err)
	}
	return candidates, nil
}

func (app *app) clearUsernameCache(workspace *ctx.Workspace, targetID int64, options parsedOptions, host string, port int) error {
	if options.Status || options.UserList != "" {
		return errors.New("--clear-cache cannot be combined with --status or --user-list")
	}
	statePath, err := usernameStatePath(workspace, targetID, options.Mode, host, port, options.Password)
	if err != nil {
		return err
	}
	cleared, err := clearPasswordCache(statePath)
	if err != nil {
		return fmt.Errorf("failed to clear username wordlist state: %w", err)
	}
	if cleared {
		_, _ = fmt.Fprintf(app.stdout, "Cleared username search cache for password <redacted> at %s:%d (%s)\n", host, port, options.Mode)
	} else {
		_, _ = fmt.Fprintf(app.stdout, "No username search cache found for password <redacted> at %s:%d (%s)\n", host, port, options.Mode)
	}
	return nil
}

func (app *app) runUsernameMode(workspace *ctx.Workspace, targetID int64, options parsedOptions, host string, port int, originalArgs []string) error {
	if _, err := app.runner.LookPath("hydra"); err != nil {
		return errors.New("hydra is required; install the hydra package")
	}
	batches, cleanup, err := app.prepareUsernameBatches(workspace, targetID, options, host, port)
	if err != nil {
		return err
	}
	defer cleanup()
	for _, batch := range batches {
		hydraArgs := buildServiceCredentialArgs(options.Mode, host, port, "", options.Password, batch.TempPath, "", options.Tasks)
		expandedArgs := buildServiceCredentialArgs(options.Mode, host, port, "", "<redacted>", batch.OriginalPath, "", options.Tasks)
		logID, err := ctx.StartCommandLog(workspace, ctx.CommandLog{
			Command:         commandString(redactCommandArgs(append([]string{"xhydra"}, originalArgs...))),
			ExpandedCommand: commandString(redactCommandArgs(append([]string{"hydra"}, expandedArgs...))),
			StartedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return fmt.Errorf("failed to start command log: %w", err)
		}
		var commandStdout, commandStderr bytes.Buffer
		runErr := app.runner.Run("hydra", hydraArgs, io.MultiWriter(app.stdout, &commandStdout), io.MultiWriter(app.stderr, &commandStderr))
		exitCode := commandExitCode(runErr)
		status := "success"
		if exitCode != 0 {
			status = "failed"
		}
		finishErr := ctx.FinishCommandLog(workspace, logID, ctx.CommandLog{Status: status, ExitCode: exitCode, Stdout: commandStdout.String(), Stderr: commandStderr.String(), EndedAt: time.Now().UTC().Format(time.RFC3339Nano)})
		if finishErr != nil {
			return fmt.Errorf("failed to finish command log: %w", finishErr)
		}
		if runErr != nil {
			return runErr
		}
		username, ok := hydraSuccessUsername(commandStdout.String())
		wordsToSave := batch.Words
		if ok {
			for i, word := range batch.Words {
				if word == username {
					wordsToSave = batch.Words[:i+1]
					break
				}
			}
		}
		if batch.StatePath != "" {
			if err := appendSearchedPasswords(batch.StatePath, wordsToSave); err != nil {
				return fmt.Errorf("failed to save username wordlist state: %w", err)
			}
		}
		if ok {
			if _, err := ctx.SetCredential(workspace, options.Mode, username, options.Password); err != nil {
				return fmt.Errorf("failed to save %s credential: %w", options.Mode, err)
			}
			_, err = fmt.Fprintf(app.stdout, "Saved %s credential for %s (log: ctx log %d)\n", options.Mode, username, logID)
			return err
		}
	}
	return errors.New("hydra completed without a successful credential")
}

func (app *app) prepareUsernameBatches(workspace *ctx.Workspace, targetID int64, options parsedOptions, host string, port int) ([]passwordBatch, func(), error) {
	config, err := ctx.LoadConfig()
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to load wordlist config: %w", err)
	}
	candidates, err := usernameWordlistCandidates(options.UserList)
	if err != nil {
		return nil, func() {}, err
	}
	statePath, err := usernameStatePath(workspace, targetID, options.Mode, host, port, options.Password)
	if err != nil {
		return nil, func() {}, err
	}
	seen, err := loadSearchedPasswords(statePath)
	if err != nil {
		return nil, func() {}, err
	}
	if options.UserList != "" {
		seen = map[string]struct{}{}
	}
	var batches []passwordBatch
	var temporaryPaths []string
	planned := 0
	for _, candidate := range candidates {
		words, err := filteredPasswordWordlist(candidate.Path, seen)
		if err != nil {
			return nil, func() {}, fmt.Errorf("failed to prepare wordlist %s: %w", candidate.Path, err)
		}
		remaining := config.PasswordMaxRequests - planned
		if remaining <= 0 {
			break
		}
		if len(words) > remaining {
			words = words[:remaining]
		}
		if len(words) == 0 {
			continue
		}
		temporaryPath, err := writeTemporaryPasswordList(words)
		if err != nil {
			return nil, func() {}, err
		}
		temporaryPaths = append(temporaryPaths, temporaryPath)
		batches = append(batches, passwordBatch{OriginalPath: candidate.Path, TempPath: temporaryPath, Words: words, StatePath: statePath})
		planned += len(words)
		for _, word := range words {
			seen[word] = struct{}{}
		}
	}
	if len(batches) == 0 {
		return nil, func() {}, errors.New("all configured username wordlists have completed; use --clear-cache to restart")
	}
	return batches, func() {
		for _, path := range temporaryPaths {
			_ = os.Remove(path)
		}
	}, nil
}

type passwordBatch struct {
	OriginalPath string
	TempPath     string
	Words        []string
	StatePath    string
}

func (app *app) preparePasswordBatches(workspace *ctx.Workspace, targetID int64, options parsedOptions, host string, port int) ([]passwordBatch, func(), error) {
	if options.PasswordList != "" {
		passwordList, cleanup, err := resolvePasswordList(options.PasswordList)
		if err != nil {
			return nil, cleanup, err
		}
		return []passwordBatch{{OriginalPath: passwordList, TempPath: passwordList}}, cleanup, nil
	}
	config, err := ctx.LoadConfig()
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to load wordlist config: %w", err)
	}
	candidates, err := recommendWordlists(ctx.WordlistKindPassword)
	if err != nil {
		return nil, func() {}, fmt.Errorf("failed to select password wordlist: %w", err)
	}
	statePath, err := passwordStatePath(workspace, targetID, options.Mode, host, port, options.Username)
	if err != nil {
		return nil, func() {}, err
	}
	seen, err := loadSearchedPasswords(statePath)
	if err != nil {
		return nil, func() {}, err
	}
	batches := make([]passwordBatch, 0)
	temporaryPaths := make([]string, 0)
	planned := 0
	for _, candidate := range candidates {
		candidateWords, err := filteredPasswordWordlist(candidate.Path, seen)
		if err != nil {
			return nil, func() {}, fmt.Errorf("failed to prepare wordlist %s: %w", candidate.Path, err)
		}
		remaining := config.PasswordMaxRequests - planned
		if remaining <= 0 {
			break
		}
		if len(candidateWords) > remaining {
			candidateWords = candidateWords[:remaining]
		}
		if len(candidateWords) == 0 {
			continue
		}
		temporaryPath, err := writeTemporaryPasswordList(candidateWords)
		if err != nil {
			return nil, func() {}, err
		}
		temporaryPaths = append(temporaryPaths, temporaryPath)
		batches = append(batches, passwordBatch{OriginalPath: candidate.Path, TempPath: temporaryPath, Words: candidateWords, StatePath: statePath})
		planned += len(candidateWords)
		for _, word := range candidateWords {
			seen[word] = struct{}{}
		}
		if planned >= config.PasswordMaxRequests {
			break
		}
	}
	if len(batches) == 0 {
		return nil, func() {}, errors.New("all configured password wordlists have completed; use --clear-cache to restart")
	}
	return batches, func() {
		for _, path := range temporaryPaths {
			_ = os.Remove(path)
		}
	}, nil
}

func passwordStatePath(workspace *ctx.Workspace, targetID int64, mode, host string, port int, username string) (string, error) {
	digest := sha256.Sum256([]byte(mode + "\x00" + host + "\x00" + strconv.Itoa(port) + "\x00" + username))
	directory := filepath.Join(workspace.DataPath, "password-wordlists", strconv.FormatInt(targetID, 10), hex.EncodeToString(digest[:]))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return filepath.Join(directory, "searched.words"), nil
}

func usernameStatePath(workspace *ctx.Workspace, targetID int64, mode, host string, port int, password string) (string, error) {
	digest := sha256.Sum256([]byte(mode + "\x00" + host + "\x00" + strconv.Itoa(port) + "\x00" + password))
	directory := filepath.Join(workspace.DataPath, "username-wordlists", strconv.FormatInt(targetID, 10), hex.EncodeToString(digest[:]))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return filepath.Join(directory, "searched.words"), nil
}

func filteredPasswordWordlist(path string, seen map[string]struct{}) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	words := make([]string, 0)
	local := make(map[string]struct{})
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word == "" || hasSearchedPassword(seen, word) {
			continue
		}
		if _, ok := local[word]; ok {
			continue
		}
		local[word] = struct{}{}
		words = append(words, word)
	}
	return words, scanner.Err()
}

func loadUniquePasswordWords(path string) (map[string]struct{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	words := make(map[string]struct{})
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word != "" {
			words[word] = struct{}{}
		}
	}
	return words, scanner.Err()
}

func loadSearchedPasswords(path string) (map[string]struct{}, error) {
	seen := make(map[string]struct{})
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return seen, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word != "" {
			seen[word] = struct{}{}
		}
	}
	return seen, scanner.Err()
}

func appendSearchedPasswords(path string, words []string) error {
	if len(words) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, word := range words {
		if _, err := fmt.Fprintln(file, word); err != nil {
			return err
		}
	}
	return nil
}

func clearPasswordCache(path string) (bool, error) {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func hasSearchedPassword(seen map[string]struct{}, word string) bool {
	_, ok := seen[word]
	return ok
}

func writeTemporaryPasswordList(words []string) (string, error) {
	file, err := os.CreateTemp("", "xhydra-passwords-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	for _, word := range words {
		if _, err := fmt.Fprintln(file, word); err != nil {
			file.Close()
			_ = os.Remove(path)
			return "", err
		}
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func (app *app) serviceHost(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit), nil
	}
	data, err := ctx.LoadPromptData(".")
	if err != nil {
		return "", err
	}
	if !data.Active || strings.TrimSpace(data.TargetIP) == "" {
		return "", errors.New("no active target; use --host <host>")
	}
	return strings.TrimSpace(data.TargetIP), nil
}

func (app *app) resolveServicePort(workspace *ctx.Workspace, mode, explicit, selection string) (int, error) {
	if explicit != "" {
		if selection != "" {
			return 0, errors.New("--port cannot be combined with --service")
		}
		port, err := strconv.Atoi(explicit)
		if err != nil || port < 1 || port > 65535 {
			return 0, errors.New("invalid --port")
		}
		return port, nil
	}
	target, err := ctx.GetPrimaryTarget(workspace)
	if err != nil {
		return 0, fmt.Errorf("failed to load primary target: %w", err)
	}
	services, err := ctx.ListServices(workspace, target)
	if err != nil {
		return 0, fmt.Errorf("failed to load services: %w", err)
	}
	candidates := matchingServices(services, mode)
	if len(candidates) == 0 {
		return defaultServicePort(mode), nil
	}
	if selection != "" {
		index, err := strconv.Atoi(selection)
		if err != nil || index < 1 || index > len(candidates) {
			return 0, errors.New("invalid service selection")
		}
		return candidates[index-1].Port, nil
	}
	if len(candidates) == 1 {
		return candidates[0].Port, nil
	}
	_, _ = fmt.Fprintf(app.stdout, "Select a %s service:\n\n", mode)
	for i, service := range candidates {
		name := service.ServiceName
		if name == "" {
			name = mode
		}
		_, _ = fmt.Fprintf(app.stdout, "  %d) %s:%d\n", i+1, name, service.Port)
	}
	_, _ = fmt.Fprintf(app.stdout, "\nSelect [1-%d]: ", len(candidates))
	line, err := bufio.NewReader(app.stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return 0, errors.New("cancelled")
	}
	index, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || index < 1 || index > len(candidates) {
		return 0, errors.New("invalid service selection")
	}
	return candidates[index-1].Port, nil
}

func matchingServices(services []ctx.Service, mode string) []ctx.Service {
	matched := make([]ctx.Service, 0)
	for _, service := range services {
		name := strings.ToLower(service.ServiceName)
		protocol := strings.ToLower(service.Protocol)
		if protocol != "" && protocol != "tcp" {
			continue
		}
		matches := strings.Contains(name, mode)
		if mode == "smb" {
			matches = matches || strings.Contains(name, "microsoft-ds")
		}
		if (mode == "ssh" && service.Port == 22) || (mode == "ftp" && service.Port == 21) || (mode == "smb" && service.Port == 445) {
			matches = true
		}
		if matches {
			matched = append(matched, service)
		}
	}
	return matched
}

func defaultServicePort(mode string) int {
	switch mode {
	case "ssh":
		return 22
	case "ftp":
		return 21
	case "smb":
		return 445
	default:
		return 0
	}
}

func buildServiceHydraArgs(mode, host string, port int, passwordList, username string, tasks int) []string {
	return buildServiceCredentialArgs(mode, host, port, passwordList, "", "", username, tasks)
}

func buildServiceCredentialArgs(mode, host string, port int, passwordList, password, usernameList, username string, tasks int) []string {
	module := mode
	if mode == "smb" {
		module = "smb2"
	}
	args := []string{}
	if usernameList != "" {
		args = append(args, "-L", usernameList)
	} else {
		args = append(args, "-l", username)
	}
	if password != "" {
		args = append(args, "-p", password)
	} else {
		args = append(args, "-P", passwordList)
	}
	if defaultTasks := defaultServiceTasks(mode); defaultTasks > 0 {
		if tasks < 1 {
			tasks = defaultTasks
		}
		args = append(args, "-t", strconv.Itoa(tasks))
	}
	args = append(args, "-f", "-s", strconv.Itoa(port), host, module)
	return args
}

func defaultServiceTasks(mode string) int {
	switch mode {
	case "ssh":
		return 4
	case "ftp":
		return 4
	case "smb":
		return 4
	default:
		return 0
	}
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
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
	candidates, err := recommendWordlists(ctx.WordlistKindPassword)
	if err != nil {
		return "", func() {}, fmt.Errorf("%w; use --password-list to override", err)
	}
	return candidates[0].Path, func() {}, nil
}

var recommendWordlists = ctx.RecommendWordlists

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

func hydraSuccessUsername(output string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		loginIndex := strings.Index(line, "login:")
		passwordIndex := strings.Index(line, "password:")
		if loginIndex < 0 || passwordIndex < 0 || loginIndex >= passwordIndex {
			continue
		}
		username := strings.TrimSpace(line[loginIndex+len("login:") : passwordIndex])
		if username != "" {
			return username, true
		}
	}
	return "", false
}

func redactCommandArgs(args []string) []string {
	redacted := append([]string(nil), args...)
	for i := 0; i < len(redacted); i++ {
		if redacted[i] == "--password" && i+1 < len(redacted) {
			redacted[i+1] = "<redacted>"
		}
	}
	return redacted
}

func parseOptions(args []string) (parsedOptions, error) {
	var options parsedOptions
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		return parsedOptions{Mode: "help"}, nil
	}
	if len(args) == 1 && args[0] == "--online-help" {
		return parsedOptions{Mode: "online-help"}, nil
	}
	if len(args) == 1 && (args[0] == "-V" || args[0] == "--version") {
		return parsedOptions{Mode: "version"}, nil
	}
	if len(args) == 0 || (args[0] != "http" && args[0] != "ssh" && args[0] != "ftp" && args[0] != "smb") {
		return parsedOptions{}, errors.New("usage: xhydra <mode> [options]")
	}
	options.Mode = args[0]
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
		case "--password":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.Password = v
		case "-L", "--user-list":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.UserList = v
		case "--host":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.Host = v
		case "-p", "--port":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.Port = v
		case "--service":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			options.Service = v
		case "-t", "--tasks":
			v, err := value(args[i])
			if err != nil {
				return parsedOptions{}, err
			}
			tasks, convErr := strconv.Atoi(v)
			if convErr != nil || tasks < 1 {
				return parsedOptions{}, errors.New("tasks must be a positive integer")
			}
			options.Tasks = tasks
		case "--force":
			return parsedOptions{}, errors.New("--force was removed; rerun the same command to continue or use --clear-cache to restart")
		case "--status":
			options.Status = true
		case "--clear-cache":
			options.ClearCache = true
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
