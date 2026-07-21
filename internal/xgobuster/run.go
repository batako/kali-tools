package xgobuster

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"req/internal/ctx"
	"req/internal/ctxapi"
	"req/internal/ctxexec"
)

var (
	Version = "1.3.0"
)

const usageText = `usage: xgobuster [dns] [gobuster-options]

Run gobuster dir against the current ctx target, or use dns mode for subdomain enumeration.

The wordlist is selected from ctx recommendations unless -w or --wordlist is provided.
Automatic escalation continues while the configured request limit allows.

options:
  dns                    enumerate DNS subdomains
  -d, --domain <domain>  target domain for dns mode
  -w, --wordlist <path>  use an explicit wordlist
  -u, --url <url>        override the URL derived from the current target
  --host <hostname>      use a registered xhost hostname for the target
  --ip                   use the target IP instead of an xhost hostname
  --service <number>     select a web service by its displayed number
  -c, --cookies <value>   send cookies with requests
  --exclude-status <code> exclude responses with these status codes
  --exclude-length <size> exclude responses with these body sizes
  -k, --no-tls-validation disable TLS certificate validation
  --tls-verify           verify TLS certificates for this run
  --preset <name>        select a technology preset
  -x, --extensions <list> pass extensions to Gobuster (for example php,html,js)
  --status               show wordlist search status
  --clear-cache          clear scoped wordlist progress without running Gobuster
  -h, --help             show this help
  -V, --version          show version`

type ExitCodeError struct{ Code int }

func (err ExitCodeError) Error() string { return fmt.Sprintf("exit code %d", err.Code) }
func (err ExitCodeError) ExitCode() int { return err.Code }

