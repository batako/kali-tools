package xffuf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"req/internal/ctx"
)

const (
	paramNameProfile  = "parameter-name"
	paramValueProfile = "parameter-value"
)

type paramTemplate struct {
	Type          string
	ParameterName string
	FixedValue    string
}

func (app *App) runParam(workspace *ctx.Workspace, target *ctx.Target, template string, options options, config *ctx.Config, original []string) error {
	parsed, err := parseParamTemplate(template)
	if err != nil {
		return err
	}
	if options.Suggest {
		return errors.New("xffuf param does not support --suggest; use ffuf filters or --no-auto-filter")
	}
	profile := inferParamProfile(parsed)
	statePath := filepath.Join(workspace.DataPath, "xffuf-param", fmt.Sprintf("%d", target.ID), cacheDigest(template+"\x00"+profile), "searched.words")
	if !options.Trial {
		if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
			return err
		}
	}
	if options.ClearCache {
		if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		_, _ = fmt.Fprintf(app.stdout, "Cleared parameter search cache for %s\n", template)
		return nil
	}
	var candidates []ctx.WordlistSelection
	if options.Wordlist == "" {
		candidates, err = discoverParamWordlists(parsed, profile)
		if err != nil {
			return err
		}
	}
	searched := make(map[string]struct{})
	if !options.Trial {
		searched, err = loadWords(statePath)
		if err != nil {
			return err
		}
	}
	if options.Status {
		return app.showParamStatus(template, profile, candidates, searched)
	}
	selected, words, err := selectWordlist(candidates, searched, options.Wordlist, config.VHostMaxRequests)
	if err != nil {
		return err
	}
	selected.Profile = profile
	selected.Type = parsed.Type
	return app.runParamScan(workspace, target, template, parsed, selected, words, options, statePath, original)
}

func parseParamTemplate(raw string) (paramTemplate, error) {
	if strings.Count(raw, "FUZZ") != 1 {
		return paramTemplate{}, errors.New("xffuf param URL must contain exactly one FUZZ marker")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return paramTemplate{}, errors.New("xffuf param requires an absolute URL")
	}
	for _, field := range strings.Split(parsed.RawQuery, "&") {
		parts := strings.SplitN(field, "=", 2)
		name := parts[0]
		value := ""
		if len(parts) == 2 {
			value = parts[1]
		}
		switch {
		case strings.Contains(name, "FUZZ"):
			return paramTemplate{Type: "param-name", FixedValue: value}, nil
		case strings.Contains(value, "FUZZ"):
			decoded, _ := url.QueryUnescape(name)
			return paramTemplate{Type: "param-value", ParameterName: decoded}, nil
		}
	}
	return paramTemplate{}, errors.New("xffuf param supports FUZZ in a query parameter name or value")
}

func inferParamProfile(template paramTemplate) string {
	if template.Type == "param-name" {
		return paramNameProfile
	}
	return paramValueProfile
}

func discoverParamWordlists(template paramTemplate, profile string) ([]ctx.WordlistSelection, error) {
	kind := ctx.WordlistKindParameterValue
	if template.Type == "param-name" {
		kind = ctx.WordlistKindParameterName
	}
	candidates, err := recommendWordlists(kind)
	if err != nil {
		return nil, fmt.Errorf("%w; use --wordlist to override", err)
	}
	for i := range candidates {
		candidates[i].Profile = profile
		candidates[i].Type = template.Type
	}
	return candidates, nil
}

