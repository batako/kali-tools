package ctx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	WordlistKindAll            = "all"
	WordlistKindDirectory      = "directory"
	WordlistKindSubdomain      = "subdomain"
	WordlistKindParameterName  = "parameter-name"
	WordlistKindParameterValue = "parameter-value"
	WordlistKindPassword       = "password"
	WordlistKindUsername       = "username"
	WordlistKindEndpoint       = "endpoint"
	WordlistKindUnknown        = "unknown"
)

var wordlistKinds = []string{WordlistKindAll, WordlistKindDirectory, WordlistKindSubdomain, WordlistKindParameterName, WordlistKindParameterValue, WordlistKindPassword, WordlistKindUsername, WordlistKindEndpoint, WordlistKindUnknown}

type WordlistEntry struct {
	Path         string              `json:"path"`
	ResolvedPath string              `json:"resolved_path,omitempty"`
	Provider     string              `json:"provider"`
	Kind         string              `json:"kind"` // primary kind kept for JSON 1.0 compatibility
	Kinds        []WordlistKindMatch `json:"kinds,omitempty"`
	Profile      string              `json:"profile,omitempty"`
	State        string              `json:"state"`
	Format       string              `json:"format,omitempty"`
	Readiness    string              `json:"readiness,omitempty"`
	Available    bool                `json:"available"`
	Usable       bool                `json:"usable"`
	Reason       string              `json:"reason,omitempty"`
	Size         int64               `json:"size,omitempty"`
	Compressed   bool                `json:"compressed,omitempty"`
	Lines        int64               `json:"lines,omitempty"`
}

type wordlistCatalog struct {
	Root      string          `json:"root"`
	Entries   []WordlistEntry `json:"entries"`
	Warnings  []string        `json:"warnings,omitempty"`
	QueryKind string          `json:"-"`
}

const wordlistUsageText = `usage: ctx wordlist <command> [options]

Inspect wordlists installed in the current environment and suggest lists by use.

commands:
  ls                 list every discovered file
  show <path>        show details for one list
  extract            verify and extract trusted wordlists

options:
  --kind <kind>      用途別の推薦、またはlsの絞り込み（directory, subdomain,
                     parameter-name, parameter-value, password, username）
  --usable-only      hide metadata, unreadable, and unsupported files
  --format <format>  shell, json, or markdown
  --format-version   JSON format version (1.0)
  -h, --help         show this help`

func runWordlist(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, wordlistUsageText)
		return err
	}
	original := append([]string(nil), args...)
	args, output, err := parseOutputOptions(args, apiFormatShell)
	if err != nil {
		return jsonArgumentError(stdout, "wordlist", original, err)
	}
	if output.Format != apiFormatShell && output.Format != apiFormatJSON && output.Format != "markdown" {
		return jsonArgumentError(stdout, "wordlist", original, fmt.Errorf("unsupported wordlist format: %s", output.Format))
	}
	if output.FormatVersion != "" && output.Format != apiFormatJSON {
		return errors.New("--format-version can only be used with --format json")
	}
	if len(args) > 0 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, wordlistUsageText)
		return err
	}
	action := "ls"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		action = args[0]
		args = args[1:]
	}
	if action != "ls" && action != "show" && action != "extract" {
		return jsonArgumentError(stdout, "wordlist", original, fmt.Errorf("unknown ctx wordlist command: %s", action))
	}
	if action == "extract" {
		return runWordlistExtract(args, output, workspaceStdin, stdout)
	}
	options, path, err := parseWordlistOptions(args)
	if err != nil {
		return jsonArgumentError(stdout, "wordlist", original, err)
	}
	root := DiscoverWordlistsRoot()
	if root == "" {
		return errors.New("wordlists directory not found; install the wordlists package")
	}
	catalog, err := buildWordlistCatalog(root)
	if err != nil {
		return err
	}
	if options.Kind != WordlistKindAll {
		catalog.Entries = recommendWordlists(catalog.Entries, options.Kind)
	} else if options.UsableOnly {
		catalog.Entries = filterWordlistEntries(catalog.Entries, WordlistKindAll, true)
	}
	if action == "show" {
		entry, ok := findWordlistEntry(catalog.Entries, path)
		if !ok {
			return fmt.Errorf("wordlist not found: %s", path)
		}
		catalog.Entries = []WordlistEntry{entry}
	}
	catalog.QueryKind = options.Kind
	return writeWordlistOutput(stdout, output, catalog, action)
}