type commandRunner interface {
	LookPath(file string) (string, error)
	Run(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

type realRunner struct{}

func (realRunner) LookPath(file string) (string, error) { return exec.LookPath(file) }

func (realRunner) Run(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := exec.Command(name, args...)
	command.Stdin = stdin
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

type App struct {
	runner commandRunner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

type plannedWordlist struct {
	Selection      ctx.WordlistSelection
	Path           string
	Extra          []string
	BaseWords      []string
	ExtensionWords []string
	Extensions     []string
}

func RunWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return New(realRunner{}, stdin, stdout, stderr).Run(args)
}

func New(runner commandRunner, stdin io.Reader, stdout, stderr io.Writer) *App {
	return &App{runner: runner, stdin: stdin, stdout: stdout, stderr: stderr}
}

func (app *App) Run(args []string) error {
	commandArgs := args[1:]
	if len(commandArgs) == 1 || (len(commandArgs) == 2 && commandArgs[0] == "dns") {
		helpArg := commandArgs[len(commandArgs)-1]
		switch helpArg {
		case "-h", "--help":
			_, err := fmt.Fprintln(app.stdout, usageText)
			return err
		case "-V", "--version":
			_, err := fmt.Fprintf(app.stdout, "xgobuster %s\n", Version)
			return err
		}
	}
	options, err := parseOptions(commandArgs)
	if err != nil {
		return err
	}

	if _, err := ctxexec.LookPath(app.runner); err != nil {
		return app.errorf("ctx is required")
	}
	commands := []string{"gobuster"}
	if options.Status || options.ClearCache {
		commands = nil
	}
	if err := app.requireCommands(commands...); err != nil {
		return err
	}
	prompt, err := app.prompt()
	if err != nil {
		return err
	}
	if !prompt.Active || prompt.WorkspacePath == "" {
		return app.errorf("no active workspace")
	}
	if prompt.TargetIP == nil || strings.TrimSpace(*prompt.TargetIP) == "" {
		return app.errorf("no primary target")
	}

	workspace, err := ctx.InitWorkspace(prompt.WorkspacePath)
	if err != nil {
		return app.errorf("failed to load workspace: %s", err)
	}
	target, err := ctx.GetPrimaryTarget(workspace)
	if err != nil {
		return app.errorf("failed to load primary target: %s", err)
	}
	if options.DNS {
		if (options.Status || options.ClearCache) && options.Wordlist != "" {
			return app.errorf("usage: xgobuster dns --status cannot be combined with --wordlist")
		}
		if options.ClearCache && options.Status {
			return app.errorf("usage: xgobuster dns --clear-cache cannot be combined with --status")
		}
		if options.Preset != "" || options.URL != "" || options.Host != "" || options.IP || options.Service != 0 || options.Cookie != "" || options.ExcludeStatus != "" || options.ExcludeLength != "" || options.Insecure || options.VerifyTLS {
			return app.errorf("dns mode accepts -d, -w, --status, --clear-cache, and Gobuster DNS options only")
		}
		hosts, hostErr := ctx.ListHosts(workspace)
		if hostErr != nil {
			return app.errorf("failed to load hosts: %s", hostErr)
		}
		domain, domainErr := resolveDNSDomain(*prompt.TargetIP, hosts, options.Domain, app.stdin, app.stdout)
		if domainErr != nil {
			return domainErr
		}
		options.Domain = domain
		return app.runDNS(workspace, target, options, args[1:])
	}

	if (options.Status || options.ClearCache) && options.Wordlist != "" {
		return app.errorf("usage: xgobuster --status|--clear-cache [--preset <name>] [-x <list>] [--url <url>]")
	}
	if options.Status && options.ClearCache {
		return app.errorf("usage: --clear-cache cannot be combined with --status")
	}
	config, configErr := ctx.LoadConfig()
	if configErr != nil {
		return app.errorf("failed to load config: %s", configErr)
	}
	if !config.TLSVerify && !options.VerifyTLS {
		options.Insecure = true
	}
	if options.URL != "" && (options.Host != "" || options.IP) {
		return app.errorf("usage: --url cannot be combined with --host or --ip")
	}
	if options.Host != "" && options.IP {
		return app.errorf("usage: --host cannot be combined with --ip")
	}
	if options.Insecure && options.VerifyTLS {
		return app.errorf("usage: -k cannot be combined with --tls-verify")
	}
	wordlist := options.Wordlist
	url := options.URL
	if url == "" {
		hosts, hostErr := ctx.ListHosts(workspace)
		if hostErr != nil {
			return app.errorf("failed to load hosts: %s", hostErr)
		}
		targetHost, targetHostErr := resolveTargetHost(*prompt.TargetIP, hosts, options.Host, options.IP, app.stdin, app.stdout)
		if targetHostErr != nil {
			return targetHostErr
		}
		services, serviceErr := app.services()
		if serviceErr != nil {
			return serviceErr
		}
		url, err = resolveURL(targetHost, services, options.Service, app.stdin, app.stdout)
		if err != nil {
			return err
		}
	} else if options.Service != 0 {
		return app.errorf("usage: --service cannot be combined with --url")
	}
	if options.Wordlist == "" {
		if err := resolveExecutionStrategy(&options); err != nil {
			return app.errorf("failed to resolve technology preset: %s", err)
		}
	}
	if options.Status {
		return app.showStatus(workspace, target, url, options)
	}
	historyURLs, historyErr := compatibleHistoryURLs(workspace, target, url)
	if historyErr != nil {
		return app.errorf("failed to load web wordlist history: %s", historyErr)
	}
	if options.ClearCache {
		if err := clearSearchedWordsForURLs(workspace, target.ID, historyURLs, effectiveExtra(options)); err != nil {
			return app.errorf("failed to clear web wordlist state: %s", err)
		}
		_, _ = fmt.Fprintf(app.stdout, "Cleared web wordlist cache for %s\n", url)
		return nil
	}
	selection := ctx.WordlistSelection{Provider: "manual", Path: wordlist}

	planned := []plannedWordlist{{Selection: selection, Path: selection.Path, Extra: effectiveExtra(options)}}
	if wordlist == "" {
		config, configErr := ctx.LoadConfig()
		if configErr != nil {
			return app.errorf("failed to load wordlist config: %s", configErr)
		}
		candidates, listErr := recommendWordlists(ctx.WordlistKindDirectory)
		if listErr != nil {
			return app.errorf("failed to select wordlist: %s", listErr)
		}
		if len(candidates) == 0 {
			return app.errorf("no directory wordlists found")
		}
		baseSearched, loadErr := loadSearchedWordsForURLs(workspace, target.ID, historyURLs, "base", "")
		if loadErr != nil {
			return app.errorf("failed to load wordlist state: %s", loadErr)
		}
		legacySearched, legacyErr := loadSearchedWordsForURLs(workspace, target.ID, historyURLs, "legacy", strings.Join(effectiveExtra(options), "\x00"))
		if legacyErr != nil {
			return app.errorf("failed to load legacy wordlist state: %s", legacyErr)
		}
		mergeSearchedWords(baseSearched, legacySearched)
		extensions := extensionsFromExtra(effectiveExtra(options))
		extensionSearched := make(map[string]map[string]struct{}, len(extensions))
		for _, extension := range extensions {
			words, wordsErr := loadSearchedWordsForURLs(workspace, target.ID, historyURLs, "extension", extension)
			if wordsErr != nil {
				return app.errorf("failed to load extension wordlist state: %s", wordsErr)
			}
			extensionSearched[extension] = words
		}
		planned = planned[:0]
		usedRequests := 0
		requestLimit := searchRequestLimit(config, options)
		for i := 0; i < len(candidates); i++ {
			candidate := candidates[i]
			words, countErr := filteredWordlist(candidate.Path, baseSearched)
			if countErr != nil {
				return app.errorf("failed to prepare wordlist %s: %s", candidate.Path, countErr)
			}
			allWords := words
			if searchModeFromOptions(options) == "file" {
				allWords, countErr = filteredWordlist(candidate.Path, nil)
				if countErr != nil {
					return app.errorf("failed to prepare wordlist %s: %s", candidate.Path, countErr)
				}
				words, countErr = filteredWordlist(candidate.Path, baseSearched)
				if countErr != nil {
					return app.errorf("failed to prepare wordlist %s: %s", candidate.Path, countErr)
				}
			}
			if searchModeFromOptions(options) != "file" {
				if len(words) == 0 {
					continue
				}
				remainingRequests := requestLimit - usedRequests
				if len(words) > remainingRequests {
					words = words[:remainingRequests]
				}
				if len(words) == 0 {
					break
				}
				temporaryPath, tempErr := writeTemporaryWordlist(words)
				if tempErr != nil {
					return app.errorf("failed to create filtered wordlist: %s", tempErr)
				}
				planned = append(planned, plannedWordlist{Selection: candidate, Path: temporaryPath, Extra: effectiveExtra(options), BaseWords: words})
				usedRequests += len(words)
				for _, word := range words {
					baseSearched[word] = struct{}{}
				}
				continue
			}

			missingBase := make([]string, 0, len(words))
			missingExtensions := make([]string, 0)
			for _, word := range words {
				missingBase = append(missingBase, word)
			}
			for _, word := range allWords {
				if _, ok := baseSearched[word]; !ok {
					continue
				}
				for _, extension := range extensions {
					request := word + "." + extension
					if _, ok := extensionSearched[extension][request]; !ok {
						missingExtensions = append(missingExtensions, request)
					}
				}
			}
			if len(missingBase) > 0 {
				requestCount := len(missingBase) * (len(extensions) + 1)
				remainingRequests := requestLimit - usedRequests
				maxWords := remainingRequests / (len(extensions) + 1)
				if maxWords < 1 {
					break
				}
				if len(missingBase) > maxWords {
					missingBase = missingBase[:maxWords]
					requestCount = len(missingBase) * (len(extensions) + 1)
				}
				temporaryPath, tempErr := writeTemporaryWordlist(missingBase)
				if tempErr != nil {
					return app.errorf("failed to create filtered wordlist: %s", tempErr)
				}
				planned = append(planned, plannedWordlist{Selection: candidate, Path: temporaryPath, Extra: effectiveExtra(options), BaseWords: missingBase, Extensions: extensions})
				usedRequests += requestCount
				for _, word := range missingBase {
					baseSearched[word] = struct{}{}
					for _, extension := range extensions {
						extensionSearched[extension][word+"."+extension] = struct{}{}
					}
				}
			}
			if len(missingExtensions) > 0 && usedRequests < requestLimit {
				remainingRequests := requestLimit - usedRequests
				if len(missingExtensions) > remainingRequests {
					missingExtensions = missingExtensions[:remainingRequests]
				}
				if len(missingExtensions) > 0 {
					temporaryPath, tempErr := writeTemporaryWordlist(missingExtensions)
					if tempErr != nil {
						return app.errorf("failed to create filtered extension wordlist: %s", tempErr)
					}
					planned = append(planned, plannedWordlist{Selection: candidate, Path: temporaryPath, Extra: withoutExtensionsOption(effectiveExtra(options)), ExtensionWords: missingExtensions, Extensions: extensions})
					usedRequests += len(missingExtensions)
					for _, request := range missingExtensions {
						for _, extension := range extensions {
							if strings.HasSuffix(request, "."+extension) {
								extensionSearched[extension][request] = struct{}{}
							}
						}
					}
				}
			}
			if usedRequests >= requestLimit {
				break
			}
		}
		if len(planned) == 0 {
			return app.errorf("all configured web wordlists have completed; use --clear-cache to restart")
		}
	}
	for _, item := range planned {
		if err := app.runWordlist(workspace, target, url, options, item); err != nil {
			return err
		}
		if item.Selection.Provider != "manual" {
			if len(item.BaseWords) > 0 {
				statePath, stateErr := searchedBaseWordsPath(workspace, target.ID, url)
				if stateErr != nil {
					return app.errorf("failed to prepare wordlist state: %s", stateErr)
				}
				if err := appendSearchedWords(statePath, item.BaseWords); err != nil {
					return app.errorf("failed to save wordlist state: %s", err)
				}
				for _, extension := range item.Extensions {
					path, pathErr := searchedExtensionWordsPath(workspace, target.ID, url, extension)
					if pathErr != nil {
						return app.errorf("failed to prepare extension wordlist state: %s", pathErr)
					}
					words := make([]string, 0, len(item.BaseWords))
					for _, word := range item.BaseWords {
						words = append(words, word+"."+extension)
					}
					if err := appendSearchedWords(path, words); err != nil {
						return app.errorf("failed to save extension wordlist state: %s", err)
					}
				}
			}
			if len(item.ExtensionWords) > 0 {
				for _, extension := range item.Extensions {
					path, pathErr := searchedExtensionWordsPath(workspace, target.ID, url, extension)
					if pathErr != nil {
						return app.errorf("failed to prepare extension wordlist state: %s", pathErr)
					}
					words := filterExtensionWords(item.ExtensionWords, extension)
					if err := appendSearchedWords(path, words); err != nil {
						return app.errorf("failed to save extension wordlist state: %s", err)
					}
				}
			}
		}
	}
	return nil
}

func searchRequestLimit(config *ctx.Config, options parsedOptions) int {
	if searchModeFromOptions(options) == "file" {
		return config.FileMaxRequests
	}
	return config.DirectoryMaxRequests
}

func (app *App) runDNS(workspace *ctx.Workspace, target *ctx.Target, options parsedOptions, originalArgs []string) error {
	statePath, err := dnsSearchedWordsPath(workspace, target.ID, options.Domain)
	if err != nil {
		return app.errorf("failed to prepare DNS wordlist state: %s", err)
	}
	searched, err := loadSearchedWords(statePath)
	if err != nil {
		return app.errorf("failed to load DNS wordlist state: %s", err)
	}
	if options.ClearCache {
		if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return app.errorf("failed to clear DNS wordlist state: %s", err)
		}
		_, _ = fmt.Fprintf(app.stdout, "Cleared DNS search cache for %s\n", options.Domain)
		return nil
	}
	config, err := ctx.LoadConfig()
	if err != nil {
		return app.errorf("failed to load config: %s", err)
	}
	candidates, err := discoverDNSWordlists()
	if err != nil {
		return app.errorf("failed to select DNS wordlists: %s", err)
	}
	if options.Status {
		return app.showDNSStatus(options.Domain, candidates, searched)
	}
	if options.Wordlist != "" {
		candidates = []ctx.WordlistSelection{{Provider: "manual", Profile: "manual", Type: "dns", Path: options.Wordlist}}
	}
	var selected ctx.WordlistSelection
	var words []string
	for _, candidate := range candidates {
		if _, statErr := os.Stat(candidate.Path); statErr != nil {
			continue
		}
		candidateSeen := searched
		if candidate.Provider == "manual" {
			candidateSeen = nil
		}
		candidateWords, wordErr := filteredWordlist(candidate.Path, candidateSeen)
		if wordErr != nil {
			return app.errorf("failed to prepare DNS wordlist %s: %s", candidate.Path, wordErr)
		}
		if len(candidateWords) == 0 {
			continue
		}
		selected, words = candidate, candidateWords
		break
	}
	if len(words) == 0 {
		return app.errorf("all DNS wordlists have completed; use --clear-cache to restart")
	}
	if len(words) > config.DNSMaxQueries {
		words = words[:config.DNSMaxQueries]
	}
	temporaryPath, err := writeTemporaryWordlist(words)
	if err != nil {
		return app.errorf("failed to create filtered DNS wordlist: %s", err)
	}
	defer os.Remove(temporaryPath)
	logWordlist := selected.Path
	gobusterArgs := []string{"dns", "--domain", options.Domain, "-w", temporaryPath}
	gobusterArgs = append(gobusterArgs, options.Extra...)
	logArgs := []string{"dns", "--domain", options.Domain, "-w", logWordlist}
	logArgs = append(logArgs, options.Extra...)
	startedAt := time.Now().UTC()
	logID, err := ctx.StartCommandLog(workspace, ctx.CommandLog{Command: formatCommand("xgobuster", originalArgs), ExpandedCommand: formatCommand("gobuster", logArgs), StartedAt: startedAt.Format(time.RFC3339Nano)})
	if err != nil {
		return app.errorf("failed to start command log: %s", err)
	}
	_, _ = fmt.Fprintf(app.stdout, "Running gobuster DNS against %s with %s...\n", options.Domain, logWordlist)
	var commandStdout, commandStderr bytes.Buffer
	runErr := app.runner.Run("gobuster", gobusterArgs, app.stdin, io.MultiWriter(app.stdout, &commandStdout), io.MultiWriter(app.stderr, &commandStderr))
	status, exitCode := "success", 0
	if runErr != nil {
		status, exitCode = "failed", commandExitCode(runErr)
	}
	endedAt := time.Now().UTC()
	if err := ctx.FinishCommandLog(workspace, logID, ctx.CommandLog{Status: status, ExitCode: exitCode, Stdout: commandStdout.String(), Stderr: commandStderr.String(), EndedAt: endedAt.Format(time.RFC3339Nano)}); err != nil {
		return app.errorf("failed to finish command log: %s", err)
	}
	if runErr == nil {
		for _, hostname := range parseDNSHosts(commandStdout.String(), options.Domain) {
			if _, err := ctx.AddHost(workspace, hostname, target.Name); err != nil {
				return app.errorf("failed to save DNS host %s: %s", hostname, err)
			}
		}
	}
	if runErr == nil && selected.Provider != "manual" {
		if err := appendSearchedWords(statePath, words); err != nil {
			return app.errorf("failed to save DNS wordlist state: %s", err)
		}
	}
	return runErr
}

func parseDNSHosts(output, domain string) []string {
	suffix := "." + strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	seen := make(map[string]struct{})
	var hosts []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Found:") {
			continue
		}
		hostname := strings.TrimSpace(strings.TrimPrefix(line, "Found:"))
		hostname = strings.TrimSuffix(hostname, ".")
		lower := strings.ToLower(hostname)
		if hostname == "" || lower == strings.TrimPrefix(suffix, ".") || !strings.HasSuffix(lower, suffix) {
			continue
		}
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		hosts = append(hosts, hostname)
	}
	return hosts
}

