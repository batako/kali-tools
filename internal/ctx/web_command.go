package ctx

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func runWeb(args []string, stdout io.Writer) error {
	originalArgs := args
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, webUsageText)
		return err
	}

	var err error
	var showHelp bool
	args, showHelp, err = resolveResourceCommand("web", args, []string{"ls", "show", "clear"}, "", "ls")
	if err != nil {
		return jsonArgumentError(stdout, "web", originalArgs, err)
	}
	if showHelp {
		_, err := fmt.Fprintln(stdout, webUsageText)
		return err
	}
	if args[0] != "ls" && args[0] != "show" && args[0] != "clear" {
		return jsonArgumentError(stdout, "web", originalArgs, fmt.Errorf("unknown ctx web command: %s", args[0]))
	}
	if args[0] == "clear" {
		return runWebClear(args[1:], stdout, originalArgs)
	}
	if args[0] == "show" {
		return runWebShow(args[1:], stdout)
	}

	targetName := ""
	typeName := ""
	remaining, output, err := parseOutputOptions(args[1:], apiFormatShell)
	if err != nil {
		return jsonArgumentError(stdout, "web", originalArgs, err)
	}
	if output.Format != apiFormatShell && output.Format != apiFormatJSON {
		return jsonArgumentError(stdout, "web", originalArgs, fmt.Errorf("unsupported web format: %s", output.Format))
	}
	if output.Format == apiFormatShell && output.FormatVersion != "" {
		return jsonArgumentError(stdout, "web", originalArgs, errors.New("--format-version can only be used with --format json"))
	}
	for i := 0; i < len(remaining); i++ {
		switch remaining[i] {
		case "--target":
			if i+1 >= len(remaining) || strings.TrimSpace(remaining[i+1]) == "" {
				return jsonArgumentError(stdout, "web", originalArgs, errors.New("usage: ctx web ls [--target <name>] [--format <shell|json>] [--format-version <version>]"))
			}
			i++
			targetName = remaining[i]
		case "--type":
			if i+1 >= len(remaining) {
				return errors.New("usage: ctx web ls --type <path|param|param-name|param-value>")
			}
			i++
			typeName = remaining[i]
			if !validWebDiscoveryTypeFilter(typeName) {
				return fmt.Errorf("invalid web discovery type: %s", typeName)
			}
		default:
			return jsonArgumentError(stdout, "web", originalArgs, errors.New("usage: ctx web ls [--target <name>] [--format <shell|json>] [--format-version <version>]"))
		}
	}

	if output.Format == apiFormatJSON {
		return runJSONEndpoint(stdout, "web", output.FormatVersion, func(version string) (any, error) {
			return webAPIData(version, targetName, typeName)
		})
	}

	workspace, target, err := webCommandContext(targetName)
	if err != nil {
		return err
	}
	discoveries, err := ListWebDiscoveries(workspace, target)
	if err != nil {
		return err
	}
	return WriteWebDiscoveryList(stdout, target, filterWebDiscoveries(discoveries, typeName))
}

func runWebShow(args []string, stdout io.Writer) error {
	targetName := ""
	if len(args) == 0 {
		return errors.New("usage: ctx web show <id> [--target <name>]")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id < 1 {
		return fmt.Errorf("invalid web discovery ID: %s", args[0])
	}
	for i := 1; i < len(args); i++ {
		if args[i] != "--target" || i+1 >= len(args) {
			return errors.New("usage: ctx web show <id> [--target <name>]")
		}
		i++
		targetName = args[i]
	}
	workspace, target, err := webCommandContext(targetName)
	if err != nil {
		return err
	}
	discovery, err := GetWebDiscovery(workspace, target, id)
	if err != nil {
		return err
	}
	return WriteWebDiscoveryDetail(stdout, target, discovery)
}

func validWebDiscoveryTypeFilter(value string) bool {
	switch value {
	case "", "path", "param", "param-name", "param-value":
		return true
	default:
		return false
	}
}

func filterWebDiscoveries(discoveries []WebDiscovery, typeName string) []WebDiscovery {
	if typeName == "" {
		return discoveries
	}
	result := make([]WebDiscovery, 0, len(discoveries))
	for _, discovery := range discoveries {
		actual := webDiscoveryType(discovery.DiscoveryType)
		if actual == typeName || typeName == "param" && strings.HasPrefix(actual, "param-") {
			result = append(result, discovery)
		}
	}
	return result
}

func runWebClear(args []string, stdout io.Writer, originalArgs []string) error {
	targetName := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return jsonArgumentError(stdout, "web", originalArgs, errors.New("usage: ctx web clear [--target <name>]"))
			}
			i++
			targetName = args[i]
		default:
			return jsonArgumentError(stdout, "web", originalArgs, errors.New("usage: ctx web clear [--target <name>]"))
		}
	}

	workspace, target, err := webCommandContext(targetName)
	if err != nil {
		return err
	}
	summary, err := InspectWebDiscoveryData(workspace, target)
	if err != nil {
		return err
	}
	ok, err := confirmWebDiscoveryClear(stdout, bufio.NewScanner(workspaceStdin), target, summary)
	if err != nil || !ok {
		return err
	}
	cleared, err := ClearWebDiscoveryData(workspace, target)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Cleared web discovery data for target: %s\nDiscoveries: %d\nWordlist runs: %d\nWordlist cache: %s\n",
		target.Name, cleared.Discoveries, cleared.WordlistRuns, webCacheClearResult(cleared.CachePresent))
	return err
}