type wordlistOptions struct {
	Kind       string
	UsableOnly bool
}

func parseWordlistOptions(args []string) (wordlistOptions, string, error) {
	o := wordlistOptions{Kind: WordlistKindAll}
	var path string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--kind":
			if i+1 >= len(args) {
				return o, path, fmt.Errorf("usage: %s requires a value", args[i])
			}
			i++
			v := args[i]
			switch args[i-1] {
			case "--kind":
				o.Kind = v
			}
		case "--usable-only":
			o.UsableOnly = true
		case "-h", "--help":
			return o, path, errors.New(wordlistUsageText)
		default:
			if strings.HasPrefix(args[i], "-") {
				return o, path, fmt.Errorf("unknown wordlist option: %s", args[i])
			}
			if path != "" {
				return o, path, errors.New("wordlist accepts only one path")
			}
			path = args[i]
		}
	}
	if !containsWordlistKind(o.Kind) {
		return o, path, errors.New("invalid wordlist kind")
	}
	return o, path, nil
}

func containsWordlistKind(v string) bool {
	for _, k := range wordlistKinds {
		if v == k {
			return true
		}
	}
	return false
}

func buildWordlistCatalog(root string) (wordlistCatalog, error) {
	c := wordlistCatalog{Root: root}
	manifest, err := wordlistManifestIndex()
	if err != nil {
		return c, err
	}
	seen := map[string]bool{}
	if err := scanWordlistNode(root, root, &c, seen, manifest); err != nil {
		return c, err
	}
	sort.Slice(c.Entries, func(i, j int) bool { return c.Entries[i].Path < c.Entries[j].Path })
	return c, nil
}