func discoverDNSWordlists() ([]ctx.WordlistSelection, error) {
	candidates, err := recommendWordlists(ctx.WordlistKindSubdomain)
	if err != nil {
		return nil, fmt.Errorf("%w; use --wordlist to override", err)
	}
	for i := range candidates {
		candidates[i].Profile = "dns-" + candidates[i].Tier
		candidates[i].Type = "dns"
	}
	return candidates, nil
}

func (app *App) showDNSStatus(domain string, candidates []ctx.WordlistSelection, searched map[string]struct{}) error {
	type statusEntry struct {
		candidate ctx.WordlistSelection
		status    string
		total     int
	}
	entries := make([]statusEntry, 0, len(candidates))
	total, covered := 0, 0
	for _, candidate := range candidates {
		words, err := loadWordlistWords(candidate.Path)
		if err != nil {
			return app.errorf("failed to inspect DNS wordlist %s: %s", candidate.Path, err)
		}
		count := 0
		for word := range words {
			if _, ok := searched[word]; ok {
				count++
			}
		}
		total += len(words)
		covered += count
		status := "pending"
		if count == len(words) {
			status = "success"
		} else if count > 0 {
			status = "partial"
		}
		entries = append(entries, statusEntry{candidate: candidate, status: status, total: len(words)})
	}
	completed := 0
	for _, entry := range entries {
		if entry.status == "success" {
			completed++
		}
	}
	_, _ = fmt.Fprintf(app.stdout, "DNS wordlist status for %s\n", domain)
	_, _ = fmt.Fprintln(app.stdout, "Mode: dns")
	_, _ = fmt.Fprintf(app.stdout, "Total: %d  Completed: %d  Shared-cache covered: %d  Pending: %d\n", len(entries), completed, covered, len(entries)-completed-covered)
	_, _ = fmt.Fprintf(app.stdout, "Entries: %d total, %d shared-cache covered, %d remaining\n", total, covered, total-covered)
	_, _ = fmt.Fprintf(app.stdout, "Searched unique words: %d\n\n", len(searched))
	for _, entry := range entries {
		_, _ = fmt.Fprintf(app.stdout, "[%s] %-13s %6d words  %s\n", entry.status, entry.candidate.Profile, entry.total, entry.candidate.Path)
	}
	return nil
}

