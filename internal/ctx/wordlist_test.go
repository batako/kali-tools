package ctx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecommendWordlistsFromRootUsesCanonicalCatalogOrder(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"seclists/Discovery/DNS/subdomains-top1million-20000.txt": "www\napi\n",
		"seclists/Discovery/DNS/subdomains-top1million-5000.txt":  "www\n",
		"seclists/Discovery/DNS/tlds.txt":                         "com\nnet\n",
	}
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := recommendWordlistsFromRoot(root, WordlistKindSubdomain)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || filepath.Base(got[0].Path) != "subdomains-top1million-5000.txt" || filepath.Base(got[1].Path) != "subdomains-top1million-20000.txt" {
		t.Fatalf("recommendations = %+v", got)
	}
}

func TestRecommendWordlistsFromRootRejectsUnsupportedKinds(t *testing.T) {
	for _, kind := range []string{WordlistKindAll, WordlistKindUnknown, "missing"} {
		if _, err := recommendWordlistsFromRoot(t.TempDir(), kind); err == nil {
			t.Fatalf("kind %q was accepted", kind)
		}
	}
}
