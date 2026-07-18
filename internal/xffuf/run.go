package xffuf

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"req/internal/ctx"
)

var (
	Version = "1.0.0"
)

const usageText = `usage: xffuf vhost [ffuf-options]

Enumerate HTTP virtual hosts against the current ctx target.

The domain and HTTP service are selected from ctx when they are not provided.
The DNS wordlists under /usr/share/seclists are selected automatically.
Automatic calibration is enabled unless a manual filter is provided.

options:
  vhost                         enumerate HTTP virtual hosts
  -d, --domain <domain>         target domain
  -w, --wordlist <path>         use an explicit wordlist
  -u, --url <url>               override the selected HTTP service
  --host <hostname>             use a registered xhost hostname
  --ip                          use the target IP as the HTTP host
  --service <number>            select a web service by its displayed number
  -c, --cookies <value>         send cookies with requests
  -k, --no-tls-validation       disable TLS certificate validation
  --tls-verify                  verify TLS certificates for this run
  --suggest                     show calibration and optionally run a trial
  --trial                       scan without logs, cache, or host registration
  --no-auto-filter              disable automatic calibration
  --status                      show vhost wordlist status
  --clear-cache                 clear vhost wordlist cache
  --next                        continue with the next wordlist
  --force                       rerun the first available wordlist
  -h, --help                    show this help
  -V, --version                 show version

Common ffuf options, including -H, -fw, -fs, -fl, -fc, and -fr, are passed through.`

type ExitCodeError struct{ Code int }

func (err ExitCodeError) Error() string { return fmt.Sprintf("exit code %d", err.Code) }

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

type options struct {
	Domain       string
	Wordlist     string
	URL          string
	Host         string
	UseIP        bool
	Service      int
	Cookies      string
	Insecure     bool
	VerifyTLS    bool
	Suggest      bool
	Trial        bool
	NoAutoFilter bool
	Status       bool
	ClearCache   bool
	Next         bool
	Force        bool
	Extra        []string
}

type service struct {
	Port        int
	Protocol    string
	ServiceName string
}

type prompt struct {
	Active        bool
	TargetIP      string
	WorkspacePath string
}

type ffufResult struct {
	Input  map[string]string `json:"input"`
	Status int               `json:"status"`
	Length int               `json:"length"`
	Words  int               `json:"words"`
	Lines  int               `json:"lines"`
	URL    string            `json:"url"`
	Host   string            `json:"host"`
}

type ffufOutput struct {
	Results []ffufResult `json:"results"`
}

type metricStats struct {
	Min     int
	Max     int
	Average float64
	Median  int
	Mode    int
	Stable  int
	Total   int
}

type calibration struct {
	Status     metricStats
	Words      metricStats
	Lines      metricStats
	Size       metricStats
	Filter     []string
	Confidence int
}

func RunWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return New(realRunner{}, stdin, stdout, stderr).Run(args)
}

func New(runner commandRunner, stdin io.Reader, stdout, stderr io.Writer) *App {
	return &App{runner: runner, stdin: stdin, stdout: stdout, stderr: stderr}
}

