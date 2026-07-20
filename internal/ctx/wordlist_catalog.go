package ctx

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	WordlistFitPrimary    = "primary"
	WordlistFitCompatible = "compatible"

	WordlistTierQuick    = "quick"
	WordlistTierStandard = "standard"
	WordlistTierDeep     = "deep"

	WordlistStateKnownVerified = "known-verified"
	WordlistStateKnownModified = "known-modified"
	WordlistStateUnknown       = "unknown"

	WordlistReadyReady          = "ready"
	WordlistReadyNeedsExtract   = "needs-extract"
	WordlistReadyNeedsNormalize = "needs-normalize"
	WordlistReadyUnsupported    = "unsupported"
	WordlistReadyUnsafe         = "unsafe"
)

type WordlistKindMatch struct {
	Name           string `json:"name"`
	Fit            string `json:"fit"`
	Tier           string `json:"tier"`
	Priority       int    `json:"priority"`
	Confidence     int    `json:"confidence"`
	Reason         string `json:"reason,omitempty"`
	Classification string `json:"classification"`
}

type wordlistManifestRecord struct {
	Path      string              `json:"path"`
	Provider  string              `json:"provider"`
	SHA256    string              `json:"sha256"`
	Size      int64               `json:"size"`
	Lines     int64               `json:"lines"`
	Format    string              `json:"format"`
	Readiness string              `json:"readiness"`
	Kinds     []WordlistKindMatch `json:"kinds,omitempty"`
}

type wordlistManifest struct {
	Version  int                      `json:"version"`
	Root     string                   `json:"root"`
	Packages map[string]string        `json:"packages,omitempty"`
	Records  []wordlistManifestRecord `json:"records"`
}

//go:embed wordlists_manifest.json
var embeddedWordlistManifest []byte

func loadWordlistManifest() (wordlistManifest, error) {
	var manifest wordlistManifest
	if err := json.Unmarshal(embeddedWordlistManifest, &manifest); err != nil {
		return manifest, fmt.Errorf("failed to load embedded wordlist catalog: %w", err)
	}
	if manifest.Version != 1 {
		return manifest, fmt.Errorf("unsupported embedded wordlist catalog version: %d", manifest.Version)
	}
	return manifest, nil
}

func wordlistManifestIndex() (map[string]wordlistManifestRecord, error) {
	manifest, err := loadWordlistManifest()
	if err != nil {
		return nil, err
	}
	index := make(map[string]wordlistManifestRecord, len(manifest.Records))
	for _, record := range manifest.Records {
		index[filepath.ToSlash(filepath.Clean(record.Path))] = record
	}
	return index, nil
}

// GenerateWordlistManifest builds the immutable catalog committed with ctx.
// It is intended for the development helper, not normal ctx execution.
func GenerateWordlistManifest(root string, packages map[string]string) (wordlistManifest, error) {
	manifest := wordlistManifest{Version: 1, Root: "/usr/share/wordlists", Packages: packages}
	seen := map[string]bool{}
	err := walkLogicalWordlistFiles(root, func(logical, real string, info os.FileInfo) error {
		resolved, err := filepath.EvalSymlinks(real)
		if err != nil {
			return nil
		}
		if seen[resolved] {
			return nil
		}
		seen[resolved] = true
		relative, err := filepath.Rel(root, logical)
		if err != nil {
			return err
		}
		format, readiness := inspectWordlistFormat(logical, real)
		lines := int64(0)
		var hash string
		if readiness == WordlistReadyReady || readiness == WordlistReadyNeedsNormalize {
			hash, lines, err = hashAndCountPlainFile(real)
		} else {
			hash, err = sha256File(real)
		}
		if err != nil {
			return err
		}
		sample, _ := sampleWordlistFile(real, info.Size())
		manifest.Records = append(manifest.Records, wordlistManifestRecord{
			Path:      filepath.ToSlash(relative),
			Provider:  wordlistProvider(logical),
			SHA256:    hash,
			Size:      info.Size(),
			Lines:     lines,
			Format:    format,
			Readiness: readiness,
			Kinds:     classifyWordlistKinds(relative, sample, lines, "curated"),
		})
		return nil
	})
	if err != nil {
		return manifest, err
	}
	sort.Slice(manifest.Records, func(i, j int) bool { return manifest.Records[i].Path < manifest.Records[j].Path })
	return manifest, nil
}