func dnsSearchedWordsPath(workspace *ctx.Workspace, targetID int64, domain string) (string, error) {
	digest := sha256.Sum256([]byte(domain))
	directory := filepath.Join(workspace.DataPath, "dns-wordlists", strconv.FormatInt(targetID, 10), hex.EncodeToString(digest[:]))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return filepath.Join(directory, "searched.words"), nil
}

func searchExtensionCount(extra []string) int {
	for i, arg := range extra {
		var value string
		switch {
		case arg == "-x" || arg == "--extensions":
			if i+1 < len(extra) {
				value = extra[i+1]
			}
		case strings.HasPrefix(arg, "-x="):
			value = strings.TrimPrefix(arg, "-x=")
		case strings.HasPrefix(arg, "--extensions="):
			value = strings.TrimPrefix(arg, "--extensions=")
		}
		if value != "" {
			count := 0
			for _, extension := range strings.Split(value, ",") {
				if strings.TrimSpace(extension) != "" {
					count++
				}
			}
			if count > 0 {
				return count
			}
		}
	}
	return 1
}

func (app *App) runWordlist(workspace *ctx.Workspace, target *ctx.Target, url string, options parsedOptions, item plannedWordlist) error {
	selection := item.Selection
	wordlist := item.Path
	logWordlist := selection.Path
	if wordlist != logWordlist {
		defer os.Remove(wordlist)
	}
	extra := item.Extra
	if extra == nil {
		extra = effectiveExtra(options)
	}
	gobusterArgs := []string{"dir", "-u", url, "-w", wordlist}
	gobusterArgs = append(gobusterArgs, extra...)
	startedAt := time.Now().UTC()
	logArgs := []string{"dir", "-u", url, "-w", logWordlist}
	logArgs = append(logArgs, extra...)
	logID, err := ctx.StartCommandLog(workspace, ctx.CommandLog{Command: "xgobuster", ExpandedCommand: formatCommand("gobuster", logArgs), StartedAt: startedAt.Format(time.RFC3339Nano)})
	if err != nil {
		return app.errorf("failed to start command log: %s", err)
	}
	runID := int64(0)
	if selection.Provider != "manual" {
		runID, err = ctx.StartWebWordlistRun(workspace, target, url, selection.Provider, runProfile(selection), searchSignature(options), logWordlist, startedAt.Format(time.RFC3339Nano), logID)
		if err != nil {
			return app.errorf("failed to start wordlist run: %s", err)
		}
	}
	_, _ = fmt.Fprintf(app.stdout, "Running gobuster against %s with %s...\n", url, logWordlist)
	var commandStdout, commandStderr bytes.Buffer
	runErr := app.runner.Run("gobuster", gobusterArgs, app.stdin, io.MultiWriter(app.stdout, &commandStdout), io.MultiWriter(app.stderr, &commandStderr))
	status := "success"
	exitCode := 0
	if runErr != nil {
		status, exitCode = "failed", commandExitCode(runErr)
	}
	endedAt := time.Now().UTC()
	if err := ctx.FinishCommandLog(workspace, logID, ctx.CommandLog{Status: status, ExitCode: exitCode, Stdout: commandStdout.String(), Stderr: commandStderr.String(), EndedAt: endedAt.Format(time.RFC3339Nano)}); err != nil {
		return app.errorf("failed to finish command log: %s", err)
	}
	if runID > 0 {
		if err := ctx.FinishWebWordlistRun(workspace, runID, status, endedAt.Format(time.RFC3339Nano)); err != nil {
			return app.errorf("failed to finish wordlist run: %s", err)
		}
	}
	for _, discovery := range parseDiscoveries(commandStdout.String(), url, logWordlist, logID) {
		discovery.TargetID = target.ID
		if _, err := ctx.SaveWebDiscovery(workspace, target, discovery); err != nil {
			return app.errorf("failed to save web discovery: %s", err)
		}
	}
	return runErr
}

