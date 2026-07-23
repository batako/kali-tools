// Package xdec provides the xdec facade for deterministic decoding and
// password recovery. John/hashcat are implementation details; callers only
// need to provide a value, file, or stdin.
package xdec

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	ctxpkg "req/internal/ctx"
	"req/internal/onlinehelp"
)

const rootUsageText = `usage: xdec [options] [FILE_OR_STRING]
       xdec <subcommand> [options]

Subcommands:
  decode              decode values and detect recoverable inputs
  recover             recover passwords and key passphrases
  rot                 apply Caesar/ROT shifts
  help                show help for the root or a subcommand
  version             show version

When an input is provided without a subcommand, xdec detects whether it should
decode the value or recover a secret and chooses the operation automatically.

Global options:
  -h, --help          show this help
  -V, --version       show version
  --online-help       show the versioned online help URL`

const helpUsageText = `usage: xdec help [subcommand]

Show help for the root command or one of its subcommands.

arguments:
  subcommand          optional help target:
  decode              decode values and detect recoverable inputs
  help                show this help
  recover             recover passwords and key passphrases
  rot                 apply Caesar/ROT shifts
  version             show version help

options:
  -h, --help          show this help`

const versionUsageText = `usage: xdec version

Show the xdec version.

options:
  -h, --help          show this help`

const usageText = `usage: xdec decode [options] [FILE_OR_STRING]
       command | xdec decode [options]

Decode values using deterministic, local transformations. Hashes and
password-protected keys are reported as recoverable and are not cracked by
this subcommand.

input:
  FILE_OR_STRING        existing regular files are read as files;
                        other values are treated as literal strings
  stdin                 read when no positional or explicit input is provided

options:
  -f, --file FILE       read FILE as input (an existing positional FILE is also detected)
      --string VALUE    treat VALUE as a string (overrides file auto-detection)
      --json              output structured results
  -h, --help             show this help`

const recoverUsageText = `usage: xdec recover [options] [FILE_OR_STRING]
       command | xdec recover [options]

Recover passwords and key passphrases from supported inputs. Recovery may take
a long time and always requires confirmation unless --yes is provided.

input:
  FILE_OR_STRING        existing regular files are read as files;
                        other values are treated as literal strings
  stdin                 read when no positional or explicit input is provided

options:
  -f, --file FILE       read FILE as input (an existing positional FILE is also detected)
      --string VALUE    treat VALUE as a string (overrides file auto-detection)
  -w, --wordlist SPEC   ctx wordlist id/path; default is ctx password recommendation
      --scope SCOPE     credential scope when saving a recovered password
      --username USER   username when the input does not contain one
      --save-credential save a verified user password to ctx
      --no-save-credential never save credentials
      --yes              explicitly approve expensive recovery (also for pipes)
      --refresh          discard this input's saved state and analyze again
      --dry-run          show the plan without running recovery
      --json              output structured results
  -h, --help             show this help`

const rotUsageText = `usage: xdec rot [options] [FILE_OR_STRING]
       command | xdec rot [options]

Apply Caesar/ROT shifts. With no shift, all Caesar shifts from 0 through 25
are printed. Shifts above 25 use printable ASCII characters (! through ~).

input:
  FILE_OR_STRING        existing regular files are read as files;
                        other values are treated as literal strings
  stdin                 read when no positional or explicit input is provided

options:
  -n, --shift N         apply one shift or a range such as 0-25 or 0-93
  -f, --file FILE       read FILE as input
      --string VALUE    treat VALUE as a string
  -h, --help            show this help`

var Version = "1.0.0"

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	if strings.TrimSpace(v) == "" {
		return errors.New("wordlist must not be empty")
	}
	*s = append(*s, v)
	return nil
}

type options struct {
	operation        string
	file             string
	stringInput      string
	wordlists        stringList
	scope            string
	username         string
	saveCredential   bool
	noSaveCredential bool
	yes              bool
	refresh          bool
	dryRun           bool
	json             bool
}

type document struct {
	Source   string
	Name     string
	Raw      []byte
	Analysis []byte
	Kind     string
	Notice   string
}

type candidate struct {
	Value    string
	Original string
	Line     int
	Username string
	Scope    string
}

type classification struct {
	Kind       string
	Confidence float64
	Reason     string
}

type result struct {
	Candidate candidate
	Kind      string
	Method    string
	Value     string
	Status    string
	Saved     bool
}

type wordlist struct {
	ID    string
	Path  string
	Lines int64
	Hash  string
}

type backend struct {
	Name string
	Path string
}

