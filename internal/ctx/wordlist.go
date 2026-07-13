package ctx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	WordlistProviderSecLists   = "seclists"
	WordlistProviderLists      = "wordlists"
	WordlistProfileWebQuick    = "web-quick"
	WordlistProfileWebStandard = "web-standard"
	WordlistProfileWebDeep     = "web-deep"
)

type WordlistProvider struct {
	Name string
	Root string
}

type WordlistSelection struct {
	Provider string
	Profile  string
	Path     string
}

var wordlistProfileCandidates = map[string]map[string][]string{
	WordlistProfileWebQuick: {
		WordlistProviderSecLists: {
			"Discovery/Web-Content/common.txt",
			"Discovery/Web-Content/quickhits.txt",
			"Discovery/Web-Content/directory-list-2.3-small.txt",
		},
		WordlistProviderLists: {
			"dirb/common.txt",
			"dirbuster/directory-list-2.3-small.txt",
		},
	},
	WordlistProfileWebStandard: {
		WordlistProviderSecLists: {
			"Discovery/Web-Content/directory-list-2.3-small.txt",
			"Discovery/Web-Content/raft-small-directories.txt",
			"Discovery/Web-Content/directory-list-2.3-medium.txt",
		},
		WordlistProviderLists: {
			"dirbuster/directory-list-2.3-medium.txt",
			"dirb/common.txt",
		},
	},
	WordlistProfileWebDeep: {
		WordlistProviderSecLists: {
			"Discovery/Web-Content/directory-list-2.3-medium.txt",
			"Discovery/Web-Content/raft-medium-directories.txt",
			"Discovery/Web-Content/directory-list-2.3-big.txt",
		},
		WordlistProviderLists: {
			"dirbuster/directory-list-2.3-big.txt",
			"dirbuster/directory-list-2.3-medium.txt",
		},
	},
}

func NormalizeWordlistProviders(value string) ([]string, error) {
	var providers []string
	seen := make(map[string]bool)
	for _, raw := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}) {
		provider := strings.ToLower(strings.TrimSpace(raw))
		if provider == "" || seen[provider] {
			continue
		}
		if provider != WordlistProviderSecLists && provider != WordlistProviderLists {
			return nil, fmt.Errorf("unsupported wordlist provider: %s", provider)
		}
		seen[provider] = true
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("wordlist providers must not be empty")
	}
	return providers, nil
}

func DiscoverWordlistProviders() []WordlistProvider {
	home, _ := os.UserHomeDir()
	candidates := map[string][]string{
		WordlistProviderSecLists: {
			"/usr/share/seclists",
			"/usr/local/share/seclists",
			filepath.Join(home, "SecLists"),
		},
		WordlistProviderLists: {
			"/usr/share/wordlists",
			"/usr/local/share/wordlists",
			filepath.Join(home, "wordlists"),
		},
	}

	providers := make([]WordlistProvider, 0, len(candidates))
	for _, name := range []string{WordlistProviderSecLists, WordlistProviderLists} {
		for _, root := range candidates[name] {
			if isDirectory(root) {
				providers = append(providers, WordlistProvider{Name: name, Root: root})
				break
			}
		}
	}
	return providers
}

func ResolveConfiguredWordlist(profile string) (WordlistSelection, error) {
	config, err := LoadConfig()
	if err != nil {
		return WordlistSelection{}, err
	}
	if config.WordlistProviders == "" {
		return WordlistSelection{}, fmt.Errorf("no wordlist provider configured; set %s", ConfigKeyWordlistProviders)
	}
	return resolveWordlist(profile, config.WordlistProviders, DiscoverWordlistProviders())
}

func resolveWordlist(profile, providerValue string, installed []WordlistProvider) (WordlistSelection, error) {
	profiles, ok := wordlistProfileCandidates[profile]
	if !ok {
		return WordlistSelection{}, fmt.Errorf("unsupported wordlist profile: %s", profile)
	}
	providers, err := NormalizeWordlistProviders(providerValue)
	if err != nil {
		return WordlistSelection{}, err
	}

	roots := make(map[string]string)
	for _, provider := range installed {
		if _, exists := roots[provider.Name]; !exists {
			roots[provider.Name] = provider.Root
		}
	}
	for _, provider := range providers {
		root, installed := roots[provider]
		if !installed {
			continue
		}
		for _, relative := range profiles[provider] {
			path := filepath.Join(root, relative)
			if isFile(path) {
				return WordlistSelection{Provider: provider, Profile: profile, Path: path}, nil
			}
		}
	}

	return WordlistSelection{}, fmt.Errorf("no wordlist found for profile %s in configured providers %s", profile, strings.Join(providers, ","))
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