func MarshalWordlistManifest(manifest wordlistManifest) ([]byte, error) {
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func walkLogicalWordlistFiles(root string, visit func(logical, real string, info os.FileInfo) error) error {
	var walk func(string, string) error
	walk = func(logical, real string) error {
		info, err := os.Lstat(logical)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(logical)
			if err != nil {
				return nil
			}
			target, err := os.Stat(resolved)
			if err != nil {
				return nil
			}
			if target.IsDir() {
				items, err := os.ReadDir(resolved)
				if err != nil {
					return err
				}
				for _, item := range items {
					if err := walk(filepath.Join(logical, item.Name()), filepath.Join(resolved, item.Name())); err != nil {
						return err
					}
				}
				return nil
			}
			return visit(logical, resolved, target)
		}
		if info.IsDir() {
			items, err := os.ReadDir(real)
			if err != nil {
				return err
			}
			for _, item := range items {
				if err := walk(filepath.Join(logical, item.Name()), filepath.Join(real, item.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if info.Mode().IsRegular() {
			return visit(logical, real, info)
		}
		return nil
	}
	return walk(root, root)
}

func inspectWordlistFormat(logical, real string) (string, string) {
	lower := strings.ToLower(logical)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar-gzip", WordlistReadyNeedsExtract
	case strings.HasSuffix(lower, ".gz"):
		return "gzip", WordlistReadyNeedsExtract
	case strings.HasSuffix(lower, ".zip"):
		return "zip", WordlistReadyNeedsExtract
	case strings.HasSuffix(lower, ".bz2"):
		return "bzip2", WordlistReadyNeedsExtract
	case strings.HasSuffix(lower, ".xz"):
		return "xz", WordlistReadyNeedsExtract
	case strings.HasSuffix(lower, ".tar"):
		return "tar", WordlistReadyNeedsExtract
	case strings.HasSuffix(lower, ".7z"):
		return "7zip", WordlistReadyNeedsExtract
	case strings.HasSuffix(lower, ".rar"):
		return "rar", WordlistReadyNeedsExtract
	}
	f, err := os.Open(real)
	if err != nil {
		return "unreadable", WordlistReadyUnsupported
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	buf = buf[:n]
	if strings.IndexByte(string(buf), 0) >= 0 || !utf8.Valid(buf) {
		return "binary", WordlistReadyUnsupported
	}
	if isCatalogMetadata(logical) {
		return "metadata", WordlistReadyUnsupported
	}
	return "text", WordlistReadyReady
}

func hashAndCountPlainFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 64*1024)
	hash := sha256.New()
	var lines int64
	var readAny bool
	var last byte
	buf := make([]byte, 64*1024)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			if _, err := hash.Write(buf[:n]); err != nil {
				return "", 0, err
			}
			readAny = true
			last = buf[n-1]
			lines += int64(bytesCountNewlines(buf[:n]))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", 0, readErr
		}
	}
	if readAny && last != '\n' {
		lines++
	}
	return hex.EncodeToString(hash.Sum(nil)), lines, nil
}