var (
	hexHash32 = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
	hexHash40 = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	hexHash64 = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
	cryptHash = regexp.MustCompile(`^\$(2[aby]|5|6|argon2)[^ ]*\$`)
	sshHash   = regexp.MustCompile(`^\$(sshng|ssh2)\$`)
)

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) <= 1 {
		_, err := fmt.Fprintln(stdout, rootUsageText)
		return err
	}
	commandArgs := args[1:]
	switch commandArgs[0] {
	case "-h", "--help":
		if len(commandArgs) != 1 {
			return errors.New("usage: xdec --help")
		}
		_, err := fmt.Fprintln(stdout, rootUsageText)
		return err
	case "help":
		switch {
		case len(commandArgs) == 1:
			_, err := fmt.Fprintln(stdout, rootUsageText)
			return err
		case len(commandArgs) == 2 && (commandArgs[1] == "-h" || commandArgs[1] == "--help" || commandArgs[1] == "help"):
			_, err := fmt.Fprintln(stdout, helpUsageText)
			return err
		case len(commandArgs) == 2 && commandArgs[1] == "decode":
			_, err := fmt.Fprintln(stdout, usageText)
			return err
		case len(commandArgs) == 2 && commandArgs[1] == "recover":
			_, err := fmt.Fprintln(stdout, recoverUsageText)
			return err
		case len(commandArgs) == 2 && commandArgs[1] == "rot":
			_, err := fmt.Fprintln(stdout, rotUsageText)
			return err
		case len(commandArgs) == 2 && commandArgs[1] == "version":
			_, err := fmt.Fprintln(stdout, versionUsageText)
			return err
		default:
			return errors.New("usage: xdec help [decode|recover|rot|help|version]")
		}
	case "-V", "--version":
		if len(commandArgs) != 1 {
			return errors.New("usage: xdec --version")
		}
		_, err := fmt.Fprintf(stdout, "xdec %s\n", Version)
		return err
	case "version":
		if len(commandArgs) == 2 && (commandArgs[1] == "-h" || commandArgs[1] == "--help") {
			_, err := fmt.Fprintln(stdout, versionUsageText)
			return err
		}
		if len(commandArgs) != 1 {
			return errors.New("usage: xdec version")
		}
		_, err := fmt.Fprintf(stdout, "xdec %s\n", Version)
		return err
	case "--online-help":
		if len(commandArgs) != 1 {
			return errors.New("usage: xdec --online-help")
		}
		return onlinehelp.Print(stdout, "xdec", Version)
	case "decode":
		return runDecode(append([]string{"xdec"}, commandArgs[1:]...), "decode", stdin, stdout, stderr)
	case "recover":
		return runDecode(append([]string{"xdec"}, commandArgs[1:]...), "recover", stdin, stdout, stderr)
	case "rot":
		return runRot(commandArgs[1:], stdin, stdout, stderr)
	default:
		return runDecode(append([]string{"xdec"}, commandArgs...), "auto", stdin, stdout, stderr)
	}
}

type rotResult struct {
	Shift int    `json:"shift"`
	Value string `json:"value"`
}

func runRot(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	parsedArgs := reorderRotFlags(args)
	if len(parsedArgs) == 0 {
		_, err := fmt.Fprintln(stdout, rotUsageText)
		return err
	}
	for _, arg := range parsedArgs {
		switch arg {
		case "-h", "--help":
			_, err := fmt.Fprintln(stdout, rotUsageText)
			return err
		case "-V", "--version", "--online-help":
			return errors.New("xdec rot: use xdec version or xdec --online-help")
		}
	}

	var shiftSpec, file, stringInput string
	fs := flag.NewFlagSet("xdec rot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprintln(stderr, rotUsageText) }
	fs.StringVar(&shiftSpec, "n", "", "shift or shift range")
	fs.StringVar(&shiftSpec, "shift", "", "shift or shift range")
	fs.StringVar(&file, "f", "", "input file")
	fs.StringVar(&file, "file", "", "input file")
	fs.StringVar(&stringInput, "string", "", "input string")
	if err := fs.Parse(parsedArgs); err != nil {
		return err
	}
	if file != "" && stringInput != "" {
		return errors.New("xdec rot: --file and --string cannot be combined")
	}
	if file != "" && len(fs.Args()) > 0 {
		return errors.New("xdec rot: --file cannot be combined with a positional input")
	}
	if stringInput != "" && len(fs.Args()) > 0 {
		return errors.New("xdec rot: --string cannot be combined with a positional input")
	}
	if len(fs.Args()) > 1 {
		return errors.New("xdec rot: only one positional input is allowed")
	}
	data, err := readPlainInput(file, stringInput, fs.Args(), stdin)
	if err != nil {
		return err
	}
	// Match the existing shell helper: command substitution removes trailing
	// newlines while preserving line breaks inside the input.
	data = bytes.TrimRight(data, "\r\n")
	if len(data) == 0 {
		return errors.New("xdec rot: empty input")
	}
	start, end, printableMode, err := parseRotRange(shiftSpec)
	if err != nil {
		return err
	}
	results := make([]rotResult, 0, end-start+1)
	for shift := start; shift <= end; shift++ {
		results = append(results, rotResult{Shift: shift, Value: applyRot(data, shift, printableMode)})
	}
	for _, result := range results {
		fmt.Fprintf(stdout, "rot %d: %s\n", result.Shift, result.Value)
	}
	return nil
}

func reorderRotFlags(args []string) []string {
	var flags, values []string
	valueFlags := map[string]bool{"-n": true, "--shift": true, "-f": true, "--file": true, "--string": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name := arg
		if separator := strings.IndexByte(arg, '='); separator >= 0 {
			name = arg[:separator]
		}
		if valueFlags[name] {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			continue
		}
		values = append(values, arg)
	}
	return append(flags, values...)
}

func readPlainInput(file, stringInput string, args []string, stdin io.Reader) ([]byte, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("xdec rot: read %s: %w", file, err)
		}
		return b, nil
	}
	if stringInput != "" {
		return []byte(stringInput), nil
	}
	if len(args) == 1 {
		if info, err := os.Stat(args[0]); err == nil && info.Mode().IsRegular() {
			b, readErr := os.ReadFile(args[0])
			if readErr != nil {
				return nil, fmt.Errorf("xdec rot: read %s: %w", args[0], readErr)
			}
			return b, nil
		}
		return []byte(args[0]), nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("xdec rot: read stdin: %w", err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, errors.New("xdec rot: no input")
	}
	return b, nil
}