func (app *App) showStatus(workspace *ctx.Workspace, target *ctx.Target, url string, options parsedOptions) error {
	candidates, err := recommendWordlists(ctx.WordlistKindDirectory)
	if err != nil {
		return app.errorf("failed to select wordlists: %s", err)
	}
	if len(candidates) == 0 {
		return app.errorf("no directory wordlists found")
	}
	allRuns, err := ctx.ListWebWordlistRunsForTarget(workspace, target)
	if err != nil {
		return app.errorf("failed to load wordlist run history: %s", err)
	}
	historyURLs := compatibleURLList(url, allRuns)
	runs := make([]ctx.WebWordlistRun, 0, len(allRuns))
	for _, run := range allRuns {
		if containsString(historyURLs, run.URL) {
			runs = append(runs, run)
		}
	}
	searched, err := loadSearchedWordsForURLs(workspace, target.ID, historyURLs, "base", "")
	if err != nil {
		return app.errorf("failed to load wordlist state: %s", err)
	}
	legacySearched, legacyErr := loadSearchedWordsForURLs(workspace, target.ID, historyURLs, "legacy", strings.Join(effectiveExtra(options), "\x00"))
	if legacyErr != nil {
		return app.errorf("failed to load legacy wordlist state: %s", legacyErr)
	}
	mergeSearchedWords(searched, legacySearched)
	extensions := extensionsFromExtra(effectiveExtra(options))
	extensionSearched := make(map[string]map[string]struct{}, len(extensions))
	for _, extension := range extensions {
		words, wordsErr := loadSearchedWordsForURLs(workspace, target.ID, historyURLs, "extension", extension)
		if wordsErr != nil {
			return app.errorf("failed to load extension wordlist state: %s", wordsErr)
		}
		extensionSearched[extension] = words
	}
	runByWordlist := make(map[string]ctx.WebWordlistRun)
	for _, run := range runs {
		runByWordlist[runProfileKey(run.Profile, run.SearchSignature, run.Wordlist)] = run
	}

	type statusEntry struct {
		candidate ctx.WordlistSelection
		status    string
		total     int
	}
	entries := make([]statusEntry, 0, len(candidates))
	totalWords := 0
	checkedWords := 0
	completed := 0
	covered := 0
	for _, candidate := range candidates {
		lineCount, countErr := wordlistLineCount(candidate.Path)
		if countErr != nil {
			return app.errorf("failed to inspect wordlist %s: %s", candidate.Path, countErr)
		}
		remaining, remainingErr := filteredWordlist(candidate.Path, searched)
		if remainingErr != nil {
			return app.errorf("failed to inspect wordlist %s: %s", candidate.Path, remainingErr)
		}
		checked := lineCount - len(remaining)
		status := "pending"
		run, ok := runByWordlist[runProfileKey(runProfile(candidate), searchSignature(options), candidate.Path)]
		remainingRequests := len(remaining)
		if searchModeFromOptions(options) == "file" {
			remainingRequests = 0
			allWords, allErr := loadWordlistWords(candidate.Path)
			if allErr != nil {
				return app.errorf("failed to inspect wordlist %s: %s", candidate.Path, allErr)
			}
			for word := range allWords {
				if _, ok := searched[word]; !ok {
					remainingRequests += len(extensions) + 1
					continue
				}
				for _, extension := range extensions {
					if _, ok := extensionSearched[extension][word+"."+extension]; !ok {
						remainingRequests++
					}
				}
			}
		}
		if ok && run.Status == "success" && remainingRequests == 0 {
			completed++
			status = "success"
		} else if remainingRequests == 0 {
			covered++
			status = "covered"
		} else if ok {
			status = run.Status
			if status == "success" {
				status = "partial"
			}
		}
		totalWords += lineCount
		checkedWords += checked
		entries = append(entries, statusEntry{candidate: candidate, status: status, total: lineCount})
	}
	_, _ = fmt.Fprintf(app.stdout, "Wordlist status for %s\n", url)
	_, _ = fmt.Fprintf(app.stdout, "Mode: %s\n", searchModeFromOptions(options))
	_, _ = fmt.Fprintf(app.stdout, "Total: %d  Completed: %d  Shared-cache covered: %d  Pending: %d\n", len(candidates), completed, covered, len(candidates)-completed-covered)
	_, _ = fmt.Fprintf(app.stdout, "Entries: %d total, %d shared-cache covered, %d remaining\n", totalWords, checkedWords, totalWords-checkedWords)
	displayedSearched := searched
	searchedLabel := "Searched unique words"
	_, _ = fmt.Fprintf(app.stdout, "%s: %d\n\n", searchedLabel, len(displayedSearched))
	for _, entry := range entries {
		_, _ = fmt.Fprintf(app.stdout, "[%s] %-8s %8d words  %s\n", entry.status, entry.candidate.Profile, entry.total, entry.candidate.Path)
	}
	return nil
}