func sampleWordlistFile(path string, size int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const window = int64(4096)
	offsets := []int64{0}
	if size > window {
		offsets = append(offsets, size/4, size/2, size*3/4, maxInt64(0, size-window))
	}
	var sample []byte
	for _, offset := range offsets {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return sample, err
		}
		buf := make([]byte, window)
		n, _ := io.ReadFull(f, buf)
		sample = append(sample, buf[:n]...)
		sample = append(sample, '\n')
	}
	return sample, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func classifyWordlistKinds(path string, sample []byte, lines int64, classification string) []WordlistKindMatch {
	p := strings.ToLower(filepath.ToSlash(path))
	b := strings.ToLower(filepath.Base(path))
	s := strings.ToLower(string(sample))
	result := map[string]WordlistKindMatch{}
	add := func(name, fit string, priority, confidence int, reason string) {
		match := WordlistKindMatch{Name: name, Fit: fit, Tier: tierForLines(lines), Priority: priority, Confidence: confidence, Reason: reason, Classification: classification}
		if old, exists := result[name]; !exists || match.Priority > old.Priority {
			result[name] = match
		}
	}

	userPass := strings.Contains(p, "userpass") || strings.Contains(p, "user-pass") || strings.Contains(p, "user_pass")
	if strings.Contains(p, "/passwords/") || strings.Contains(p, "/password/") || strings.Contains(b, "password") || strings.Contains(b, "passwd") || strings.Contains(b, "rockyou") || b == "john.lst" || userPass {
		add(WordlistKindPassword, WordlistFitPrimary, 90, 98, "password-oriented list")
	}
	if strings.Contains(p, "/usernames/") || strings.Contains(p, "/names/") || strings.Contains(b, "username") || strings.Contains(b, "user-list") || strings.Contains(b, "user_list") || strings.HasSuffix(b, "_users.txt") || strings.HasSuffix(b, "_user.txt") || strings.Contains(b, "apache-user-enum") || userPass {
		add(WordlistKindUsername, WordlistFitPrimary, 90, 97, "username-oriented list")
	}
	if isSubdomainWordlistPath(p, b) {
		add(WordlistKindSubdomain, WordlistFitPrimary, 90, 98, "DNS or subdomain discovery list")
	}
	if strings.Contains(p, "/paramminer/") || strings.Contains(b, "parameter-name") || strings.Contains(b, "parameter_names") || strings.Contains(b, "parameters.txt") || strings.Contains(b, "url-params") {
		add(WordlistKindParameterName, WordlistFitPrimary, 95, 99, "HTTP parameter-name list")
	}
	webContent := strings.Contains(p, "/discovery/web-content/")
	directoryTree := strings.HasPrefix(p, "dirb/") || strings.HasPrefix(p, "dirbuster/") || webContent || strings.Contains(p, "/wfuzz/general/")
	if directoryTree && !strings.Contains(p, "/paramminer/") && !strings.Contains(b, "parameter") && !strings.Contains(b, "params") && !strings.Contains(b, "headers") {
		add(WordlistKindDirectory, WordlistFitPrimary, 80, 92, "web content discovery list")
	}
	apiEndpoint := webContent && (strings.Contains(p, "/api/") || strings.HasPrefix(b, "api-") || strings.HasPrefix(b, "api_") || strings.Contains(b, "-api-endpoint") || strings.Contains(b, "_api_endpoint") || strings.Contains(b, "common-api"))
	webContentURLs := strings.Contains(p, "/discovery/web-content/urls/")
	if strings.Contains(p, "/cgis/") || strings.Contains(b, "cgi") || strings.Contains(b, "endpoint") || apiEndpoint || webContentURLs || strings.Contains(b, "wp-plugins") || strings.Contains(b, "wp-themes") {
		add(WordlistKindEndpoint, WordlistFitPrimary, 85, 94, "endpoint-specific list")
		if webContent {
			add(WordlistKindDirectory, WordlistFitCompatible, 35, 85, "endpoints can be used for web content discovery")
		}
	}
	parameterPath := strings.Contains(p, "/fuzzing/") || strings.Contains(p, "/payloads/") || strings.Contains(p, "/payload/") || strings.Contains(p, "/lfi/") || strings.Contains(p, "/sqli/") || strings.Contains(p, "/xss/") || strings.Contains(p, "traversal") || strings.Contains(p, "/wfuzz/injections/") || strings.Contains(p, "/wfuzz/vulns/")
	parameterPath = parameterPath && !strings.Contains(p, "/fuzzing/user-agents/")
	if parameterPath {
		add(WordlistKindParameterValue, WordlistFitPrimary, 75, 92, "request-value or fuzzing payload list")
	}
	if isURLValueWordlistBase(b) && !webContentURLs {
		add(WordlistKindParameterValue, WordlistFitPrimary, 82, 90, "URL-like parameter values")
	}
	if len(result) == 0 && classification == "inferred" {
		linesInSample := strings.Split(s, "\n")
		domainLike, pathLike := 0, 0
		for _, line := range linesInSample {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.ContainsAny(line, " /:@") && strings.Contains(line, ".") {
				domainLike++
			}
			if strings.HasPrefix(line, "/") || strings.Contains(line, "/") {
				pathLike++
			}
		}
		if domainLike >= 5 {
			add(WordlistKindSubdomain, WordlistFitCompatible, 10, 55, "sample contains domain-like values")
		}
		if pathLike >= 5 {
			add(WordlistKindDirectory, WordlistFitCompatible, 10, 50, "sample contains path-like values")
		}
	}
	for name, match := range result {
		curated, keep := curateWordlistKind(path, match)
		if !keep {
			delete(result, name)
			continue
		}
		result[name] = curated
	}

	kinds := make([]WordlistKindMatch, 0, len(result))
	for _, match := range result {
		kinds = append(kinds, match)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i].Name < kinds[j].Name })
	return kinds
}

