package ctx

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValueProjectRoot(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	chdirForTest(t, base)

	want := filepath.Join(base, "cases")
	got, err := SetConfigValue(ConfigKeyProjectRoot, "cases")
	if err != nil {
		t.Fatalf("SetConfigValue(project.root) error = %v", err)
	}
	if got != want {
		t.Fatalf("SetConfigValue(project.root) = %q, want %q", got, want)
	}

	got, err = GetConfigValue(ConfigKeyProjectRoot)
	if err != nil {
		t.Fatalf("GetConfigValue(project.root) error = %v", err)
	}
	if got != want {
		t.Fatalf("GetConfigValue(project.root) = %q, want %q", got, want)
	}

	entries, err := ListConfigValues()
	if err != nil {
		t.Fatalf("ListConfigValues() error = %v", err)
	}
	if len(entries) != 2 || entries[0].Key != ConfigKeyProjectRoot || entries[0].Value != want ||
		entries[1].Key != ConfigKeyWebWordlist || entries[1].Value != "" {
		t.Fatalf("ListConfigValues() = %+v, want project.root and empty web.wordlist", entries)
	}
}

func TestConfigValueWebWordlist(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	chdirForTest(t, base)

	want := filepath.Join(base, "wordlists", "web.txt")
	got, err := SetConfigValue(ConfigKeyWebWordlist, "wordlists/web.txt")
	if err != nil {
		t.Fatalf("SetConfigValue(web.wordlist) error = %v", err)
	}
	if got != want {
		t.Fatalf("SetConfigValue(web.wordlist) = %q, want %q", got, want)
	}

	got, err = GetConfigValue(ConfigKeyWebWordlist)
	if err != nil {
		t.Fatalf("GetConfigValue(web.wordlist) error = %v", err)
	}
	if got != want {
		t.Fatalf("GetConfigValue(web.wordlist) = %q, want %q", got, want)
	}
}

func TestConfigValueRejectsUnknownKey(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	if _, err := GetConfigValue("project.unknown"); err == nil {
		t.Fatal("GetConfigValue(project.unknown) error = nil, want error")
	}
	if _, err := SetConfigValue("project.unknown", "value"); err == nil {
		t.Fatal("SetConfigValue(project.unknown) error = nil, want error")
	}
}

func TestRunConfigGetSetProjectRoot(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	chdirForTest(t, base)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "config", "set", ConfigKeyProjectRoot, "cases"}, &out); err != nil {
		t.Fatalf("Run(ctx config set project.root cases) error = %v", err)
	}
	want := filepath.Join(base, "cases")
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("set output = %q, want %q", got, want)
	}

	out.Reset()
	if err := Run([]string{"ctx", "config", "get", ConfigKeyProjectRoot}, &out); err != nil {
		t.Fatalf("Run(ctx config get project.root) error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("get output = %q, want %q", got, want)
	}

	for _, args := range [][]string{
		{"ctx", "config"},
		{"ctx", "config", "ls"},
	} {
		out.Reset()
		if err := Run(args, &out); err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		text := out.String()
		for _, wantText := range []string{"KEY", "VALUE", ConfigKeyProjectRoot, want} {
			if !strings.Contains(text, wantText) {
				t.Fatalf("Run(%v) output = %q, want %q", args, text, wantText)
			}
		}
	}
}