func parseRotRange(spec string) (start, end int, printableMode bool, err error) {
	if spec == "" {
		return 0, 25, false, nil
	}
	parts := strings.Split(spec, "-")
	if len(parts) > 2 || parts[0] == "" {
		return 0, 0, false, errors.New("xdec rot: --shift needs N or START-END")
	}
	start, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false, errors.New("xdec rot: --shift needs numeric values")
	}
	end = start
	if len(parts) == 2 {
		end, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false, errors.New("xdec rot: --shift needs numeric values")
		}
	}
	printableMode = start > 25 || end > 25
	max := 25
	if printableMode {
		max = 93
	}
	if start < 0 || end < start || end > max {
		return 0, 0, false, fmt.Errorf("xdec rot: shift must be between 0 and %d", max)
	}
	return start, end, printableMode, nil
}

func applyRot(input []byte, shift int, printableMode bool) string {
	out := append([]byte(nil), input...)
	if printableMode {
		for i, c := range out {
			if c >= '!' && c <= '~' {
				out[i] = byte((int(c-'!')+shift)%94) + '!'
			}
		}
		return string(out)
	}
	for i, c := range out {
		switch {
		case c >= 'a' && c <= 'z':
			out[i] = byte((int(c-'a')+shift)%26) + 'a'
		case c >= 'A' && c <= 'Z':
			out[i] = byte((int(c-'A')+shift)%26) + 'A'
		}
	}
	return string(out)
}