var recommendWordlists = ctx.RecommendWordlists

func runProfile(selection ctx.WordlistSelection) string {
	return selection.Profile
}

func runProfileKey(profile, searchSignature, wordlist string) string {
	return profile + "\x00" + searchSignature + "\x00" + wordlist
}

func resolveExecutionStrategy(options *parsedOptions) error {
	if options.Preset != "" {
		if err := applyPreset(options); err != nil {
			return err
		}
	}
	options.Mode = searchModeFromOptions(*options)
	return nil
}

func effectiveExtra(options parsedOptions) []string {
	extra := append([]string(nil), options.Extra...)
	if hasExtensionsOption(options.Extra) {
		// Keep explicitly supplied Gobuster extensions unchanged.
	} else if options.PresetExtensions != "" {
		extra = append(extra, "-x", options.PresetExtensions)
	}
	if options.Insecure {
		extra = append(extra, "-k")
	}
	if options.Cookie != "" {
		extra = append(extra, "--cookies", options.Cookie)
	}
	if options.ExcludeStatus != "" {
		extra = append(extra, "--status-codes-blacklist", excludedStatusCodes(options.ExcludeStatus))
	}
	if options.ExcludeLength != "" {
		extra = append(extra, "--exclude-length", options.ExcludeLength)
	}
	return extra
}

func excludedStatusCodes(value string) string {
	value = strings.TrimSpace(value)
	for _, item := range strings.Split(value, ",") {
		if strings.TrimSpace(item) == "404" {
			return value
		}
	}
	return "404," + value
}

func searchSignature(options parsedOptions) string {
	return strings.Join(effectiveExtra(options), "\x00")
}

func searchModeFromOptions(options parsedOptions) string {
	if options.Mode != "" {
		return options.Mode
	}
	if options.PresetExtensions != "" || hasExtensionsOption(options.Extra) {
		return "file"
	}
	return "directory"
}

func applyPreset(options *parsedOptions) error {
	preset := strings.ToLower(strings.TrimSpace(options.Preset))
	if preset == "" || preset == "unknown" {
		return nil
	}
	extensions, ok := technologyPresetValues(preset)
	if !ok {
		if options.Preset != "" {
			return fmt.Errorf("unknown technology preset: %s", preset)
		}
		return nil
	}
	options.Preset = preset
	options.PresetExtensions = extensions
	options.Mode = "file"
	return nil
}

func technologyPresetValues(preset string) (string, bool) {
	switch preset {
	case "php", "wordpress":
		return "php,inc,phps", true
	case "aspnet":
		return "asp,aspx,config", true
	case "java":
		return "jsp,do,action", true
	case "node":
		return "js,json", true
	case "static":
		return "html,htm,js", true
	default:
		return "", false
	}
}

func hasExtensionsOption(extra []string) bool {
	for _, arg := range extra {
		if arg == "-x" || arg == "--extensions" || strings.HasPrefix(arg, "-x=") || strings.HasPrefix(arg, "--extensions=") {
			return true
		}
	}
	return false
}

func wordlistLineCount(path string) (int, error) {
	words, err := loadWordlistWords(path)
	if err != nil {
		return 0, err
	}
	return len(words), nil
}

func loadWordlistWords(path string) (map[string]struct{}, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	seen := make(map[string]struct{})
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word != "" {
			seen[word] = struct{}{}
		}
	}
	return seen, scanner.Err()
}

func searchedWordsPath(workspace *ctx.Workspace, targetID int64, url string, extra []string) (string, error) {
	key := url + "\x00" + strings.Join(extra, "\x00")
	digest := sha256.Sum256([]byte(key))
	directory := filepath.Join(workspace.DataPath, "web-wordlists", strconv.FormatInt(targetID, 10), hex.EncodeToString(digest[:]))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return filepath.Join(directory, "searched.words"), nil
}

func searchedBaseWordsPath(workspace *ctx.Workspace, targetID int64, url string) (string, error) {
	return searchedStatePath(workspace, targetID, url+"\x00base", "searched.words")
}

func searchedExtensionWordsPath(workspace *ctx.Workspace, targetID int64, url, extension string) (string, error) {
	return searchedStatePath(workspace, targetID, url+"\x00extension\x00"+extension, "searched.words")
}

func searchedStatePath(workspace *ctx.Workspace, targetID int64, key, filename string) (string, error) {
	digest := sha256.Sum256([]byte(key))
	directory := filepath.Join(workspace.DataPath, "web-wordlists", strconv.FormatInt(targetID, 10), hex.EncodeToString(digest[:]))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return filepath.Join(directory, filename), nil
}

func searchedWordsProfilePath(workspace *ctx.Workspace, targetID int64, url string, extra []string, profile string) (string, error) {
	path, err := searchedWordsPath(workspace, targetID, url, extra)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), profile+".searched.words"), nil
}

func compatibleHistoryURLs(workspace *ctx.Workspace, target *ctx.Target, currentURL string) ([]string, error) {
	runs, err := ctx.ListWebWordlistRunsForTarget(workspace, target)
	if err != nil {
		return nil, err
	}
	return compatibleURLList(currentURL, runs), nil
}

func compatibleURLList(currentURL string, runs []ctx.WebWordlistRun) []string {
	urls := []string{currentURL}
	seen := map[string]struct{}{currentURL: struct{}{}}
	for _, run := range runs {
		if _, ok := seen[run.URL]; ok || !compatibleWebURL(currentURL, run.URL) {
			continue
		}
		seen[run.URL] = struct{}{}
		urls = append(urls, run.URL)
	}
	return urls
}