func (app *App) Run(args []string) error {
	if len(args) > 1 {
		switch args[len(args)-1] {
		case "-h", "--help":
			_, err := fmt.Fprintln(app.stdout, usageText)
			return err
		case "-V", "--version":
			_, err := fmt.Fprintf(app.stdout, "xffuf %s\n", Version)
			return err
		}
	}
	parsed, err := parseOptions(args[1:])
	if err != nil {
		return err
	}
	if !parsed.Status && !parsed.ClearCache {
		if _, err := app.runner.LookPath("ffuf"); err != nil {
			return errors.New("ffuf is required")
		}
	}
	data, err := ctx.LoadPromptData(".")
	if err != nil {
		return fmt.Errorf("failed to load ctx prompt: %w", err)
	}
	if !data.Active || data.WorkspacePath == "" || data.TargetIP == "" {
		return errors.New("no active ctx target")
	}
	workspace, err := ctx.InitWorkspace(data.WorkspacePath)
	if err != nil {
		return fmt.Errorf("failed to load workspace: %w", err)
	}
	target, err := ctx.GetPrimaryTarget(workspace)
	if err != nil {
		return fmt.Errorf("failed to load primary target: %w", err)
	}
	config, err := ctx.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if !config.TLSVerify && !parsed.VerifyTLS {
		parsed.Insecure = true
	}
	hosts, err := ctx.ListHosts(workspace)
	if err != nil {
		return fmt.Errorf("failed to load hosts: %w", err)
	}
	targetHost, err := resolveTargetHost(target.IP, hosts, parsed.Host, parsed.UseIP)
	if err != nil {
		return err
	}
	if parsed.URL != "" && (parsed.Host != "" || parsed.UseIP) {
		return errors.New("usage: --url cannot be combined with --host or --ip")
	}
	baseURL, err := app.resolveURL(workspace, target, targetHost, parsed)
	if err != nil {
		return err
	}
	domain, err := resolveDomain(target.IP, hosts, parsed.Domain, app.stdin, app.stdout)
	if err != nil {
		return err
	}
	parsed.Domain = domain

	var suggestedFilter []string
	if parsed.Suggest {
		if len(manualFilters(parsed.Extra)) > 0 {
			return errors.New("usage: --suggest cannot be combined with manual filter options")
		}
		result, err := app.calibrate(baseURL, domain, parsed, config.VHostCalibrationSamples, config.VHostCalibrationConfidence)
		if err != nil {
			return err
		}
		printCalibration(app.stdout, result)
		if len(result.Filter) == 0 {
			return nil
		}
		trial, err := confirmTrial(app.stdin, app.stdout, result.Filter)
		if err != nil {
			return err
		}
		if !trial {
			return nil
		}
		parsed.Suggest = false
		parsed.Trial = true
		suggestedFilter = result.Filter
	}
	statePath := ""
	searched := make(map[string]struct{})
	if !parsed.Trial {
		statePath, err = searchedWordsPath(workspace, target.ID, baseURL, domain)
		if err != nil {
			return fmt.Errorf("failed to prepare vhost cache: %w", err)
		}
		if parsed.ClearCache {
			if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("failed to clear vhost cache: %w", err)
			}
			_, _ = fmt.Fprintf(app.stdout, "Cleared vhost search cache for %s\n", domain)
			return nil
		}
	}
	candidates, err := discoverWordlists()
	if err != nil {
		return err
	}
	if !parsed.Trial {
		searched, err = loadWords(statePath)
		if err != nil {
			return fmt.Errorf("failed to load vhost wordlist cache: %w", err)
		}
	}
	if parsed.Status {
		return app.showStatus(baseURL, domain, candidates, searched)
	}
	selected, words, err := selectWordlist(candidates, searched, parsed.Wordlist, parsed.Next, parsed.Force, config.VHostMaxRequests)
	if err != nil {
		return err
	}

	filterArgs := manualFilters(parsed.Extra)
	if len(filterArgs) == 0 && len(suggestedFilter) > 0 {
		filterArgs = suggestedFilter
	}
	if len(filterArgs) == 0 && !parsed.NoAutoFilter {
		result, calibrationErr := app.calibrate(baseURL, domain, parsed, config.VHostCalibrationSamples, config.VHostCalibrationConfidence)
		if calibrationErr != nil {
			return calibrationErr
		}
		if len(result.Filter) == 0 {
			return errors.New("unable to determine a stable vhost filter; use --suggest or specify -fw, -fs, -fl, -fc, or -fr")
		}
		filterArgs = result.Filter
		_, _ = fmt.Fprintf(app.stdout, "Automatic filter: %s\n", formatArgs(filterArgs))
	}

	return app.runScan(workspace, target, baseURL, domain, selected, words, parsed, filterArgs, statePath, args[1:])
}

func confirmTrial(in io.Reader, out io.Writer, filter []string) (bool, error) {
	_, _ = fmt.Fprintf(out, "\nRun a trial with %s? [y/N]: ", formatArgs(filter))
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("failed to read trial selection: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	case "", "n", "no":
		return false, nil
	default:
		return false, errors.New("invalid trial selection: enter y or n")
	}
}