func runDecode(args []string, operation string, stdin io.Reader, stdout, stderr io.Writer) error {
	usage := usageText
	if operation == "recover" {
		usage = recoverUsageText
	}
	parsedArgs := reorderFlags(args[1:])
	if len(parsedArgs) == 0 {
		_, err := fmt.Fprintln(stdout, usage)
		return err
	}
	for _, arg := range parsedArgs {
		switch arg {
		case "-h", "--help":
			_, err := fmt.Fprintln(stdout, usage)
			return err
		case "-V", "--version":
			return fmt.Errorf("xdec %s: --version is a root option; use xdec --version", operation)
		case "--online-help":
			return fmt.Errorf("xdec %s: --online-help is a root option; use xdec --online-help", operation)
		}
	}

	var opts options
	opts.operation = operation
	fs := flag.NewFlagSet("xdec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprintln(stderr, usage) }
	fs.StringVar(&opts.file, "f", "", "input file")
	fs.StringVar(&opts.file, "file", "", "input file")
	fs.StringVar(&opts.stringInput, "string", "", "input string")
	fs.BoolVar(&opts.json, "json", false, "JSON output")
	if operation != "decode" {
		fs.Var(&opts.wordlists, "w", "wordlist")
		fs.Var(&opts.wordlists, "wordlist", "wordlist")
		fs.StringVar(&opts.scope, "scope", "", "credential scope")
		fs.StringVar(&opts.username, "username", "", "credential username")
		fs.BoolVar(&opts.saveCredential, "save-credential", false, "save recovered credential")
		fs.BoolVar(&opts.noSaveCredential, "no-save-credential", false, "do not save credentials")
		fs.BoolVar(&opts.yes, "yes", false, "approve expensive recovery")
		fs.BoolVar(&opts.refresh, "refresh", false, "discard saved state for this input")
		fs.BoolVar(&opts.dryRun, "dry-run", false, "show plan only")
	}
	if err := fs.Parse(parsedArgs); err != nil {
		return err
	}
	if opts.saveCredential && opts.noSaveCredential {
		return errors.New("xdec: --save-credential and --no-save-credential cannot be combined")
	}
	if opts.file != "" && opts.stringInput != "" {
		return errors.New("xdec: --file and --string cannot be combined")
	}
	if opts.file != "" && len(fs.Args()) > 0 {
		return errors.New("xdec: --file cannot be combined with a positional input")
	}
	if opts.stringInput != "" && len(fs.Args()) > 0 {
		return errors.New("xdec: --string cannot be combined with a positional input")
	}
	if len(fs.Args()) > 1 {
		return errors.New("xdec: only one positional input is allowed")
	}

	doc, err := readDocument(opts, fs.Args(), stdin)
	if err != nil {
		return err
	}
	parent, workspace := startParentLog(doc, args[1:])
	state, err := openState(workspace, doc, parent)
	if err != nil {
		return err
	}
	runKey := digest(doc.Raw)
	if opts.refresh {
		state.resetRun(runKey, doc, parent)
		if err := state.save(); err != nil {
			return err
		}
	}
	parentOut := &bytes.Buffer{}
	parentErr := &bytes.Buffer{}
	defer func() {
		if parent != 0 && workspace != nil {
			status := "completed"
			_ = ctxpkg.FinishCommandLog(workspace, parent, ctxpkg.CommandLog{
				Status: status, ExitCode: 0, Stdout: parentOut.String(), Stderr: parentErr.String(),
				EndedAt: time.Now().Format(time.RFC3339Nano),
			})
		}
	}()
	if doc.Notice != "" {
		_, _ = fmt.Fprintln(stdout, doc.Notice)
		state.Data.Runs[runKey].Status = "completed"
		_ = state.save()
		return nil
	}

	candidates := extractCandidates(doc, opts)
	if len(candidates) == 0 {
		return errors.New("xdec: no input candidates found")
	}

	var results []result
	var expensive []candidate
	for _, c := range candidates {
		classes := classify(c.Value)
		if opts.operation == "recover" {
			if len(classes) == 0 {
				results = append(results, result{Candidate: c, Status: "not-recoverable"})
				continue
			}
			expensive = append(expensive, c)
			continue
		}
		if len(classes) == 0 {
			if out, kind, ok := decodeInstant(c.Value); ok {
				results = append(results, result{Candidate: c, Kind: kind, Method: "builtin", Value: out, Status: "decoded"})
				continue
			}
			results = append(results, result{Candidate: c, Status: "unresolved"})
			continue
		}
		if out, kind, ok := decodeInstant(c.Value); ok {
			results = append(results, result{Candidate: c, Kind: kind, Method: "builtin", Value: out, Status: "decoded"})
			continue
		}
		if opts.operation == "decode" && len(classes) > 0 {
			results = append(results, result{Candidate: c, Kind: firstKind(c.Value), Status: "recover-required"})
			continue
		}
		expensive = append(expensive, c)
	}

	if opts.json {
		// JSON is emitted once, after expensive actions, so it remains valid.
	}
	if len(expensive) > 0 {
		wls, err := resolveWordlists(opts.wordlists)
		if err != nil {
			return err
		}
		be := selectBackend(expensive[0])
		pending := make([]candidate, 0, len(expensive))
		for _, c := range expensive {
			cs := state.candidate(runKey, c)
			if cs.Status == "cracked" {
				if cs.Recovered != "" {
					results = append(results, result{Candidate: c, Kind: firstKind(c.Value), Method: "cached", Value: cs.Recovered, Status: "cracked"})
				} else {
					results = append(results, result{Candidate: c, Kind: firstKind(c.Value), Status: "skipped"})
				}
				continue
			}
			hasPending := false
			for _, wl := range wls {
				a := cs.Attempts[attemptKey(c, wl, be)]
				if a == nil || a.Status == "pending" || a.Status == "running" || a.Status == "interrupted" {
					hasPending = true
				}
			}
			if hasPending {
				pending = append(pending, c)
			} else {
				cs.Status = "exhausted"
				results = append(results, result{Candidate: c, Kind: firstKind(c.Value), Status: "skipped"})
			}
		}
		if len(pending) == 0 {
			state.Data.Runs[runKey].Status = "completed"
			_ = state.save()
			return writeResults(stdout, results, opts.json)
		}
		wordlistLabel := fmt.Sprintf("%d specified lists", len(wls))
		if len(opts.wordlists) == 0 {
			wordlistLabel = fmt.Sprintf("ctx password set (%d lists)", len(wls))
		}
		if opts.dryRun {
			printPlan(stdout, pending, wordlistLabel, be)
			return nil
		}
		if !opts.yes && !confirm(stdin, stderr, pending, wordlistLabel, be) {
			fmt.Fprintln(stderr, "xdec: confirmation not provided; use --yes when input is piped")
			for _, c := range pending {
				results = append(results, result{Candidate: c, Status: "waiting-confirmation"})
			}
			state.Data.Runs[runKey].Status = "waiting-confirmation"
			_ = state.save()
			return writeResults(stdout, results, opts.json)
		} else {
			cracked := map[string]bool{}
			for _, wl := range wls {
				for _, c := range pending {
					if cracked[c.Value] {
						continue
					}
					cs := state.candidate(runKey, c)
					aKey := attemptKey(c, wl, be)
					if a := cs.Attempts[aKey]; a != nil && a.Status != "pending" && a.Status != "running" && a.Status != "interrupted" {
						continue
					}
					state.markAttempt(runKey, c, wl, be, "running")
					_ = state.save()
					r := crackOne(c, wl, be, workspace, parent, stderr)
					if r.Status == "cracked" {
						state.markAttempt(runKey, c, wl, be, "cracked")
					} else {
						state.markAttempt(runKey, c, wl, be, "unresolved")
					}
					_ = state.save()
					if r.Status == "cracked" && shouldSave(c, opts) && workspace != nil {
						scope := c.Scope
						if scope == "" {
							scope = opts.scope
						}
						user := c.Username
						if user == "" {
							user = opts.username
						}
						if scope != "" && user != "" {
							if _, saveErr := ctxpkg.SetCredential(workspace, scope, user, r.Value); saveErr == nil {
								r.Saved = true
								fmt.Fprintf(stderr, "[+] credential saved: %s/%s\n", scope, user)
							} else {
								fmt.Fprintf(stderr, "[!] credential save failed: %v\n", saveErr)
							}
						}
					}
					results = append(results, r)
					if r.Status == "cracked" {
						state.candidate(runKey, c).Recovered = r.Value
						cracked[c.Value] = true
					}
				}
			}
			for _, c := range pending {
				cs := state.candidate(runKey, c)
				if cracked[c.Value] {
					cs.Status = "cracked"
				} else {
					cs.Status = "exhausted"
				}
			}
			state.Data.Runs[runKey].Status = "completed"
			_ = state.save()
		}
	}

	state.Data.Runs[runKey].Status = "completed"
	_ = state.save()
	return writeResults(stdout, results, opts.json)
}

func readDocument(opts options, args []string, stdin io.Reader) (document, error) {
	if len(args) > 1 {
		return document{}, errors.New("xdec: only one positional input is allowed")
	}
	if opts.file != "" {
		return readFileDocument(opts.file)
	}
	if opts.stringInput != "" {
		return document{Source: "argument", Name: "argument", Raw: []byte(opts.stringInput)}, nil
	}
	if len(args) == 1 {
		if info, err := os.Stat(args[0]); err == nil && info.Mode().IsRegular() {
			return readFileDocument(args[0])
		}
	}
	if len(args) > 0 {
		return document{Source: "argument", Name: "argument", Raw: []byte(strings.Join(args, " "))}, nil
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return document{}, fmt.Errorf("xdec: read stdin: %w", err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return document{}, errors.New("xdec: no input")
	}
	return document{Source: "stdin", Name: "stdin", Raw: b}, nil
}

func readFileDocument(path string) (document, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return document{}, fmt.Errorf("xdec: read %s: %w", path, err)
	}
	doc := document{Source: "file", Name: path, Raw: b}
	if looksLikeSSHPrivateKey(b) {
		encrypted, err := sshPrivateKeyHasPassphrase(path)
		if err != nil {
			return document{}, err
		}
		if !encrypted {
			doc.Kind = "ssh-unencrypted"
			doc.Notice = "xdec: SSH private key is not encrypted\nxdec: no password required"
			return doc, nil
		}
		converted, err := convertSSHPrivateKey(path)
		if err != nil {
			return document{}, err
		}
		doc.Analysis, doc.Kind = converted, "ssh"
	}
	return doc, nil
}

func reorderFlags(args []string) []string {
	var flags, values []string
	valueFlags := map[string]bool{"-f": true, "--file": true, "--string": true, "-w": true, "--wordlist": true, "--scope": true, "--username": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name := arg
		if separator := strings.IndexByte(arg, '='); separator >= 0 {
			name = arg[:separator]
		}
		if valueFlags[name] {
			flags = append(flags, arg)
			if separator := strings.IndexByte(arg, '='); separator < 0 && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			continue
		}
		values = append(values, arg)
	}
	return append(flags, values...)
}

func looksLikeSSHPrivateKey(b []byte) bool {
	return bytes.Contains(b, []byte("PRIVATE KEY-----")) || bytes.Contains(b, []byte("openssh-key-v1\x00"))
}

func convertSSHPrivateKey(path string) ([]byte, error) {
	tool, err := exec.LookPath("ssh2john")
	if err != nil {
		tool, err = exec.LookPath("ssh2john.py")
	}
	if err != nil {
		return nil, errors.New("xdec: ssh private key detected, but ssh2john is not installed")
	}
	cmd := exec.Command(tool, path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("xdec: ssh2john failed: %w", err)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, errors.New("xdec: ssh2john produced no analyzable hash")
	}
	return out, nil
}

func sshPrivateKeyHasPassphrase(path string) (bool, error) {
	tool, err := exec.LookPath("ssh-keygen")
	if err != nil {
		return true, nil
	}
	cmd := exec.Command(tool, "-y", "-P", "", "-f", path)
	if err := cmd.Run(); err == nil {
		return false, nil
	}
	// A non-zero exit normally means that the key rejected the empty
	// passphrase. ssh2john will provide the more specific validation error.
	return true, nil
}

func extractCandidates(doc document, opts options) []candidate {
	input := doc.Raw
	if len(doc.Analysis) > 0 {
		input = doc.Analysis
	}
	text := string(input)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	seen := map[string]bool{}
	var out []candidate
	add := func(c candidate) {
		c.Value = strings.TrimSpace(c.Value)
		if c.Value == "" || seen[c.Value] {
			return
		}
		seen[c.Value] = true
		out = append(out, c)
	}
	if doc.Kind == "ssh" {
		for i, line := range lines {
			if separator := strings.IndexByte(line, ':'); separator >= 0 {
				value := strings.TrimSpace(line[separator+1:])
				if isHashLike(value) {
					add(parseCandidate(value, i+1, opts))
				}
			}
		}
		return out
	}
	if doc.Source == "argument" && !strings.Contains(text, "\n") {
		add(parseCandidate(text, 1, opts))
		return out
	}
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if c := parseCandidate(line, i+1, opts); c.Value != "" && (isHashLike(c.Value) || (!strings.ContainsAny(line, " \t") && printable([]byte(c.Value)))) {
			add(c)
		}
		for _, token := range strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' || r == ',' || r == ';' }) {
			if isHashLike(token) {
				c := parseCandidate(token, i+1, opts)
				fields := strings.Fields(line)
				if len(fields) == 2 && fields[1] == token && !isHashLike(fields[0]) {
					c.Username = fields[0]
				}
				add(c)
			}
		}
	}
	return out
}