func compatibleWebURL(currentURL, previousURL string) bool {
	current, currentErr := url.Parse(currentURL)
	previous, previousErr := url.Parse(previousURL)
	if currentErr != nil || previousErr != nil || current.Scheme != previous.Scheme || current.Port() != previous.Port() || current.EscapedPath() != previous.EscapedPath() || current.RawQuery != previous.RawQuery {
		return false
	}
	currentIP := net.ParseIP(current.Hostname())
	previousIP := net.ParseIP(previous.Hostname())
	return currentIP != nil && previousIP != nil && currentIP.To4() != nil && previousIP.To4() != nil
}

func loadSearchedWordsForURLs(workspace *ctx.Workspace, targetID int64, urls []string, kind, value string) (map[string]struct{}, error) {
	merged := make(map[string]struct{})
	for _, candidateURL := range urls {
		var path string
		var err error
		switch kind {
		case "base":
			path, err = searchedBaseWordsPath(workspace, targetID, candidateURL)
		case "legacy":
			var extra []string
			if value != "" {
				extra = strings.Split(value, "\x00")
			}
			path, err = searchedWordsPath(workspace, targetID, candidateURL, extra)
		case "extension":
			path, err = searchedExtensionWordsPath(workspace, targetID, candidateURL, value)
		default:
			return nil, fmt.Errorf("unknown searched word state: %s", kind)
		}
		if err != nil {
			return nil, err
		}
		words, loadErr := loadSearchedWords(path)
		if loadErr != nil {
			return nil, loadErr
		}
		mergeSearchedWords(merged, words)
	}
	return merged, nil
}

