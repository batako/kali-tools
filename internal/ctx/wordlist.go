package ctx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	WordlistProviderLists           = "wordlists"
	WordlistTypeDirectory           = "directory"
	WordlistTypeEndpoint            = "endpoint"
	WordlistTypeParameter           = "parameter"
	WordlistTypePassword            = "password"
	WordlistTypeUnknown             = "unknown"
	WordlistProfileWebQuick         = "web-quick"
	WordlistProfileWebStandard      = "web-standard"
	WordlistProfileWebDeep          = "web-deep"
	WordlistProfilePasswordQuick    = "password-quick"
	WordlistProfilePasswordStandard = "password-standard"
	WordlistProfilePasswordDeep     = "password-deep"
	WordlistProfileUsernameQuick    = "username-quick"
	WordlistProfileUsernameStandard = "username-standard"
	WordlistProfileUsernameDeep     = "username-deep"
)

type WordlistSelection struct {
	Provider string
	Profile  string
	Type     string
	Path     string
}

// DiscoverConfiguredPasswordWordlists returns password wordlists in an order
// suitable for progressive Hydra runs.
func DiscoverConfiguredPasswordWordlists() ([]WordlistSelection, error) {
	root := DiscoverWordlistsRoot()
	if root == "" {
		return nil, fmt.Errorf("wordlists directory not found; install the wordlists package")
	}

	type source struct {
		path     string
		provider string
		rank     int
	}
	sources := []source{
		{path: root, provider: WordlistProviderLists, rank: 0},
		{path: filepath.Join(root, "seclists", "Passwords"), provider: "seclists", rank: 1},
		{path: "/usr/share/seclists/Passwords", provider: "seclists", rank: 1},
	}
	type candidate struct {
		selection WordlistSelection
		rank      int
		size      int64
		key       string
	}
	var candidates []candidate
	seen := make(map[string]bool)
	for _, source := range sources {
		base, err := filepath.EvalSymlinks(source.path)
		if err != nil || !isDirectory(base) {
			continue
		}
		err = filepath.Walk(base, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info == nil || !info.Mode().IsRegular() || !isWordlistFile(path) || classifyWordlist(path) != WordlistTypePassword {
				return nil
			}
			realPath, evalErr := filepath.EvalSymlinks(path)
			if evalErr == nil && seen[realPath] {
				return nil
			}
			if evalErr == nil {
				seen[realPath] = true
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rank := passwordWordlistRank(path)
			profile := WordlistProfilePasswordDeep
			if rank == 0 {
				profile = WordlistProfilePasswordQuick
			} else if rank == 1 {
				profile = WordlistProfilePasswordStandard
			}
			candidates = append(candidates, candidate{
				selection: WordlistSelection{Provider: source.provider, Profile: profile, Type: WordlistTypePassword, Path: path},
				rank:      rank, size: info.Size(), key: relative,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan password wordlists: %w", err)
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
	result := make([]WordlistSelection, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.selection)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no password wordlist found; install wordlists or seclists")
	}
	return result, nil
}

// DiscoverConfiguredUsernameWordlists returns username wordlists in a
// deterministic order suitable for progressive Hydra runs.
func DiscoverConfiguredUsernameWordlists() ([]WordlistSelection, error) {
	root := DiscoverWordlistsRoot()
	if root == "" {
		return nil, fmt.Errorf("wordlists directory not found; install the wordlists package")
	}
	type source struct {
		path     string
		provider string
	}
	sources := []source{
		{path: root, provider: WordlistProviderLists},
		{path: filepath.Join(root, "seclists", "Usernames"), provider: "seclists"},
		{path: "/usr/share/seclists/Usernames", provider: "seclists"},
	}
	type candidate struct {
		selection WordlistSelection
		rank      int
		size      int64
		key       string
	}
	var candidates []candidate
	seen := make(map[string]bool)
	for _, source := range sources {
		base, err := filepath.EvalSymlinks(source.path)
		if err != nil || !isDirectory(base) {
			continue
		}
		err = filepath.Walk(base, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info == nil || !info.Mode().IsRegular() || !isWordlistFile(path) || !isUsernameWordlist(path) {
				return nil
			}
			realPath, evalErr := filepath.EvalSymlinks(path)
			if evalErr == nil && seen[realPath] {
				return nil
			}
			if evalErr == nil {
				seen[realPath] = true
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rank := usernameWordlistRank(path)
			profile := WordlistProfileUsernameDeep
			if rank == 0 {
				profile = WordlistProfileUsernameQuick
			} else if rank == 1 {
				profile = WordlistProfileUsernameStandard
			}
			candidates = append(candidates, candidate{
				selection: WordlistSelection{Provider: source.provider, Profile: profile, Type: WordlistTypeUnknown, Path: path},
				rank:      rank, size: info.Size(), key: relative,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan username wordlists: %w", err)
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
	result := make([]WordlistSelection, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.selection)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no username wordlist found; install seclists or provide --user-list")
	}
	return result, nil
}

// DiscoverConfiguredWebWordlists returns directory wordlists from the
// standard Kali wordlists directory in an efficient, deterministic order.
func DiscoverConfiguredWebWordlists() ([]WordlistSelection, error) {
	root := DiscoverWordlistsRoot()
	if root == "" {
		return nil, fmt.Errorf("wordlists directory not found; install the wordlists package")
	}

	type candidate struct {
		selection    WordlistSelection
		rank         int
		size         int64
		providerRank int
		key          string
	}
	var candidates []candidate
	seen := make(map[string]bool)
	for _, source := range webWordlistRoots(root) {
		base, evalErr := filepath.EvalSymlinks(source.path)
		if evalErr != nil || !isDirectory(base) {
			continue
		}
		err := filepath.Walk(base, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info == nil || !info.Mode().IsRegular() || !isWordlistFile(path) {
				return nil
			}
			listType := classifyWordlist(path)
			if listType != WordlistTypeDirectory {
				return nil
			}
			realPath, evalErr := filepath.EvalSymlinks(path)
			if evalErr == nil && seen[realPath] {
				return nil
			}
			if evalErr == nil {
				seen[realPath] = true
			}
			stat, statErr := os.Stat(path)
			if statErr != nil {
				return statErr
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			profile, rank := webWordlistProfile(relative)
			candidates = append(candidates, candidate{
				selection: WordlistSelection{Provider: WordlistProviderLists, Profile: profile, Type: listType, Path: path},
				rank:      rank, size: stat.Size(), providerRank: source.rank, key: relative,
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to scan wordlists: %w", err)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		if candidates[i].providerRank != candidates[j].providerRank {
			return candidates[i].providerRank < candidates[j].providerRank
		}
		if candidates[i].size != candidates[j].size {
			return candidates[i].size < candidates[j].size
		}
		return candidates[i].key < candidates[j].key
	})
	result := make([]WordlistSelection, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, candidate.selection)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no directory wordlist found under %s; install seclists, dirb, or dirbuster", root)
	}
	return result, nil
}

type webWordlistRoot struct {
	path string
	rank int
}

func webWordlistRoots(root string) []webWordlistRoot {
	return []webWordlistRoot{
		{path: filepath.Join(root, "dirb"), rank: 0},
		{path: filepath.Join(root, "dirbuster"), rank: 1},
		{path: filepath.Join(root, "seclists", "Discovery", "Web-Content"), rank: 2},
	}
}

func classifyWordlist(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))
	switch {
	case strings.Contains(lower, "/password"), strings.Contains(lower, "/user"), strings.Contains(lower, "rockyou"), strings.Contains(lower, "john.lst"), strings.Contains(lower, "fasttrack"):
		return WordlistTypePassword
	case strings.Contains(lower, "/parameter"), strings.Contains(lower, "/payload"), strings.Contains(lower, "/lfi"):
		return WordlistTypeParameter
	case strings.Contains(lower, "/fuzz"), strings.Contains(lower, ".fuzz."), strings.Contains(lower, "/cgi"):
		return WordlistTypeEndpoint
	case strings.Contains(lower, "/dirb/"), strings.Contains(lower, "/dirbuster/"), strings.Contains(lower, "/seclists/discovery/web-content/"):
		return WordlistTypeDirectory
	default:
		return WordlistTypeUnknown
	}
}

func passwordWordlistRank(path string) int {
	lower := strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(lower, "best1050") || strings.Contains(lower, "fasttrack") || strings.Contains(lower, "common-credentials") && !strings.Contains(lower, "10-million") {
		return 0
	}
	if strings.Contains(lower, "top-100000") || strings.Contains(lower, "top-1000000") || strings.Contains(lower, "10-million") {
		return 1
	}
	return 2
}

func isUsernameWordlist(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(lower, "password") || strings.Contains(lower, "rockyou") || strings.Contains(lower, "john.lst") {
		return false
	}
	return strings.Contains(lower, "username") || strings.Contains(lower, "usernames") || strings.Contains(lower, "user-list") || strings.Contains(lower, "userlist") || strings.Contains(lower, "/names") || strings.Contains(lower, "login") || strings.Contains(lower, "account")
}

func usernameWordlistRank(path string) int {
	lower := strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(lower, "common") || strings.Contains(lower, "small") || strings.Contains(lower, "top-100") || strings.Contains(lower, "top-1000") {
		return 0
	}
	if strings.Contains(lower, "medium") || strings.Contains(lower, "top-10000") || strings.Contains(lower, "top-100000") {
		return 1
	}
	return 2
}

func isWordlistFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".txt" || ext == ".lst" || ext == ".list"
}

func webWordlistProfile(path string) (string, int) {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "common") || strings.Contains(lower, "quick") || strings.Contains(lower, "small") {
		return WordlistProfileWebQuick, 0
	}
	if strings.Contains(lower, "medium") || strings.Contains(lower, "raft") || strings.Contains(lower, "directory-list") {
		return WordlistProfileWebStandard, 1
	}
	return WordlistProfileWebDeep, 2
}

func DiscoverWordlistsRoot() string {
	home, _ := os.UserHomeDir()
	for _, root := range []string{"/usr/share/wordlists", "/usr/local/share/wordlists", filepath.Join(home, "wordlists")} {
		if isDirectory(root) {
			return root
		}
	}
	return ""
}

func ResolveConfiguredWordlist(profile string) (WordlistSelection, error) {
	selections, err := DiscoverConfiguredWebWordlists()
	if err != nil {
		return WordlistSelection{}, err
	}
	for _, selection := range selections {
		if selection.Profile == profile {
			return selection, nil
		}
	}
	return WordlistSelection{}, fmt.Errorf("no wordlist found for profile %s", profile)
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