func parseOptions(args []string) (options, error) {
	var result options
	start := 0
	if len(args) > 0 && args[0] == "vhost" {
		start = 1
	}
	for i := start; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-d", "--domain":
			value, next, err := nextValue(args, i, "domain")
			if err != nil {
				return options{}, err
			}
			result.Domain, i = value, next
		case "-w", "--wordlist":
			value, next, err := nextValue(args, i, "wordlist")
			if err != nil {
				return options{}, err
			}
			result.Wordlist, i = value, next
		case "-u", "--url":
			value, next, err := nextValue(args, i, "url")
			if err != nil {
				return options{}, err
			}
			result.URL, i = value, next
		case "--host":
			value, next, err := nextValue(args, i, "host")
			if err != nil {
				return options{}, err
			}
			result.Host, i = value, next
		case "--ip":
			result.UseIP = true
		case "--service":
			value, next, err := nextValue(args, i, "service")
			if err != nil {
				return options{}, err
			}
			service, convErr := strconv.Atoi(value)
			if convErr != nil || service < 1 {
				return options{}, errors.New("invalid service number")
			}
			result.Service, i = service, next
		case "-c", "--cookies":
			value, next, err := nextValue(args, i, "cookies")
			if err != nil {
				return options{}, err
			}
			result.Cookies, i = value, next
		case "-k", "--no-tls-validation":
			result.Insecure = true
		case "--tls-verify":
			result.VerifyTLS = true
		case "--suggest":
			result.Suggest = true
		case "--trial":
			result.Trial = true
		case "--no-auto-filter":
			result.NoAutoFilter = true
		case "--status":
			result.Status = true
		case "--clear-cache":
			result.ClearCache = true
		case "--next":
			result.Next = true
		case "--force":
			result.Force = true
		default:
			if strings.HasPrefix(arg, "--domain=") {
				result.Domain = strings.TrimPrefix(arg, "--domain=")
				continue
			}
			if strings.HasPrefix(arg, "--wordlist=") {
				result.Wordlist = strings.TrimPrefix(arg, "--wordlist=")
				continue
			}
			if strings.HasPrefix(arg, "--url=") {
				result.URL = strings.TrimPrefix(arg, "--url=")
				continue
			}
			if strings.HasPrefix(arg, "--host=") {
				result.Host = strings.TrimPrefix(arg, "--host=")
				continue
			}
			if strings.HasPrefix(arg, "--service=") {
				service, convErr := strconv.Atoi(strings.TrimPrefix(arg, "--service="))
				if convErr != nil || service < 1 {
					return options{}, errors.New("invalid service number")
				}
				result.Service = service
				continue
			}
			result.Extra = append(result.Extra, arg)
		}
	}
	if result.Suggest && (result.Status || result.ClearCache || result.Next || result.Force) {
		return options{}, errors.New("usage: --suggest cannot be combined with --status, --clear-cache, --next, or --force")
	}
	if result.Trial && (result.Suggest || result.Status || result.ClearCache || result.Next || result.Force) {
		return options{}, errors.New("usage: --trial cannot be combined with --suggest, --status, --clear-cache, --next, or --force")
	}
	if result.ClearCache && (result.Status || result.Next || result.Force) {
		return options{}, errors.New("usage: --clear-cache cannot be combined with --status, --next, or --force")
	}
	if result.Next && result.Wordlist != "" {
		return options{}, errors.New("usage: --next cannot be combined with --wordlist")
	}
	if result.Status && result.Wordlist != "" {
		return options{}, errors.New("usage: --status cannot be combined with --wordlist")
	}
	return result, nil
}

func nextValue(args []string, index int, name string) (string, int, error) {
	if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
		return "", index, fmt.Errorf("usage: --%s <value>", name)
	}
	return args[index+1], index + 1, nil
}