func scanWordlistNode(logical, real string, c *wordlistCatalog, seen map[string]bool, manifest map[string]wordlistManifestRecord) error {
	info, err := os.Lstat(logical)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, e := filepath.EvalSymlinks(logical)
		if e != nil {
			c.Entries = append(c.Entries, WordlistEntry{Path: logical, Provider: wordlistProvider(logical), Kind: WordlistKindUnknown, State: WordlistStateUnknown, Readiness: WordlistReadyUnsupported, Reason: "broken symlink"})
			return nil
		}
		target, e := os.Stat(resolved)
		if e != nil {
			c.Entries = append(c.Entries, WordlistEntry{Path: logical, Provider: wordlistProvider(logical), Kind: WordlistKindUnknown, State: WordlistStateUnknown, Readiness: WordlistReadyUnsupported, Reason: "unavailable target"})
			return nil
		}
		if target.IsDir() {
			items, e := os.ReadDir(resolved)
			if e != nil {
				c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %v", logical, e))
				return nil
			}
			for _, item := range items {
				if e := scanWordlistNode(filepath.Join(logical, item.Name()), filepath.Join(resolved, item.Name()), c, seen, manifest); e != nil {
					c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %v", filepath.Join(logical, item.Name()), e))
				}
			}
			return nil
		}
		return catalogWordlistFile(logical, resolved, target, c, seen, manifest)
	}
	if info.IsDir() {
		items, err := os.ReadDir(real)
		if err != nil {
			c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %v", logical, err))
			return nil
		}
		for _, item := range items {
			if err := scanWordlistNode(filepath.Join(logical, item.Name()), filepath.Join(real, item.Name()), c, seen, manifest); err != nil {
				c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %v", filepath.Join(logical, item.Name()), err))
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	return catalogWordlistFile(logical, real, info, c, seen, manifest)
}

func catalogWordlistFile(logical, real string, info os.FileInfo, c *wordlistCatalog, seen map[string]bool, manifest map[string]wordlistManifestRecord) error {
	resolved, _ := filepath.EvalSymlinks(real)
	key := resolved
	if key == "" {
		key = real
	}
	if seen[key] {
		return nil
	}
	seen[key] = true
	relative, relErr := filepath.Rel(c.Root, logical)
	if relErr != nil {
		return relErr
	}
	manifestRecord, knownPath := manifest[filepath.ToSlash(relative)]
	e := WordlistEntry{Path: logical, ResolvedPath: resolved, Provider: wordlistProvider(logical), Available: true, Size: info.Size(), State: WordlistStateUnknown}
	if knownPath {
		e.Provider = manifestRecord.Provider
		e.Format = manifestRecord.Format
		e.Readiness = manifestRecord.Readiness
		e.Lines = manifestRecord.Lines
		e.Kinds = append([]WordlistKindMatch(nil), manifestRecord.Kinds...)
		if info.Size() == manifestRecord.Size && hashMatches(real, manifestRecord.SHA256) {
			e.State = WordlistStateKnownVerified
		} else {
			e.State = WordlistStateKnownModified
			sample, _ := sampleWordlistFile(real, info.Size())
			e.Kinds = classifyWordlistKinds(relative, sample, 0, "inferred")
			e.Lines = 0
			e.Reason = "path exists in the embedded catalog but content differs"
		}
	} else {
		e.Format, e.Readiness = inspectWordlistFormat(logical, real)
		sample, _ := sampleWordlistFile(real, info.Size())
		e.Kinds = classifyWordlistKinds(relative, sample, 0, "inferred")
	}
	e.Compressed = e.Readiness == WordlistReadyNeedsExtract
	e.Usable = e.Readiness == WordlistReadyReady || e.Readiness == WordlistReadyNeedsNormalize
	if !e.Usable && e.Reason == "" {
		switch e.Readiness {
		case WordlistReadyNeedsExtract:
			e.Reason = "compressed file is cataloged but not directly usable"
		case WordlistReadyUnsafe:
			e.Reason = "unsafe file is not an operational candidate"
		default:
			e.Reason = "metadata, binary, or unsupported file"
		}
	}
	e.Kind = primaryWordlistKind(e.Kinds)
	if e.Kind == "" {
		e.Kind = WordlistKindUnknown
	} else if match := wordlistKindMatch(e, e.Kind); match != nil {
		e.Profile = wordlistProfileForTier(e.Kind, match.Tier)
	}
	c.Entries = append(c.Entries, e)
	return nil
}

func wordlistProvider(path string) string {
	p := filepath.ToSlash(path)
	if strings.Contains(p, "/seclists/") {
		return "seclists"
	}
	return WordlistProviderLists
}
func primaryWordlistKind(kinds []WordlistKindMatch) string {
	best := WordlistKindMatch{}
	for _, match := range kinds {
		if best.Name == "" || fitRank(match.Fit) > fitRank(best.Fit) || fitRank(match.Fit) == fitRank(best.Fit) && match.Priority > best.Priority {
			best = match
		}
	}
	return best.Name
}
func isCatalogMetadata(path string) bool {
	b := strings.ToLower(filepath.Base(path))
	return b == "readme" || strings.HasPrefix(b, "readme.") || b == "license" || b == "copying" || b == "changelog" || strings.HasSuffix(b, ".md") || strings.HasSuffix(b, ".html")
}
func bytesCountNewlines(b []byte) int {
	n := 0
	for _, x := range b {
		if x == '\n' {
			n++
		}
	}
	return n
}

func filterWordlistEntries(entries []WordlistEntry, kind string, usableOnly bool) []WordlistEntry {
	out := entries[:0]
	for _, e := range entries {
		if (kind == WordlistKindAll || wordlistKindMatch(e, kind) != nil || kind == WordlistKindUnknown && len(e.Kinds) == 0) && (!usableOnly || e.Usable) {
			out = append(out, e)
		}
	}
	return out
}
func recommendWordlists(entries []WordlistEntry, kind string) []WordlistEntry {
	if kind == WordlistKindAll {
		kind = WordlistKindDirectory
	}
	entries = filterWordlistEntries(entries, kind, true)
	sort.SliceStable(entries, func(i, j int) bool {
		return compareWordlistRecommendations(entries[i], entries[j], kind)
	})
	return entries
}
func wordlistKindMatch(e WordlistEntry, kind string) *WordlistKindMatch {
	for i := range e.Kinds {
		if e.Kinds[i].Name == kind {
			return &e.Kinds[i]
		}
	}
	return nil
}
func compareWordlistRecommendations(a, b WordlistEntry, kind string) bool {
	am, bm := wordlistKindMatch(a, kind), wordlistKindMatch(b, kind)
	if am == nil || bm == nil {
		return am != nil
	}
	if stateRank(a.State) != stateRank(b.State) {
		return stateRank(a.State) > stateRank(b.State)
	}
	if fitRank(am.Fit) != fitRank(bm.Fit) {
		return fitRank(am.Fit) > fitRank(bm.Fit)
	}
	if am.Priority != bm.Priority {
		return am.Priority > bm.Priority
	}
	if tierRank(am.Tier) != tierRank(bm.Tier) {
		return tierRank(am.Tier) < tierRank(bm.Tier)
	}
	if am.Confidence != bm.Confidence {
		return am.Confidence > bm.Confidence
	}
	if a.Size != b.Size {
		return a.Size < b.Size
	}
	return a.Path < b.Path
}
func stateRank(state string) int {
	switch state {
	case WordlistStateKnownVerified:
		return 3
	case WordlistStateKnownModified:
		return 2
	default:
		return 1
	}
}
func fitRank(fit string) int {
	if fit == WordlistFitPrimary {
		return 2
	}
	return 1
}
func tierRank(tier string) int {
	switch tier {
	case WordlistTierQuick:
		return 0
	case WordlistTierStandard:
		return 1
	default:
		return 2
	}
}
func findWordlistEntry(entries []WordlistEntry, path string) (WordlistEntry, bool) {
	for _, e := range entries {
		if e.Path == path || e.ResolvedPath == path {
			return e, true
		}
	}
	return WordlistEntry{}, false
}

func writeWordlistOutput(w io.Writer, output outputOptions, c wordlistCatalog, action string) error {
	switch output.Format {
	case apiFormatJSON:
		return runJSONEndpoint(w, "wordlist", output.FormatVersion, func(version string) (any, error) {
			return map[string]any{"format_version": version, "action": action, "root": c.Root, "kind": c.QueryKind, "entries": c.Entries, "warnings": c.Warnings}, nil
		})
	case "markdown":
		if c.QueryKind != "" && c.QueryKind != WordlistKindAll {
			fmt.Fprintf(w, "# Wordlists\n\nRoot: `%s`\n\n| Fit | Tier | Confidence | State | Path |\n|---|---|---:|---|---|\n", c.Root)
			for _, e := range c.Entries {
				match := wordlistKindMatch(e, c.QueryKind)
				if match != nil {
					fmt.Fprintf(w, "| %s | %s | %d | %s | `%s` |\n", match.Fit, match.Tier, match.Confidence, e.State, e.Path)
				}
			}
			return nil
		}
		fmt.Fprintf(w, "# Wordlists\n\nRoot: `%s`\n\n| Kinds | State | Usable | Path |\n|---|---|---:|---|\n", c.Root)
		for _, e := range c.Entries {
			fmt.Fprintf(w, "| %s | %s | %t | `%s` |\n", wordlistKindNames(e.Kinds), e.State, e.Usable, e.Path)
		}
		return nil
	default:
		if c.QueryKind != "" && c.QueryKind != WordlistKindAll {
			fmt.Fprintf(w, "Root: %s\n\n%-15s %-10s %-10s %-10s %-7s %s\n", c.Root, "STATE", "FIT", "TIER", "CONFIDENCE", "LINES", "PATH")
			for _, e := range c.Entries {
				match := wordlistKindMatch(e, c.QueryKind)
				if match != nil {
					fmt.Fprintf(w, "%-15s %-10s %-10s %-10d %-7s %s\n", e.State, match.Fit, match.Tier, match.Confidence, strconv.FormatInt(e.Lines, 10), e.Path)
				}
			}
			return nil
		}
		fmt.Fprintf(w, "Root: %s\n\n%-15s %-24s %-12s %-10s %-7s %s\n", c.Root, "STATE", "KINDS", "READINESS", "LINES", "SIZE", "PATH")
		for _, e := range c.Entries {
			fmt.Fprintf(w, "%-15s %-24s %-12s %-10s %-7s %s\n", e.State, wordlistKindNames(e.Kinds), e.Readiness, strconv.FormatInt(e.Lines, 10), strconv.FormatInt(e.Size, 10), e.Path)
		}
		return nil
	}
}

func wordlistKindNames(kinds []WordlistKindMatch) string {
	if len(kinds) == 0 {
		return WordlistKindUnknown
	}
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, kind.Name)
	}
	return strings.Join(names, ",")
}