func parseCandidate(value string, line int, opts options) candidate {
	c := candidate{Value: strings.TrimSpace(value), Original: value, Line: line, Scope: opts.scope, Username: opts.username}
	if i := strings.IndexByte(c.Value, ':'); i > 0 && i < len(c.Value)-1 {
		left, right := strings.TrimSpace(c.Value[:i]), strings.TrimSpace(c.Value[i+1:])
		if isHashLike(right) {
			c.Username, c.Value = left, right
		}
	}
	return c
}

func isHashLike(v string) bool {
	v = strings.TrimSpace(v)
	return hexHash32.MatchString(v) || hexHash40.MatchString(v) || hexHash64.MatchString(v) || cryptHash.MatchString(v) || sshHash.MatchString(v)
}

func classify(v string) []classification {
	v = strings.TrimSpace(v)
	switch {
	case sshHash.MatchString(v):
		return []classification{{"ssh", .99, "ssh2john format"}}
	case strings.HasPrefix(v, "$2a$") || strings.HasPrefix(v, "$2b$") || strings.HasPrefix(v, "$2y$"):
		return []classification{{"bcrypt", .99, "bcrypt prefix"}}
	case strings.HasPrefix(v, "$argon2"):
		return []classification{{"argon2", .99, "argon2 prefix"}}
	case hexHash32.MatchString(v):
		return []classification{{"md5", .75, "32 hex characters"}, {"ntlm", .55, "32 hex characters"}, {"md4", .40, "32 hex characters"}}
	case hexHash40.MatchString(v):
		return []classification{{"sha1", .85, "40 hex characters"}}
	case hexHash64.MatchString(v):
		return []classification{{"sha256", .85, "64 hex characters"}}
	}
	return nil
}