func (app *App) resolveURL(workspace *ctx.Workspace, target *ctx.Target, targetHost string, options options) (string, error) {
	if options.URL != "" {
		if options.Service != 0 {
			return "", errors.New("usage: --service cannot be combined with --url")
		}
		return strings.TrimRight(options.URL, "/"), nil
	}
	services, err := ctx.ListServices(workspace, target)
	if err != nil {
		return "", fmt.Errorf("failed to load services: %w", err)
	}
	var candidates []service
	for _, item := range services {
		name := strings.ToLower(item.ServiceName)
		if strings.Contains(name, "http") || item.Port == 80 || item.Port == 443 {
			candidates = append(candidates, service{Port: item.Port, Protocol: item.Protocol, ServiceName: item.ServiceName})
		}
	}
	if len(candidates) == 0 {
		return "", errors.New("no HTTP service found; run xscan first or specify -u <url>")
	}
	if options.Service > 0 {
		if options.Service > len(candidates) {
			return "", fmt.Errorf("invalid web service selection: %d (choose 1-%d)", options.Service, len(candidates))
		}
		return serviceURL(targetHost, candidates[options.Service-1]), nil
	}
	if len(candidates) == 1 {
		return serviceURL(targetHost, candidates[0]), nil
	}
	_, _ = fmt.Fprintln(app.stdout, "Select a web service:")
	_, _ = fmt.Fprintln(app.stdout)
	for i, item := range candidates {
		_, _ = fmt.Fprintf(app.stdout, "  %d) %s\n", i+1, serviceURL(targetHost, item))
	}
	_, _ = fmt.Fprintf(app.stdout, "\nSelect [1-%d]: ", len(candidates))
	line, err := bufio.NewReader(app.stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return "", errors.New("cancelled")
	}
	index, convErr := strconv.Atoi(strings.TrimSpace(line))
	if convErr != nil || index < 1 || index > len(candidates) {
		return "", errors.New("invalid web service selection")
	}
	return serviceURL(targetHost, candidates[index-1]), nil
}

func resolveTargetHost(targetIP string, hosts []ctx.Host, requested string, forceIP bool) (string, error) {
	if forceIP {
		return targetIP, nil
	}
	if requested == "" {
		return targetIP, nil
	}
	want := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(requested)), ".")
	for _, host := range hosts {
		if host.TargetIP == targetIP && strings.TrimSuffix(strings.ToLower(host.Hostname), ".") == want {
			return host.Hostname, nil
		}
	}
	return "", fmt.Errorf("xhost hostname not found for target %s: %s", targetIP, requested)
}

func serviceURL(targetIP string, item service) string {
	scheme := "http"
	if item.Port == 443 || strings.Contains(strings.ToLower(item.ServiceName), "https") {
		scheme = "https"
	}
	host := targetIP
	if (scheme == "http" && item.Port == 80) || (scheme == "https" && item.Port == 443) {
		return scheme + "://" + host
	}
	return scheme + "://" + host + ":" + strconv.Itoa(item.Port)
}

func resolveDomain(targetIP string, hosts []ctx.Host, requested string, stdin io.Reader, stdout io.Writer) (string, error) {
	if requested != "" {
		return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(requested)), "."), nil
	}
	var registered []string
	seen := make(map[string]struct{})
	for _, host := range hosts {
		if host.TargetIP != targetIP || strings.TrimSpace(host.Hostname) == "" {
			continue
		}
		name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host.Hostname)), ".")
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		registered = append(registered, name)
	}
	if len(registered) == 0 {
		return "", errors.New("vhost domain is not set; use --domain or register a hostname with xhost")
	}
	if len(registered) == 1 {
		return registered[0], nil
	}
	_, _ = fmt.Fprintln(stdout, "Select a vhost domain:")
	_, _ = fmt.Fprintln(stdout)
	for i, name := range registered {
		_, _ = fmt.Fprintf(stdout, "  %d) %s\n", i+1, name)
	}
	_, _ = fmt.Fprintf(stdout, "\nSelect [1-%d]: ", len(registered))
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return "", errors.New("cancelled")
	}
	index, convErr := strconv.Atoi(strings.TrimSpace(line))
	if convErr != nil || index < 1 || index > len(registered) {
		return "", errors.New("invalid vhost domain selection")
	}
	return registered[index-1], nil
}

