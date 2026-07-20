package ctx

import (
	"compress/gzip"
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
	Path         string `json:"path"`
	ResolvedPath string `json:"resolved_path,omitempty"`
	Provider     string `json:"provider"`
	Kind         string `json:"kind"`
	Profile      string `json:"profile,omitempty"`
	Available    bool   `json:"available"`
	Usable       bool   `json:"usable"`
	Reason       string `json:"reason,omitempty"`
	Size         int64  `json:"size,omitempty"`
	Compressed   bool   `json:"compressed,omitempty"`
	Lines        int64  `json:"lines,omitempty"`
}

type wordlistCatalog struct {
	Root     string          `json:"root"`
	Entries  []WordlistEntry `json:"entries"`
	Warnings []string        `json:"warnings,omitempty"`
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
	implicitList := len(args) == 0 || strings.HasPrefix(args[0], "-")
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
	if implicitList && options.Kind != WordlistKindAll {
		catalog.Entries = recommendWordlists(catalog.Entries, options.Kind)
	} else if options.Kind != WordlistKindAll {
		catalog.Entries = filterWordlistEntries(catalog.Entries, options.Kind, options.UsableOnly)
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
	seen := map[string]bool{}
	if err := scanWordlistNode(root, root, &c, seen); err != nil {
		return c, err
	}
	sort.Slice(c.Entries, func(i, j int) bool { return c.Entries[i].Path < c.Entries[j].Path })
	return c, nil
}

func scanWordlistNode(logical, real string, c *wordlistCatalog, seen map[string]bool) error {
	info, err := os.Lstat(logical)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, e := filepath.EvalSymlinks(logical)
		if e != nil {
			c.Entries = append(c.Entries, WordlistEntry{Path: logical, Provider: wordlistProvider(logical), Kind: WordlistKindUnknown, Reason: "broken symlink"})
			return nil
		}
		target, e := os.Stat(resolved)
		if e != nil {
			c.Entries = append(c.Entries, WordlistEntry{Path: logical, Provider: wordlistProvider(logical), Kind: WordlistKindUnknown, Reason: "unavailable target"})
			return nil
		}
		if target.IsDir() {
			items, e := os.ReadDir(resolved)
			if e != nil {
				c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %v", logical, e))
				return nil
			}
			for _, item := range items {
				if e := scanWordlistNode(filepath.Join(logical, item.Name()), filepath.Join(resolved, item.Name()), c, seen); e != nil {
					c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %v", filepath.Join(logical, item.Name()), e))
				}
			}
			return nil
		}
		return catalogWordlistFile(logical, resolved, target, c, seen)
	}
	if info.IsDir() {
		items, err := os.ReadDir(real)
		if err != nil {
			c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %v", logical, err))
			return nil
		}
		for _, item := range items {
			if err := scanWordlistNode(filepath.Join(logical, item.Name()), filepath.Join(real, item.Name()), c, seen); err != nil {
				c.Warnings = append(c.Warnings, fmt.Sprintf("%s: %v", filepath.Join(logical, item.Name()), err))
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	return catalogWordlistFile(logical, real, info, c, seen)
}

func catalogWordlistFile(logical, real string, info os.FileInfo, c *wordlistCatalog, seen map[string]bool) error {
	resolved, _ := filepath.EvalSymlinks(real)
	key := resolved
	if key == "" {
		key = real
	}
	if seen[key] {
		return nil
	}
	seen[key] = true
	e := WordlistEntry{Path: logical, ResolvedPath: resolved, Provider: wordlistProvider(logical), Kind: classifyCatalogWordlist(logical), Available: true, Size: info.Size()}
	e.Compressed = strings.HasSuffix(strings.ToLower(logical), ".gz") || strings.HasSuffix(strings.ToLower(logical), ".bz2")
	e.Usable = !isCatalogMetadata(logical) && !e.Compressed
	if !e.Usable {
		if e.Compressed {
			e.Reason = "compressed file is cataloged but not directly usable"
		} else {
			e.Reason = "metadata or non-wordlist file"
		}
	}
	e.Profile = wordlistProfile(e.Kind, logical)
	if e.Usable {
		e.Lines = countWordlistLines(real, e.Compressed)
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
func classifyCatalogWordlist(path string) string {
	p := strings.ToLower(filepath.ToSlash(path))
	b := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(p, "/password"), strings.Contains(p, "rockyou"), strings.Contains(p, "john.lst"):
		return WordlistKindPassword
	case strings.Contains(p, "/username"), strings.Contains(p, "/usernames/"), strings.Contains(b, "user"):
		return WordlistKindUsername
	case strings.Contains(p, "parameter-name"), strings.Contains(b, "parameter-names"):
		return WordlistKindParameterName
	case strings.Contains(p, "/lfi/"), strings.Contains(p, "/payloads/"), strings.Contains(p, "/payload/"), strings.Contains(p, "parameter-value"), strings.Contains(b, "redirect"):
		return WordlistKindParameterValue
	case strings.Contains(p, "/fuzzing/"):
		return WordlistKindEndpoint
	case strings.Contains(p, "/discovery/dns/"), strings.Contains(p, "subdomain"), strings.Contains(p, "subdomains"), strings.Contains(b, "dns"):
		return WordlistKindSubdomain
	case strings.Contains(p, "/web-content/"), strings.Contains(p, "/dirb/"), strings.Contains(p, "/dirbuster/"):
		return WordlistKindDirectory
	case strings.Contains(p, "endpoint"), strings.Contains(p, "cgi"):
		return WordlistKindEndpoint
	}
	return WordlistKindUnknown
}
func wordlistProfile(kind, path string) string {
	if kind == WordlistKindDirectory {
		_, r := webWordlistProfile(path)
		if r == 0 {
			return WordlistProfileWebQuick
		}
		if r == 1 {
			return WordlistProfileWebStandard
		}
		return WordlistProfileWebDeep
	}
	lower := strings.ToLower(filepath.ToSlash(path))
	if kind == WordlistKindSubdomain {
		if strings.Contains(lower, "top-1000") || strings.Contains(lower, "top-5000") || strings.Contains(lower, "top-10000") || strings.Contains(lower, "small") || strings.Contains(lower, "common") {
			return "subdomain-quick"
		}
		if strings.Contains(lower, "top-100000") || strings.Contains(lower, "top-1000000") || strings.Contains(lower, "top1million") || strings.Contains(lower, "top-1million") {
			return "subdomain-standard"
		}
		return "subdomain-deep"
	}
	if kind == WordlistKindPassword {
		switch passwordWordlistRank(path) {
		case 0:
			return WordlistProfilePasswordQuick
		case 1:
			return WordlistProfilePasswordStandard
		default:
			return WordlistProfilePasswordDeep
		}
	}
	if kind == WordlistKindUsername {
		switch usernameWordlistRank(path) {
		case 0:
			return WordlistProfileUsernameQuick
		case 1:
			return WordlistProfileUsernameStandard
		default:
			return WordlistProfileUsernameDeep
		}
	}
	return ""
}
func isCatalogMetadata(path string) bool {
	b := strings.ToLower(filepath.Base(path))
	return b == "readme" || strings.HasPrefix(b, "readme.") || b == "license" || b == "copying" || b == "changelog" || strings.HasSuffix(b, ".md") || strings.HasSuffix(b, ".html")
}
func countWordlistLines(path string, compressed bool) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	var r io.Reader = f
	if compressed {
		gz, e := gzip.NewReader(f)
		if e != nil {
			return 0
		}
		defer gz.Close()
		r = gz
	}
	buf := make([]byte, 32*1024)
	var n int64
	for {
		k, e := r.Read(buf)
		n += int64(bytesCountNewlines(buf[:k]))
		if e != nil {
			break
		}
	}
	return n
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
		if (kind == WordlistKindAll || e.Kind == kind) && (!usableOnly || e.Usable) {
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
		return wordlistSuggestionScore(entries[i]) > wordlistSuggestionScore(entries[j])
	})
	return entries
}
func wordlistSuggestionScore(e WordlistEntry) int {
	p := strings.ToLower(e.Path)
	score := 0
	if e.Profile != "" {
		if strings.HasSuffix(e.Profile, "quick") {
			score += 30
		} else if strings.HasSuffix(e.Profile, "standard") {
			score += 20
		}
	}
	if e.Kind == WordlistKindParameterName && strings.Contains(p, "burp") {
		score += 40
	}
	return score
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
			return map[string]any{"format_version": version, "action": action, "root": c.Root, "entries": c.Entries, "warnings": c.Warnings}, nil
		})
	case "markdown":
		fmt.Fprintf(w, "# Wordlists\n\nRoot: `%s`\n\n| Kind | Usable | Path | Profile |\n|---|---:|---|---|\n", c.Root)
		for _, e := range c.Entries {
			fmt.Fprintf(w, "| %s | %t | `%s` | %s |\n", e.Kind, e.Usable, e.Path, e.Profile)
		}
		return nil
	default:
		fmt.Fprintf(w, "Root: %s\n\n%-6s %-18s %-20s %-10s %-7s %s\n", c.Root, "USABLE", "KIND", "PROFILE", "LINES", "SIZE", "PATH")
		for _, e := range c.Entries {
			fmt.Fprintf(w, "%-6t %-18s %-20s %-10s %-7s %s\n", e.Usable, e.Kind, e.Profile, strconv.FormatInt(e.Lines, 10), strconv.FormatInt(e.Size, 10), e.Path)
		}
		return nil
	}
}