func decodeInstant(v string) (string, string, bool) {
	trim := strings.TrimSpace(v)
	if decoded, err := base64.StdEncoding.DecodeString(trim); err == nil && utf8.Valid(decoded) && printable(decoded) {
		return string(decoded), "base64", true
	}
	if decoded, err := hex.DecodeString(trim); err == nil && printable(decoded) && string(decoded) != trim {
		return string(decoded), "hex", true
	}
	if n, err := fmt.Sscanf(trim, "%d", new(int)); err == nil && n == 1 {
		// Decimal integer decoding is intentionally left to a future decoder;
		// it must not be confused with an arbitrary numeric password/hash.
	}
	return "", "", false
}

func printable(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if (c < 32 || c == 127) && c != '\n' && c != '\r' && c != '\t' {
			return false
		}
	}
	return true
}

func resolveWordlists(specs []string) ([]wordlist, error) {
	if len(specs) == 0 {
		ws, err := ctxpkg.RecommendWordlists(ctxpkg.WordlistKindPassword)
		if err != nil || len(ws) == 0 {
			return nil, errors.New("xdec: ctx password wordlist unavailable")
		}
		result := make([]wordlist, 0, len(ws))
		seen := map[string]bool{}
		for _, selection := range ws {
			path := resolvedWordlistPath(selection.Path)
			h, err := fileDigest(path)
			if err != nil {
				continue
			}
			if seen[h] {
				continue
			}
			seen[h] = true
			result = append(result, wordlist{ID: selection.Path, Path: path, Lines: selection.Lines, Hash: h})
		}
		if len(result) == 0 {
			return nil, errors.New("xdec: no usable ctx password wordlist")
		}
		return result, nil
	}
	result := make([]wordlist, 0, len(specs))
	for _, spec := range specs {
		wl, err := resolveOneWordlist(spec)
		if err != nil {
			return nil, err
		}
		result = append(result, wl)
	}
	return result, nil
}

