package ctx

import (
	"fmt"
	"os"
	"path/filepath"
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

// WordlistSelection is the canonical result shared by ctx wordlist and the
// tool wrappers. Selection and ordering must be implemented only by ctx.
type WordlistSelection struct {
	Provider   string
	Profile    string
	Type       string
	Path       string
	Fit        string
	Tier       string
	Priority   int
	Confidence int
	State      string
	Lines      int64
}

// RecommendWordlists returns directly usable wordlists in the same order as
// `ctx wordlist --kind <kind>`.
func RecommendWordlists(kind string) ([]WordlistSelection, error) {
	root := DiscoverWordlistsRoot()
	if root == "" {
		return nil, fmt.Errorf("wordlists directory not found; install the wordlists package")
	}
	return recommendWordlistsFromRoot(root, kind)
}

func recommendWordlistsFromRoot(root, kind string) ([]WordlistSelection, error) {
	if kind == WordlistKindAll || kind == WordlistKindUnknown || !containsWordlistKind(kind) {
		return nil, fmt.Errorf("invalid recommendation kind: %s", kind)
	}
	catalog, err := buildWordlistCatalog(root)
	if err != nil {
		return nil, err
	}
	entries := recommendWordlists(catalog.Entries, kind)
	if len(entries) == 0 {
		return nil, fmt.Errorf("no suitable %s wordlist found", kind)
	}
	selections := make([]WordlistSelection, 0, len(entries))
	for _, entry := range entries {
		match := wordlistKindMatch(entry, kind)
		if match == nil {
			continue
		}
		selections = append(selections, WordlistSelection{
			Provider:   entry.Provider,
			Profile:    wordlistProfileForTier(kind, match.Tier),
			Type:       kind,
			Path:       entry.Path,
			Fit:        match.Fit,
			Tier:       match.Tier,
			Priority:   match.Priority,
			Confidence: match.Confidence,
			State:      entry.State,
			Lines:      entry.Lines,
		})
	}
	return selections, nil
}

func wordlistProfileForTier(kind, tier string) string {
	switch kind {
	case WordlistKindDirectory:
		switch tier {
		case WordlistTierQuick:
			return WordlistProfileWebQuick
		case WordlistTierStandard:
			return WordlistProfileWebStandard
		default:
			return WordlistProfileWebDeep
		}
	case WordlistKindPassword:
		switch tier {
		case WordlistTierQuick:
			return WordlistProfilePasswordQuick
		case WordlistTierStandard:
			return WordlistProfilePasswordStandard
		default:
			return WordlistProfilePasswordDeep
		}
	case WordlistKindUsername:
		switch tier {
		case WordlistTierQuick:
			return WordlistProfileUsernameQuick
		case WordlistTierStandard:
			return WordlistProfileUsernameStandard
		default:
			return WordlistProfileUsernameDeep
		}
	default:
		return kind + "-" + tier
	}
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

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