func discoverWordlists() ([]ctx.WordlistSelection, error) {
	root := ctx.DiscoverWordlistsRoot()
	if root == "" {
		return nil, errors.New("wordlists directory not found; install wordlists or seclists")
	}
	roots := []string{filepath.Join(root, "seclists", "Discovery", "DNS"), "/usr/share/seclists/Discovery/DNS"}
	seen := make(map[string]struct{})
	type candidate struct {
		selection ctx.WordlistSelection
		rank      int
		size      int64
		key       string
	}
	var candidates []candidate
	for _, base := range roots {
		realBase, err := filepath.EvalSymlinks(base)
		if err != nil {
			continue
		}
		err = filepath.Walk(realBase, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info == nil || !info.Mode().IsRegular() || filepath.Ext(path) != ".txt" {
				return nil
			}
			realPath, evalErr := filepath.EvalSymlinks(path)
			if evalErr == nil {
				if _, ok := seen[realPath]; ok {
					return nil
				}
				seen[realPath] = struct{}{}
			}
			relative, _ := filepath.Rel(root, path)
			name := strings.ToLower(filepath.Base(path))
			rank := 2
			if strings.Contains(name, "top1million-5000") || strings.Contains(name, "top5000") {
				rank = 0
			} else if strings.Contains(name, "top1million") || strings.Contains(name, "top50000") {
				rank = 1
			}
			candidates = append(candidates, candidate{selection: ctx.WordlistSelection{Provider: "seclists", Profile: "vhost", Type: "vhost", Path: path}, rank: rank, size: info.Size(), key: relative})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan vhost wordlists: %w", err)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		if candidates[i].size != candidates[j].size {
			return candidates[i].size < candidates[j].size
		}
		return candidates[i].key < candidates[j].key
	})
	result := make([]ctx.WordlistSelection, 0, len(candidates))
	for _, item := range candidates {
		result = append(result, item.selection)
	}
	if len(result) == 0 {
		return nil, errors.New("no DNS wordlist found; install seclists or use --wordlist")
	}
	return result, nil
}

func selectWordlist(candidates []ctx.WordlistSelection, searched map[string]struct{}, explicit string, next, force bool, limit int) (ctx.WordlistSelection, []string, error) {
	if explicit != "" {
		words, err := loadWordlist(explicit, nil, false)
		return ctx.WordlistSelection{Provider: "manual", Profile: "manual", Type: "vhost", Path: explicit}, capWords(words, limit), err
	}
	skip := next
	for index, candidate := range candidates {
		words, err := loadWordlist(candidate.Path, searched, force && index == 0)
		if err != nil {
			return ctx.WordlistSelection{}, nil, fmt.Errorf("failed to prepare vhost wordlist %s: %w", candidate.Path, err)
		}
		if len(words) == 0 {
			continue
		}
		if skip {
			skip = false
			continue
		}
		return candidate, capWords(words, limit), nil
	}
	return ctx.WordlistSelection{}, nil, errors.New("all vhost wordlists have completed; use --force to rerun")
}

func capWords(words []string, limit int) []string {
	if limit > 0 && len(words) > limit {
		return words[:limit]
	}
	return words
}

func loadWordlist(path string, searched map[string]struct{}, ignoreSeen bool) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	result := make([]string, 0)
	local := make(map[string]struct{})
	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word == "" || (!ignoreSeen && searched != nil && hasWord(searched, word)) {
			continue
		}
		if _, ok := local[word]; ok {
			continue
		}
		local[word] = struct{}{}
		result = append(result, word)
	}
	return result, scanner.Err()
}

func hasWord(words map[string]struct{}, word string) bool { _, ok := words[word]; return ok }

func searchedWordsPath(workspace *ctx.Workspace, targetID int64, baseURL, domain string) (string, error) {
	digest := sha256.Sum256([]byte(baseURL + "\x00" + domain))
	directory := filepath.Join(workspace.DataPath, "xffuf-vhost", strconv.FormatInt(targetID, 10), hex.EncodeToString(digest[:]))
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	return filepath.Join(directory, "searched.words"), nil
}

func loadWords(path string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if word := strings.TrimSpace(scanner.Text()); word != "" {
			result[word] = struct{}{}
		}
	}
	return result, scanner.Err()
}

