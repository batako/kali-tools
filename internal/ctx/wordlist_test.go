package ctx

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeWordlistProviders(t *testing.T) {
	providers, err := NormalizeWordlistProviders("wordlists, seclists, wordlists")
	if err != nil {
		t.Fatalf("NormalizeWordlistProviders() error = %v", err)
	}
	if want := []string{"wordlists", "seclists"}; !reflect.DeepEqual(providers, want) {
		t.Fatalf("providers = %#v, want %#v", providers, want)
	}

	if _, err := NormalizeWordlistProviders("unknown"); err == nil {
		t.Fatal("NormalizeWordlistProviders(unknown) error = nil")
	}
}

func TestResolveWordlistUsesProviderOrderAndProfile(t *testing.T) {
	root := t.TempDir()
	seclistsRoot := filepath.Join(root, "seclists")
	wordlistsRoot := filepath.Join(root, "wordlists")
	if err := os.MkdirAll(filepath.Join(seclistsRoot, "Discovery", "Web-Content"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wordlistsRoot, "dirb"), 0755); err != nil {
		t.Fatal(err)
	}
	seclistsPath := filepath.Join(seclistsRoot, "Discovery", "Web-Content", "directory-list-2.3-small.txt")
	wordlistsPath := filepath.Join(wordlistsRoot, "dirb", "common.txt")
	if err := os.WriteFile(seclistsPath, []byte("seclists\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wordlistsPath, []byte("wordlists\n"), 0644); err != nil {
		t.Fatal(err)
	}

	selection, err := resolveWordlist(WordlistProfileWebStandard, "wordlists,seclists", []WordlistProvider{
		{Name: WordlistProviderSecLists, Root: seclistsRoot},
		{Name: WordlistProviderLists, Root: wordlistsRoot},
	})
	if err != nil {
		t.Fatalf("resolveWordlist() error = %v", err)
	}
	if selection.Provider != WordlistProviderLists || selection.Path != wordlistsPath {
		t.Fatalf("selection = %+v, want wordlists selection", selection)
	}

	selection, err = resolveWordlist(WordlistProfileWebStandard, "seclists,wordlists", []WordlistProvider{
		{Name: WordlistProviderSecLists, Root: seclistsRoot},
		{Name: WordlistProviderLists, Root: wordlistsRoot},
	})
	if err != nil {
		t.Fatalf("resolveWordlist() second error = %v", err)
	}
	if selection.Provider != WordlistProviderSecLists || selection.Path != seclistsPath {
		t.Fatalf("selection = %+v, want seclists selection", selection)
	}
}

func TestResolveConfiguredWordlistRequiresProviderOrExplicitPath(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	if _, err := ResolveConfiguredWordlist(WordlistProfileWebQuick); err == nil || !strings.Contains(err.Error(), ConfigKeyWordlistProviders) {
		t.Fatalf("ResolveConfiguredWordlist() error = %v, want provider configuration guidance", err)
	}
}