func resolveOneWordlist(spec string) (wordlist, error) {
	if st, err := os.Stat(spec); err == nil && !st.IsDir() {
		h, err := fileDigest(spec)
		if err != nil {
			return wordlist{}, err
		}
		return wordlist{ID: filepath.Base(spec), Path: spec, Hash: h}, nil
	}
	for _, w := range mustRecommendWordlists() {
		if w.Path == spec || filepath.Base(w.Path) == spec || w.Provider+":"+w.Path == spec {
			path := resolvedWordlistPath(w.Path)
			h, err := fileDigest(path)
			if err != nil {
				return wordlist{}, err
			}
			return wordlist{ID: spec, Path: path, Lines: w.Lines, Hash: h}, nil
		}
	}
	return wordlist{}, fmt.Errorf("xdec: ctx wordlist not found: %s", spec)
}

func fileDigest(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("xdec: read wordlist %s: %w", path, err)
	}
	return digest(b), nil
}

func resolvedWordlistPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ctxpkg.DiscoverWordlistsRoot(), path)
}

func mustRecommendWordlists() []ctxpkg.WordlistSelection {
	ws, _ := ctxpkg.RecommendWordlists(ctxpkg.WordlistKindPassword)
	return ws
}

func selectBackend(c candidate) backend {
	// Hashcat is selected when present for the common raw hash formats. John
	// remains the fallback and handles crypt formats and *2john converters.
	kind := firstKind(c.Value)
	if p, err := exec.LookPath("hashcat"); err == nil && (kind == "md5" || kind == "sha1" || kind == "sha256" || kind == "ntlm") {
		return backend{Name: "hashcat", Path: p}
	}
	if p, err := exec.LookPath("john"); err == nil {
		return backend{Name: "john", Path: p}
	}
	return backend{Name: "unavailable"}
}

func confirm(r io.Reader, w io.Writer, cs []candidate, wordlistLabel string, be backend) bool {
	if be.Name == "unavailable" {
		fmt.Fprintln(w, "xdec: no password recovery backend (install john or hashcat)")
		return false
	}
	fmt.Fprintf(w, "[?] password recovery may take a long time\n    candidates: %d\n    backend:    %s\n    wordlists:  %s\n    continue? [y/N] ", len(cs), be.Name, wordlistLabel)
	var answer string
	if _, err := fmt.Fscan(r, &answer); err != nil {
		return false
	}
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
}

func crackOne(c candidate, wl wordlist, be backend, workspace *ctxpkg.Workspace, parent int64, stderr io.Writer) result {
	r := result{Candidate: c, Kind: firstKind(c.Value), Method: be.Name, Status: "unresolved"}
	if be.Name == "unavailable" {
		return r
	}
	started := time.Now()
	var child int64
	if workspace != nil && parent != 0 {
		child, _ = ctxpkg.StartCommandLog(workspace, ctxpkg.CommandLog{ParentID: parent, Phase: "password-recovery", Command: "xdec password-recovery", ExpandedCommand: be.Name + " format=" + r.Kind + " wordlist=" + wl.ID, StartedAt: started.Format(time.RFC3339Nano)})
	}
	hashFile, err := os.CreateTemp("", "xdec-hash-*")
	if err != nil {
		return r
	}
	defer os.Remove(hashFile.Name())
	if _, err = fmt.Fprintln(hashFile, c.Value); err != nil {
		return r
	}
	hashFile.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()
	var cmd *exec.Cmd
	var output bytes.Buffer
	var runErr error
	format := johnFormat(r.Kind)
	if be.Name == "hashcat" {
		mode := hashcatMode(r.Kind)
		outFile, _ := os.CreateTemp("", "xdec-out-*")
		if outFile != nil {
			outFile.Close()
			defer os.Remove(outFile.Name())
		}
		args := []string{"-m", mode, "-a", "0", hashFile.Name(), wl.Path, "--quiet"}
		if outFile != nil {
			args = append(args, "--outfile", outFile.Name(), "--outfile-format", "2")
		}
		cmd = exec.CommandContext(ctx, be.Path, args...)
		cmd.Stdout, cmd.Stderr = &output, &output
		runErr = cmd.Run()
		if outFile != nil {
			if b, e := os.ReadFile(outFile.Name()); e == nil && len(bytes.TrimSpace(b)) > 0 {
				parts := strings.SplitN(strings.TrimSpace(string(b)), ":", 2)
				if len(parts) == 2 {
					r.Value, r.Status = parts[1], "cracked"
				}
			}
		}
	} else {
		pot, _ := os.CreateTemp("", "xdec-pot-*")
		if pot != nil {
			pot.Close()
			defer os.Remove(pot.Name())
		}
		args := []string{"--format=" + format, "--wordlist=" + wl.Path, "--pot=" + pot.Name(), hashFile.Name()}
		cmd = exec.CommandContext(ctx, be.Path, args...)
		cmd.Stdout, cmd.Stderr = &output, &output
		runErr = cmd.Run()
		show := exec.CommandContext(ctx, be.Path, "--format="+format, "--pot="+pot.Name(), "--show", hashFile.Name())
		b, _ := show.Output()
		if p := parseJohnPassword(string(b)); p != "" {
			r.Value, r.Status = p, "cracked"
		}
	}
	fmt.Fprintf(stderr, "[i] %s %s: %s (%s)\n", be.Name, r.Kind, r.Status, time.Since(started).Round(time.Millisecond))
	if child != 0 && workspace != nil {
		status := "completed"
		if r.Status != "cracked" {
			status = "unresolved"
		}
		stderrText := ""
		if runErr != nil {
			stderrText = runErr.Error()
		}
		_ = ctxpkg.FinishCommandLog(workspace, child, ctxpkg.CommandLog{Status: status, ExitCode: 0, Stdout: "backend=" + be.Name + "\nstatus=" + r.Status + "\noutput=" + sanitizeOutput(output.String(), c.Value, r.Value) + "\n", Stderr: stderrText, EndedAt: time.Now().Format(time.RFC3339Nano)})
	}
	return r
}