func appendWords(path string, words []string) error {
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

func (app *App) calibrate(baseURL, domain string, options options, samples, confidence int) (calibration, error) {
	if samples < 1 {
		samples = ctx.DefaultVHostCalibrationSamples
	}
	if confidence < 50 {
		confidence = ctx.DefaultVHostCalibrationConfidence
	}
	words := calibrationWords(samples)
	wordlist, err := writeTemporaryWordlist(words)
	if err != nil {
		return calibration{}, fmt.Errorf("failed to create calibration wordlist: %w", err)
	}
	defer os.Remove(wordlist)
	output, err := os.CreateTemp("", "xffuf-calibration-*.json")
	if err != nil {
		return calibration{}, err
	}
	outputPath := output.Name()
	_ = output.Close()
	defer os.Remove(outputPath)
	extra := append([]string(nil), options.Extra...)
	args := []string{"-c", "-noninteractive", "-mc", "all", "-w", wordlist, "-u", baseURL, "-H", "Host: FUZZ." + domain, "-o", outputPath, "-of", "json"}
	args = append(args, effectiveArgs(options, extra)...)
	var stdout, stderr bytes.Buffer
	if err := app.runner.Run("ffuf", args, app.stdin, &stdout, &stderr); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return calibration{}, fmt.Errorf("vhost calibration failed: %s", message)
	}
	results, err := readResults(outputPath)
	if err != nil {
		return calibration{}, fmt.Errorf("failed to read calibration results: %w", err)
	}
	if len(results) != samples {
		return calibration{}, fmt.Errorf("vhost calibration returned %d/%d responses", len(results), samples)
	}
	result := calibration{Status: stats(values(results, func(item ffufResult) int { return item.Status })), Words: stats(values(results, func(item ffufResult) int { return item.Words })), Lines: stats(values(results, func(item ffufResult) int { return item.Lines })), Size: stats(values(results, func(item ffufResult) int { return item.Length })), Confidence: confidence}
	result.Filter = filterProposal(result, confidence)
	return result, nil
}

func calibrationWords(samples int) []string {
	if samples < 1 {
		return nil
	}
	const minLength = 6
	const maxLength = 48
	seed := strconv.FormatInt(time.Now().UnixNano(), 10)
	words := make([]string, samples)
	for index := range words {
		length := minLength
		if samples > 1 {
			length += index * (maxLength - minLength) / (samples - 1)
		}
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", seed, index)))
		encoded := hex.EncodeToString(digest[:])
		words[index] = "x" + encoded[:length-1]
	}
	return words
}

func effectiveArgs(options options, extra []string) []string {
	result := append([]string(nil), extra...)
	if options.Cookies != "" {
		result = append(result, "-b", options.Cookies)
	}
	if options.Insecure {
		result = append(result, "-k")
	}
	return result
}

func values(results []ffufResult, pick func(ffufResult) int) []int {
	result := make([]int, len(results))
	for i, item := range results {
		result[i] = pick(item)
	}
	return result
}