func clearSearchedWordsForURLs(workspace *ctx.Workspace, targetID int64, urls []string, extra []string) error {
	exts := extensionsFromExtra(extra)
	for _, candidateURL := range urls {
		paths := make([]string, 0, len(exts)+2)
		base, err := searchedBaseWordsPath(workspace, targetID, candidateURL)
		if err != nil {
			return err
		}
		legacy, err := searchedWordsPath(workspace, targetID, candidateURL, extra)
		if err != nil {
			return err
		}
		paths = append(paths, base, legacy)
		for _, extension := range exts {
			path, pathErr := searchedExtensionWordsPath(workspace, targetID, candidateURL, extension)
			if pathErr != nil {
				return pathErr
			}
			paths = append(paths, path)
		}
		for _, path := range paths {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func loadSearchedWords(path string) (map[string]struct{}, error) {
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

func mergeSearchedWords(target, source map[string]struct{}) {
	for word := range source {
		target[word] = struct{}{}
	}
}

func filteredWordlist(path string, seen map[string]struct{}) ([]string, error) {
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
		if word == "" || hasWord(seen, word) {
			continue
		}
		if _, exists := local[word]; exists {
			continue
		}
		local[word] = struct{}{}
		words = append(words, word)
	}
	return words, scanner.Err()
}

func extensionsFromExtra(extra []string) []string {
	for i, arg := range extra {
		var value string
		switch {
		case arg == "-x" || arg == "--extensions":
			if i+1 < len(extra) {
				value = extra[i+1]
			}
		case strings.HasPrefix(arg, "-x="):
			value = strings.TrimPrefix(arg, "-x=")
		case strings.HasPrefix(arg, "--extensions="):
			value = strings.TrimPrefix(arg, "--extensions=")
		}
		if value == "" {
			continue
		}
		seen := make(map[string]struct{})
		var extensions []string
		for _, extension := range strings.Split(value, ",") {
			extension = strings.TrimPrefix(strings.TrimSpace(extension), ".")
			if extension == "" {
				continue
			}
			if _, ok := seen[extension]; ok {
				continue
			}
			seen[extension] = struct{}{}
			extensions = append(extensions, extension)
		}
		return extensions
	}
	return nil
}

func withoutExtensionsOption(extra []string) []string {
	result := make([]string, 0, len(extra))
	for i := 0; i < len(extra); i++ {
		arg := extra[i]
		if arg == "-x" || arg == "--extensions" {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-x=") || strings.HasPrefix(arg, "--extensions=") {
			continue
		}
		result = append(result, arg)
	}
	return result
}

func filterExtensionWords(words []string, extension string) []string {
	suffix := "." + extension
	filtered := make([]string, 0)
	for _, word := range words {
		if strings.HasSuffix(word, suffix) {
			filtered = append(filtered, word)
		}
	}
	return filtered
}

func removeString(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func hasWord(seen map[string]struct{}, word string) bool {
	_, ok := seen[word]
	return ok
}

func writeTemporaryWordlist(words []string) (string, error) {
	file, err := os.CreateTemp("", "xgobuster-wordlist-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	for _, word := range words {
		if _, err := fmt.Fprintln(file, word); err != nil {
			file.Close()
			os.Remove(path)
			return "", err
		}
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func appendSearchedWords(path string, words []string) error {
	if len(words) == 0 {
		return nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
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

func (app *App) requireCommands(commands ...string) error {
	for _, command := range commands {
		if _, err := app.runner.LookPath(command); err != nil {
			return app.errorf("%s is required", command)
		}
	}
	return nil
}

func (app *App) prompt() (*PromptData, error) {
	result, err := ctxapi.Call[PromptData](ctxapi.NewV1(ctxRunner{runner: app.runner}), "prompt")
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (app *App) services() ([]Service, error) {
	result, err := ctxapi.Call[ServiceData](ctxapi.NewV1(ctxRunner{runner: app.runner}), "service", "ls")
	if err != nil {
		return nil, err
	}
	return result.Data.Services, nil
}

type ctxRunner struct {
	runner commandRunner
}

func (runner ctxRunner) Run(name string, args []string, _ []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return runner.runner.Run(name, args, stdin, stdout, stderr)
}

func (app *App) errorf(format string, args ...any) error { return fmt.Errorf(format, args...) }

type PromptData struct {
	Active        bool    `json:"active"`
	TargetIP      *string `json:"target_ip"`
	WorkspacePath string  `json:"workspace_path"`
}

type ServiceData struct {
	Services []Service `json:"services"`
}

type Service struct {
	Port        int     `json:"port"`
	Protocol    string  `json:"protocol"`
	ServiceName *string `json:"service_name"`
}

type parsedOptions struct {
	DNS              bool
	Domain           string
	Wordlist         string
	URL              string
	Host             string
	IP               bool
	Service          int
	Cookie           string
	ExcludeStatus    string
	ExcludeLength    string
	Insecure         bool
	VerifyTLS        bool
	Preset           string
	PresetExtensions string
	Mode             string
	Extra            []string
	Status           bool
	ClearCache       bool
}

func parseOptions(args []string) (parsedOptions, error) {
	var options parsedOptions
	start := 0
	if len(args) > 0 && args[0] == "dns" {
		options.DNS = true
		start = 1
	}
	for i := start; i < len(args); i++ {
		switch args[i] {
		case "-d", "--domain":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster dns --domain <domain> [gobuster-options]")
			}
			options.Domain = args[i+1]
			i++
		case "--status":
			options.Status = true
		case "--clear-cache":
			options.ClearCache = true
		case "--next", "--force", "--profile":
			return parsedOptions{}, fmt.Errorf("%s was removed; rerun the same command to continue or use --clear-cache to restart", args[i])
		case "--preset":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
			}
			options.Preset = args[i+1]
			i++
		case "-w", "--wordlist":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
			}
			options.Wordlist = args[i+1]
			i++
		case "--wordlist=":
			return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
		case "-u", "--url":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
			}
			options.URL = args[i+1]
			i++
		case "--host":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
			}
			options.Host = args[i+1]
			i++
		case "--ip":
			options.IP = true
		case "--service":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
			}
			service, convErr := strconv.Atoi(args[i+1])
			if convErr != nil || service < 1 {
				return parsedOptions{}, errors.New("invalid service number")
			}
			options.Service = service
			i++
		case "-c", "--cookies":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
			}
			options.Cookie = args[i+1]
			i++
		case "--exclude-length":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
			}
			options.ExcludeLength = args[i+1]
			i++
		case "--exclude-status":
			if i+1 >= len(args) || args[i+1] == "" {
				return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
			}
			options.ExcludeStatus = args[i+1]
			i++
		case "-k", "--no-tls-validation":
			options.Insecure = true
		case "--tls-verify":
			options.VerifyTLS = true
		default:
			if strings.HasPrefix(args[i], "--profile=") {
				return parsedOptions{}, errors.New("--profile was removed; ctx wordlist recommendation order is always used")
			}
			if strings.HasPrefix(args[i], "--domain=") {
				value := args[i]
				value = strings.TrimPrefix(value, "--domain=")
				if value == "" {
					return parsedOptions{}, errors.New("usage: xgobuster dns --domain <domain> [gobuster-options]")
				}
				options.Domain = value
				continue
			}
			if strings.HasPrefix(args[i], "--wordlist=") {
				options.Wordlist = strings.TrimPrefix(args[i], "--wordlist=")
				if options.Wordlist == "" {
					return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
				}
				continue
			}
			if strings.HasPrefix(args[i], "--url=") {
				options.URL = strings.TrimPrefix(args[i], "--url=")
				if options.URL == "" {
					return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
				}
				continue
			}
			if strings.HasPrefix(args[i], "--host=") {
				options.Host = strings.TrimPrefix(args[i], "--host=")
				if options.Host == "" {
					return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
				}
				continue
			}
			if strings.HasPrefix(args[i], "--service=") {
				value := strings.TrimPrefix(args[i], "--service=")
				service, convErr := strconv.Atoi(value)
				if convErr != nil || service < 1 {
					return parsedOptions{}, errors.New("invalid service number")
				}
				options.Service = service
				continue
			}
			if strings.HasPrefix(args[i], "--cookies=") {
				options.Cookie = strings.TrimPrefix(args[i], "--cookies=")
				if options.Cookie == "" {
					return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
				}
				continue
			}
			if strings.HasPrefix(args[i], "--exclude-length=") {
				options.ExcludeLength = strings.TrimPrefix(args[i], "--exclude-length=")
				if options.ExcludeLength == "" {
					return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
				}
				continue
			}
			if strings.HasPrefix(args[i], "--exclude-status=") {
				options.ExcludeStatus = strings.TrimPrefix(args[i], "--exclude-status=")
				if options.ExcludeStatus == "" {
					return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
				}
				continue
			}
			if strings.HasPrefix(args[i], "--preset=") {
				options.Preset = strings.TrimPrefix(args[i], "--preset=")
				if options.Preset == "" {
					return parsedOptions{}, errors.New("usage: xgobuster [gobuster-options]")
				}
				continue
			}
			options.Extra = append(options.Extra, args[i])
		}
	}
	return options, nil
}

var discoveryPattern = regexp.MustCompile(`^(\S+)\s+\(Status: ([0-9]{3})\)(?:\s+\[Size: ([0-9]+)\])?`)

func parseDiscoveries(output, baseURL, wordlist string, logID int64) []ctx.WebDiscovery {
	var discoveries []ctx.WebDiscovery
	for _, line := range strings.Split(output, "\n") {
		matches := discoveryPattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) == 0 {
			continue
		}
		status, _ := strconv.Atoi(matches[2])
		size, sizeErr := strconv.ParseInt(matches[3], 10, 64)
		path := matches[1]
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		storedPath := path
		if parsedBase, parseErr := url.Parse(baseURL); parseErr == nil {
			basePath := strings.TrimRight(parsedBase.Path, "/")
			if basePath != "" && basePath != "/" {
				storedPath = basePath + path
			}
		}
		url := strings.TrimRight(baseURL, "/") + path
		discoveries = append(discoveries, ctx.WebDiscovery{
			URL:                url,
			Path:               storedPath,
			StatusCode:         status,
			ContentLength:      size,
			ContentLengthValid: sizeErr == nil,
			SourceTool:         "gobuster",
			Wordlist:           wordlist,
			CommandLogID:       logID,
			CommandLogIDValid:  logID > 0,
		})
	}
	return discoveries
}

func formatCommand(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\n") {
			parts = append(parts, strconv.Quote(arg))
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func commandExitCode(err error) int {
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}