func firstKind(v string) string {
	c := classify(v)
	if len(c) == 0 {
		return "unknown"
	}
	return c[0].Kind
}
func johnFormat(k string) string {
	switch k {
	case "md5":
		return "raw-md5"
	case "sha1":
		return "raw-sha1"
	case "sha256":
		return "raw-sha256"
	case "ntlm":
		return "NT"
	case "md4":
		return "Raw-MD4"
	default:
		return k
	}
}
func hashcatMode(k string) string {
	switch k {
	case "md5":
		return "0"
	case "sha1":
		return "100"
	case "sha256":
		return "1400"
	case "ntlm":
		return "1000"
	default:
		return "0"
	}
}
func parseJohnPassword(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if i := strings.IndexByte(line, ':'); i > 0 {
			return line[i+1:]
		}
	}
	return ""
}

func sanitizeOutput(s string, values ...string) string {
	for _, value := range values {
		if value != "" {
			s = strings.ReplaceAll(s, value, "<redacted>")
		}
	}
	return s
}

func shouldSave(c candidate, o options) bool {
	return !o.noSaveCredential && (o.saveCredential || (c.Username != "" && c.Scope != ""))
}

func startParentLog(doc document, args []string) (int64, *ctxpkg.Workspace) {
	ws, err := ctxpkg.FindWorkspace(".")
	if err != nil || ws == nil {
		return 0, nil
	}
	command := strings.Join(redactArgs(append([]string{"xdec"}, args...)), " ")
	id, err := ctxpkg.StartCommandLog(ws, ctxpkg.CommandLog{Command: command, ExpandedCommand: "xdec", StartedAt: time.Now().Format(time.RFC3339Nano)})
	if err != nil {
		return 0, ws
	}
	return id, ws
}

func redactArgs(args []string) []string {
	out := append([]string(nil), args...)
	for i := range out {
		if i > 0 && !strings.HasPrefix(out[i], "-") {
			out[i] = "<input>"
		}
	}
	for i, a := range out {
		if a == "--username" || a == "--scope" || a == "-w" || a == "--wordlist" || a == "--string" {
			if i+1 < len(out) {
				out[i+1] = "<redacted>"
			}
		}
	}
	return out
}

func printPlan(w io.Writer, cs []candidate, wordlistLabel string, be backend) {
	fmt.Fprintf(w, "xdec plan\ncandidates: %d\nbackend: %s\nwordlists: %s\ncost: expensive\nconfirmation: required\n", len(cs), be.Name, wordlistLabel)
}

func writeResults(w io.Writer, rs []result, jsonOut bool) error {
	if jsonOut {
		return writeJSON(w, rs)
	}
	for _, r := range rs {
		switch r.Status {
		case "decoded", "cracked":
			if identity := resultIdentity(r.Candidate); identity != "" {
				fmt.Fprintf(w, "%s: %s\n", identity, r.Value)
			} else {
				fmt.Fprintln(w, r.Value)
			}
		default:
			fmt.Fprintf(w, "xdec: %s (%s)\n", r.Status, r.Candidate.Value)
		}
	}
	return nil
}

func writeJSON(w io.Writer, rs []result) error {
	type jsonResult struct {
		Candidate string `json:"candidate"`
		Username  string `json:"username,omitempty"`
		Scope     string `json:"scope,omitempty"`
		Kind      string `json:"kind"`
		Method    string `json:"method"`
		Status    string `json:"status"`
		Saved     bool   `json:"saved"`
		Value     string `json:"value,omitempty"`
	}
	out := make([]jsonResult, 0, len(rs))
	for _, r := range rs {
		out = append(out, jsonResult{Candidate: r.Candidate.Value, Username: r.Candidate.Username, Scope: r.Candidate.Scope, Kind: r.Kind, Method: r.Method, Status: r.Status, Saved: r.Saved, Value: r.Value})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

func resultIdentity(c candidate) string {
	return c.Username
}