func confirmWebDiscoveryClear(stdout io.Writer, scanner *bufio.Scanner, target *Target, summary WebDiscoveryDataSummary) (bool, error) {
	if _, err := fmt.Fprintf(stdout, "Clear all web discovery data for target %s (%s)?\n", target.Name, target.IP); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintf(stdout, "\nDiscoveries: %d\nWordlist runs: %d\nWordlist cache: %s\n\nThis resets xgobuster search progress and cannot be undone.\n[y/N]: ",
		summary.Discoveries, summary.WordlistRuns, webCacheStatus(summary.CachePresent)); err != nil {
		return false, err
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("failed to read web discovery clear confirmation: %w", err)
		}
		_, err := fmt.Fprintln(stdout, "\ncancelled")
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	if answer != "y" && answer != "yes" {
		_, err := fmt.Fprintln(stdout, "cancelled")
		return false, err
	}
	return true, nil
}

func webCacheStatus(present bool) string {
	if present {
		return "present"
	}
	return "absent"
}

func webCacheClearResult(present bool) string {
	if present {
		return "removed"
	}
	return "absent"
}

func webAPIData(version, targetName, typeName string) (any, error) {
	switch version {
	case "1.0":
		return webAPIDataV1_0(targetName, typeName)
	default:
		return nil, fmt.Errorf("unsupported web format version after resolution: %s", version)
	}
}

func webAPIDataV1_0(targetName, typeName string) (any, error) {
	workspace, target, err := webCommandContext(targetName)
	if err != nil {
		return nil, err
	}
	discoveries, err := ListWebDiscoveries(workspace, target)
	if err != nil {
		return nil, err
	}
	views := SummarizeWebDiscoveries(filterWebDiscoveries(discoveries, typeName))
	items := make([]map[string]any, 0, len(views))
	for _, view := range views {
		items = append(items, map[string]any{
			"id":             view.ID,
			"type":           view.DiscoveryType,
			"url":            view.URL,
			"origin":         view.Origin,
			"path":           view.Path,
			"status_code":    view.StatusCode,
			"content_length": nullableWebInt64(view.ContentLengthValid, view.ContentLength),
			"redirect_url":   nullableWebString(view.RedirectURLValid, view.RedirectURL),
			"sources":        view.Sources,
			"last_seen":      view.LastSeen,
			"template_url":   nullableWebText(view.TemplateURL),
			"parameter": map[string]any{
				"name": view.ParameterName, "value": view.ParameterValue, "fuzzed_part": view.FuzzPart,
			},
			"word_count": nullableWebInt(view.WordCountValid, view.WordCount),
			"line_count": nullableWebInt(view.LineCountValid, view.LineCount),
		})
	}
	return map[string]any{"discoveries": items}, nil
}

func webCommandContext(targetName string) (*Workspace, *Target, error) {
	workspace, err := currentWorkspace()
	if err != nil {
		return nil, nil, err
	}
	var target *Target
	if targetName == "" {
		target, err = GetPrimaryTarget(workspace)
	} else {
		target, err = GetTargetByName(workspace, targetName)
	}
	if err != nil {
		return nil, nil, err
	}
	return workspace, target, nil
}