func stats(values []int) metricStats {
	result := metricStats{Total: len(values)}
	if len(values) == 0 {
		return result
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	result.Min, result.Max = sorted[0], sorted[len(sorted)-1]
	sum, modeCount := 0, 0
	counts := make(map[int]int)
	for _, value := range sorted {
		sum += value
		counts[value]++
		if counts[value] > modeCount || counts[value] == modeCount && value < result.Mode {
			result.Mode, modeCount = value, counts[value]
		}
	}
	result.Average = float64(sum) / float64(len(sorted))
	if len(sorted)%2 == 0 {
		result.Median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	} else {
		result.Median = sorted[len(sorted)/2]
	}
	result.Stable = modeCount
	return result
}

func filterProposal(result calibration, confidence int) []string {
	type candidate struct {
		flag  string
		value metricStats
	}
	candidates := []candidate{{"-fw", result.Words}, {"-fl", result.Lines}, {"-fs", result.Size}}
	for _, item := range candidates {
		if item.value.Total > 0 && item.value.Stable*100/item.value.Total >= confidence {
			return []string{item.flag, strconv.Itoa(item.value.Mode)}
		}
	}
	if result.Status.Total > 0 && result.Status.Stable*100/result.Status.Total >= confidence && result.Status.Mode != 200 && result.Status.Mode != 204 && result.Status.Mode != 301 && result.Status.Mode != 302 {
		return []string{"-fc", strconv.Itoa(result.Status.Mode)}
	}
	return nil
}

func printCalibration(out io.Writer, result calibration) {
	_, _ = fmt.Fprintf(out, "Calibration samples: %d\n\n", result.Words.Total)
	printMetric(out, "status", result.Status)
	printMetric(out, "words", result.Words)
	printMetric(out, "lines", result.Lines)
	printMetric(out, "size", result.Size)
	if len(result.Filter) == 0 {
		_, _ = fmt.Fprintln(out, "\nNo stable filter found.")
	} else {
		_, _ = fmt.Fprintf(out, "\nSuggested filter: %s\n", formatArgs(result.Filter))
	}
	confidence := 0
	for _, item := range []metricStats{result.Status, result.Words, result.Lines, result.Size} {
		if item.Total > 0 && item.Stable*100/item.Total > confidence {
			confidence = item.Stable * 100 / item.Total
		}
	}
	_, _ = fmt.Fprintf(out, "Confidence: %d%%\n", confidence)
}

func printMetric(out io.Writer, name string, value metricStats) {
	_, _ = fmt.Fprintf(out, "%-6s min=%-6d max=%-6d average=%-8.1f median=%-6d mode=%-6d stable=%d/%d\n", name, value.Min, value.Max, value.Average, value.Median, value.Mode, value.Stable, value.Total)
}

func (app *App) runScan(workspace *ctx.Workspace, target *ctx.Target, baseURL, domain string, selection ctx.WordlistSelection, words []string, options options, filterArgs []string, statePath string, original []string) error {
	wordlist, err := writeTemporaryWordlist(words)
	if err != nil {
		return fmt.Errorf("failed to create vhost wordlist: %w", err)
	}
	defer os.Remove(wordlist)
	resultFile, err := os.CreateTemp("", "xffuf-results-*.json")
	if err != nil {
		return err
	}
	resultPath := resultFile.Name()
	_ = resultFile.Close()
	defer os.Remove(resultPath)
	args := []string{"-c", "-noninteractive", "-w", wordlist, "-u", baseURL, "-H", "Host: FUZZ." + domain}
	args = append(args, effectiveArgs(options, options.Extra)...)
	if len(manualFilters(options.Extra)) == 0 {
		args = append(args, filterArgs...)
	}
	args = append(args, "-o", resultPath, "-of", "json")
	logArgs := []string{"vhost", "-w", selection.Path, "-u", baseURL, "-H", "Host: FUZZ." + domain}
	logArgs = append(logArgs, effectiveArgs(options, options.Extra)...)
	if len(manualFilters(options.Extra)) == 0 {
		logArgs = append(logArgs, filterArgs...)
	}
	signatureArgs := effectiveArgs(options, options.Extra)
	if len(manualFilters(options.Extra)) == 0 {
		signatureArgs = append(signatureArgs, filterArgs...)
	}
	commandArgs := append([]string(nil), original...)
	if len(commandArgs) == 0 || commandArgs[0] != "vhost" {
		commandArgs = append([]string{"vhost"}, commandArgs...)
	}
	started := time.Now().UTC()
	logID := int64(0)
	if !options.Trial {
		logID, err = ctx.StartCommandLog(workspace, ctx.CommandLog{Command: formatCommand("xffuf", commandArgs), ExpandedCommand: formatCommand("ffuf", logArgs), StartedAt: started.Format(time.RFC3339Nano)})
		if err != nil {
			return fmt.Errorf("failed to start command log: %w", err)
		}
	}
	runID := int64(0)
	if !options.Trial && selection.Provider != "manual" {
		runID, err = ctx.StartWebWordlistRun(workspace, target, baseURL+" (vhost "+domain+")", selection.Provider, "vhost", strings.Join(signatureArgs, "\x00"), selection.Path, started.Format(time.RFC3339Nano), logID)
		if err != nil {
			return fmt.Errorf("failed to start vhost wordlist run: %w", err)
		}
	}
	if options.Trial {
		_, _ = fmt.Fprintf(app.stdout, "Trial ffuf vhost against %s with %s...\n", baseURL, selection.Path)
	} else {
		_, _ = fmt.Fprintf(app.stdout, "Running ffuf vhost against %s with %s...\n", baseURL, selection.Path)
	}
	var commandStdout, commandStderr bytes.Buffer
	runErr := app.runner.Run("ffuf", args, app.stdin, io.MultiWriter(app.stdout, &commandStdout), io.MultiWriter(app.stderr, &commandStderr))
	status, exitCode := "success", 0
	if runErr != nil {
		status, exitCode = "failed", commandExitCode(runErr)
	}
	results, parseErr := readResults(resultPath)
	if parseErr != nil && runErr == nil {
		status, exitCode = "failed", 1
		runErr = fmt.Errorf("failed to read ffuf results: %w", parseErr)
	}
	suspicious := len(results) > 100 && len(results)*100 > len(words)
	if suspicious && runErr == nil {
		status, exitCode = "failed", 1
		runErr = fmt.Errorf("too many vhost hits (%d/%d); no hosts or cache entries were saved", len(results), len(words))
		_, _ = fmt.Fprintln(&commandStderr, runErr)
	}
	ended := time.Now().UTC()
	if !options.Trial {
		if err := ctx.FinishCommandLog(workspace, logID, ctx.CommandLog{Status: status, ExitCode: exitCode, Stdout: commandStdout.String(), Stderr: commandStderr.String(), EndedAt: ended.Format(time.RFC3339Nano)}); err != nil {
			return err
		}
	}
	if runID > 0 {
		if err := ctx.FinishWebWordlistRun(workspace, runID, status, ended.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if runErr != nil {
		return runErr
	}
	if options.Trial {
		_, _ = fmt.Fprintln(app.stdout, "Trial completed; no logs, cache entries, or hosts were saved.")
		return nil
	}
	if selection.Provider != "manual" {
		if err := appendWords(statePath, words); err != nil {
			return fmt.Errorf("failed to save vhost wordlist cache: %w", err)
		}
	}
	for _, item := range results {
		hostname := resultHostname(item, domain)
		if hostname != "" {
			if _, err := ctx.AddHost(workspace, hostname, target.Name); err != nil {
				return fmt.Errorf("failed to save vhost %s: %w", hostname, err)
			}
		}
	}
	return nil
}

func resultHostname(item ffufResult, domain string) string {
	value := item.Input["FUZZ"]
	if value == "" {
		value = item.Host
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, ".") {
		value += "." + domain
	}
	return strings.TrimSuffix(strings.ToLower(value), ".")
}

func readResults(path string) ([]ffufResult, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var output ffufOutput
	if err := json.Unmarshal(content, &output); err != nil {
		return nil, err
	}
	return output.Results, nil
}

func writeTemporaryWordlist(words []string) (string, error) {
	file, err := os.CreateTemp("", "xffuf-wordlist-*")
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

func commandExitCode(err error) int {
	var exitErr ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}

func formatCommand(name string, args []string) string {
	parts := append([]string{name}, args...)
	for i, part := range parts[1:] {
		if strings.ContainsAny(part, " \t\n") {
			parts[i+1] = strconv.Quote(part)
		}
	}
	return strings.Join(parts, " ")
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return formatCommand("", args)[1:]
}

func originalArgs(options options) []string {
	result := []string{}
	if options.Domain != "" {
		result = append(result, "--domain", options.Domain)
	}
	if options.Wordlist != "" {
		result = append(result, "--wordlist", options.Wordlist)
	}
	if options.URL != "" {
		result = append(result, "--url", options.URL)
	}
	result = append(result, options.Extra...)
	return result
}

func manualFilters(extra []string) []string {
	flags := map[string]bool{"-fw": true, "-fs": true, "-fl": true, "-fc": true, "-fr": true, "--filter-words": true, "--filter-size": true, "--filter-lines": true, "--filter-status": true, "--filter-regex": true}
	result := []string{}
	for i, arg := range extra {
		if flags[arg] {
			result = append(result, arg)
			if i+1 < len(extra) {
				result = append(result, extra[i+1])
			}
		}
	}
	return result
}

func (app *App) showStatus(baseURL, domain string, candidates []ctx.WordlistSelection, searched map[string]struct{}) error {
	_, _ = fmt.Fprintf(app.stdout, "Vhost wordlist status for %s (%s)\n", baseURL, domain)
	total, covered := 0, 0
	for _, item := range candidates {
		words, err := loadWordlist(item.Path, searched, false)
		if err != nil {
			return err
		}
		count, err := countWordlist(item.Path)
		if err != nil {
			return err
		}
		done := count - len(words)
		total += count
		covered += done
		status := "pending"
		if len(words) == 0 {
			status = "completed"
		}
		_, _ = fmt.Fprintf(app.stdout, "[%s] %6d words  %s\n", status, count, item.Path)
	}
	_, _ = fmt.Fprintf(app.stdout, "Entries: %d total, %d covered, %d remaining\n", total, covered, total-covered)
	_, _ = fmt.Fprintf(app.stdout, "Searched unique words: %d\n", len(searched))
	return nil
}

func countWordlist(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count, scanner.Err()
}
