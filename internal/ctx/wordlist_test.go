package ctx

import (
	"testing"
)

func TestClassifyWordlistByPurpose(t *testing.T) {
	tests := map[string]string{
		"/usr/share/wordlists/dirb/common.txt":                                   WordlistTypeDirectory,
		"/usr/share/wordlists/seclists/Discovery/Web-Content/CGIs/test.fuzz.txt": WordlistTypeEndpoint,
		"/usr/share/wordlists/seclists/Fuzzing/LFI/lfi.txt":                      WordlistTypeParameter,
		"/usr/share/wordlists/rockyou.txt":                                       WordlistTypePassword,
		"/usr/share/wordlists/custom.txt":                                        WordlistTypeUnknown,
	}
	for path, want := range tests {
		if got := classifyWordlist(path); got != want {
			t.Errorf("classifyWordlist(%q) = %q, want %q", path, got, want)
		}
	}
}
