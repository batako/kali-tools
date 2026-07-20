package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	internalctx "req/internal/ctx"
)

func main() {
	root := flag.String("root", "/usr/share/wordlists", "wordlists root to catalog")
	output := flag.String("output", "internal/ctx/wordlists_manifest.json", "manifest output path")
	flag.Parse()
	packages := installedPackages([]string{"wordlists", "seclists", "dirb", "dirbuster", "john", "metasploit-framework", "nmap", "sqlmap", "wfuzz"})
	manifest, err := internalctx.GenerateWordlistManifest(*root, packages)
	if err != nil {
		fatal(err)
	}
	data, err := internalctx.MarshalWordlistManifest(manifest)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		fatal(err)
	}
}

func installedPackages(names []string) map[string]string {
	result := map[string]string{}
	for _, name := range names {
		output, err := exec.Command("dpkg-query", "-W", "-f=${Version}", name).Output()
		if err == nil {
			result[name] = strings.TrimSpace(string(output))
		}
	}
	return result
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