func (app *App) runParamScan(workspace *ctx.Workspace, target *ctx.Target, template string, parsed paramTemplate, selection ctx.WordlistSelection, words []string, options options, statePath string, original []string) error {
	wordlist, err := writeTemporaryWordlist(words)
	if err != nil {
		return err
	}
	defer os.Remove(wordlist)
	resultFile, err := os.CreateTemp("", "xffuf-param-results-*.json")
	if err != nil {
		return err
	}
	resultPath := resultFile.Name()
	_ = resultFile.Close()
	defer os.Remove(resultPath)
	args := []string{"-c", "-noninteractive", "-w", wordlist, "-u", template}
	args = append(args, effectiveArgs(options, options.Extra)...)
	if useParamAutoFilter(options) {
		args = append(args, "-ac")
	}
	args = append(args, "-o", resultPath, "-of", "json")
	logArgs := append([]string{"-w", selection.Path, "-u", template}, effectiveArgs(options, options.Extra)...)
	if useParamAutoFilter(options) {
		logArgs = append(logArgs, "-ac")
	}
	commandArgs := append([]string(nil), original...)
	if len(commandArgs) == 0 || commandArgs[0] != "param" {
		commandArgs = append([]string{"param"}, commandArgs...)
	}
	started := time.Now().UTC()
	logID := int64(0)
	if !options.Trial {
		logID, err = ctx.StartCommandLog(workspace, ctx.CommandLog{Command: formatCommand("xffuf", commandArgs), ExpandedCommand: formatCommand("ffuf", logArgs), StartedAt: started.Format(time.RFC3339Nano)})
		if err != nil {
			return err
		}
	}
	runID := int64(0)
	if !options.Trial && selection.Provider != "manual" {
		runID, err = ctx.StartWebWordlistRun(workspace, target, template, selection.Provider, selection.Profile, strings.Join(effectiveArgs(options, options.Extra), "\x00"), selection.Path, started.Format(time.RFC3339Nano), logID)
		if err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintf(app.stdout, "Running ffuf %s against %s with %s...\n", parsed.Type, template, selection.Path)
	var commandStdout, commandStderr bytes.Buffer
	runErr := app.runner.Run("ffuf", args, app.stdin, io.MultiWriter(app.stdout, &commandStdout), io.MultiWriter(app.stderr, &commandStderr))
	status, exitCode := "success", 0
	if runErr != nil {
		status, exitCode = "failed", commandExitCode(runErr)
	}
	results, parseErr := readResults(resultPath)
	if parseErr != nil && runErr == nil {
		runErr, status, exitCode = parseErr, "failed", 1
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
		_, _ = fmt.Fprintln(app.stdout, "Trial completed; no results were saved.")
		return nil
	}
	if selection.Provider != "manual" {
		if err := appendWords(statePath, words); err != nil {
			return err
		}
	}
	for _, item := range results {
		word := strings.TrimSpace(item.Input["FUZZ"])
		if word == "" {
			continue
		}
		resultURL := item.URL
		if resultURL == "" {
			resultURL = strings.ReplaceAll(template, "FUZZ", word)
		}
		name, value := paramResult(parsed, word)
		path := "/"
		if parsedURL, parseErr := url.Parse(resultURL); parseErr == nil && parsedURL.EscapedPath() != "" {
			path = parsedURL.EscapedPath()
		}
		discovery := ctx.WebDiscovery{URL: resultURL, Path: path, StatusCode: item.Status, ContentLength: int64(item.Length), ContentLengthValid: true, SourceTool: "xffuf", Wordlist: selection.Path, CommandLogID: logID, CommandLogIDValid: true, DiscoveryType: parsed.Type, TemplateURL: template, ParameterName: name, ParameterValue: value, FuzzPart: strings.TrimPrefix(parsed.Type, "param-"), WordCount: item.Words, WordCountValid: true, LineCount: item.Lines, LineCountValid: true}
		if item.RedirectLocation != "" {
			discovery.RedirectURL = item.RedirectLocation
			discovery.RedirectURLValid = true
		}
		_, err = ctx.SaveWebDiscovery(workspace, target, discovery)
		if err != nil {
			return err
		}
	}
	return nil
}

func useParamAutoFilter(options options) bool {
	return !options.NoAutoFilter && len(manualFilters(options.Extra)) == 0 && len(manualMatchers(options.Extra)) == 0
}

func paramResult(template paramTemplate, word string) (string, string) {
	if template.Type == "param-name" {
		value, _ := url.QueryUnescape(template.FixedValue)
		return word, value
	}
	return template.ParameterName, word
}

func (app *App) showParamStatus(template, profile string, candidates []ctx.WordlistSelection, searched map[string]struct{}) error {
	_, _ = fmt.Fprintf(app.stdout, "Parameter wordlist status for %s (%s)\n", template, profile)
	for _, candidate := range candidates {
		words, err := loadWordlist(candidate.Path, searched)
		if err != nil {
			return err
		}
		count, err := countWordlist(candidate.Path)
		if err != nil {
			return err
		}
		status := "pending"
		if len(words) == 0 {
			status = "completed"
		}
		_, _ = fmt.Fprintf(app.stdout, "[%s] %6d words  %s\n", status, count, candidate.Path)
	}
	return nil
}

func cacheDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