func isURLValueWordlistBase(base string) bool {
	return strings.HasPrefix(base, "url-") || strings.HasPrefix(base, "url_") || strings.HasPrefix(base, "urls-") || strings.HasPrefix(base, "urls_") || strings.Contains(base, "-urls.") || strings.Contains(base, "_urls.") || strings.Contains(base, "redirect")
}

func isSubdomainWordlistPath(path, base string) bool {
	if base == "tlds.txt" || base == "services-names.txt" || strings.Contains(base, "resolver") {
		return false
	}
	return strings.Contains(path, "/discovery/dns/") || strings.Contains(base, "subdomain") || strings.Contains(base, "subdomains") || strings.Contains(base, "dns-") || strings.Contains(base, "dns_")
}

func curateWordlistKind(path string, match WordlistKindMatch) (WordlistKindMatch, bool) {
	p := strings.ToLower(filepath.ToSlash(path))
	b := strings.ToLower(filepath.Base(path))
	set := func(fit string, priority int, reason string) {
		match.Fit = fit
		match.Priority = priority
		if reason != "" {
			match.Reason = reason
		}
	}
	switch match.Name {
	case WordlistKindDirectory:
		switch {
		case strings.Contains(p, "file-extensions"), strings.Contains(b, "extension"), strings.Contains(p, "/cms/"), strings.Contains(p, "/web-servers/"), strings.Contains(p, "programming-language-specific"), strings.Contains(p, "domino-hunter"), strings.Contains(p, "/discovery/web-content/urls/"):
			set(WordlistFitCompatible, 20, "specialized web-content list")
		case b == "common.txt" && strings.HasPrefix(p, "dirb/"):
			set(WordlistFitPrimary, 120, "small established general web-content list")
		case b == "common.txt" || b == "common_directories.txt":
			set(WordlistFitPrimary, 115, "common web-content names")
		case strings.Contains(b, "raft-small"):
			set(WordlistFitPrimary, 110, "small RAFT web-content list")
		case strings.Contains(b, "directory-list-2.3-small"):
			set(WordlistFitPrimary, 105, "small DirBuster directory list")
		case strings.Contains(b, "raft-medium"):
			set(WordlistFitPrimary, 100, "medium RAFT web-content list")
		case strings.Contains(b, "directory-list-2.3-medium"):
			set(WordlistFitPrimary, 95, "medium DirBuster directory list")
		case strings.Contains(b, "raft-large") || strings.Contains(b, "directory-list-1.0"):
			set(WordlistFitPrimary, 85, "large general web-content list")
		default:
			set(match.Fit, 50, "web-content discovery list")
		}
	case WordlistKindSubdomain:
		switch {
		case strings.Contains(b, "subdomains-top1million-5000"):
			set(WordlistFitPrimary, 120, "small general subdomain list")
		case strings.Contains(b, "subdomains-top1million-20000"):
			set(WordlistFitPrimary, 115, "medium general subdomain list")
		case strings.Contains(b, "subdomains-top1million-110000"), strings.Contains(b, "bitquark-subdomains-top100000"):
			set(WordlistFitPrimary, 105, "large general subdomain list")
		case strings.Contains(b, "dns-jhaddix"), strings.Contains(b, "combined_subdomains"), strings.Contains(b, "shubs-subdomains"):
			set(WordlistFitPrimary, 100, "broad subdomain discovery list")
		case strings.Contains(p, "metasploit/"), strings.Contains(b, "spanish"), strings.Contains(b, "italian"):
			set(WordlistFitCompatible, 30, "platform- or language-specific subdomain list")
		default:
			set(match.Fit, 60, "subdomain discovery list")
		}
	case WordlistKindParameterName:
		if strings.Contains(b, "burp-parameter-names") {
			set(WordlistFitPrimary, 120, "general HTTP parameter-name list")
		} else {
			set(match.Fit, 80, "HTTP parameter-name list")
		}
	case WordlistKindParameterValue:
		switch {
		case strings.Contains(b, "url-params"):
			return match, false
		case isURLValueWordlistBase(b):
			set(WordlistFitCompatible, 40, "specialized URL-like parameter values")
		case strings.Contains(b, "fuzz-bo0om-friendly"), strings.Contains(b, "big-list-of-naughty-strings"), strings.Contains(b, "special-chars"):
			set(WordlistFitPrimary, 115, "general parameter fuzzing values")
		case strings.Contains(p, "/fuzzing/amounts/"), strings.Contains(p, "/fuzzing/dates/"):
			set(WordlistFitCompatible, 30, "specialized structured parameter values")
		case strings.Contains(b, "digits"), strings.Contains(b, "numeric"):
			set(WordlistFitPrimary, 105, "structured parameter values")
		case strings.Contains(p, "/fuzzing/"):
			set(WordlistFitPrimary, 80, "request-value fuzzing payloads")
		default:
			set(WordlistFitCompatible, 35, "specialized request-value payloads")
		}
	case WordlistKindPassword:
		switch {
		case strings.Contains(p, "/php-hashes/"), strings.Contains(p, "/hashes/"):
			return match, false
		case strings.Contains(b, "best1050"), strings.Contains(b, "fasttrack"):
			set(WordlistFitPrimary, 130, "small common-password list")
		case b == "rockyou.txt" || b == "rockyou.txt.gz":
			set(WordlistFitPrimary, 100, "established broad password list")
		case strings.Contains(b, "10-million-password-list-top-100000"):
			set(WordlistFitPrimary, 122, "common top-100000 password list")
		case strings.Contains(p, "/common-credentials/language-specific/"):
			set(WordlistFitCompatible, 45, "language-specific credentials")
		case strings.Contains(p, "/common-credentials/"):
			set(WordlistFitPrimary, 120, "common credential list")
		case strings.Contains(p, "/default-credentials/routers/"):
			set(WordlistFitCompatible, 15, "device-specific default credentials")
		case strings.Contains(p, "/default-credentials/") || strings.Contains(p, "metasploit/"):
			set(WordlistFitCompatible, 35, "service-specific default credentials")
		default:
			set(match.Fit, 60, "password-oriented list")
		}
	case WordlistKindUsername:
		switch {
		case strings.Contains(b, "top-usernames-shortlist"):
			set(WordlistFitPrimary, 125, "small common-username list")
		case strings.Contains(b, "cirt-default-usernames"):
			set(WordlistFitPrimary, 115, "common default usernames")
		case strings.Contains(p, "/names/"):
			set(WordlistFitCompatible, 45, "human names usable for username generation")
		case strings.Contains(p, "/common-credentials/language-specific/"):
			set(WordlistFitCompatible, 40, "language-specific username and password pairs")
		case strings.Contains(p, "metasploit/") || strings.Contains(p, "/default-credentials/"):
			set(WordlistFitCompatible, 35, "service-specific usernames")
		default:
			set(match.Fit, 70, "username-oriented list")
		}
	case WordlistKindEndpoint:
		if strings.Contains(p, "/discovery/web-content/urls/") {
			set(WordlistFitCompatible, 45, "product-specific endpoint paths")
		} else if strings.Contains(b, "api") || strings.Contains(b, "endpoint") {
			set(WordlistFitPrimary, 100, "API or endpoint discovery list")
		} else {
			set(match.Fit, 60, "specialized endpoint list")
		}
	}
	return match, true
}

func tierForLines(lines int64) string {
	switch {
	case lines > 0 && lines <= 5000:
		return WordlistTierQuick
	case lines > 0 && lines <= 100000:
		return WordlistTierStandard
	default:
		return WordlistTierDeep
	}
}

func hashMatches(path, expected string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == expected
}
