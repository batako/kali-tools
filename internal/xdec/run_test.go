package xdec

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAndExtractLiteralHash(t *testing.T) {
	doc, err := readDocument(options{}, []string{"admin:5f4dcc3b5aa765d61d8327deb882cf99"}, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	got := extractCandidates(doc, options{scope: "ssh"})
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want 1", len(got))
	}
	if got[0].Username != "admin" || got[0].Value != "5f4dcc3b5aa765d61d8327deb882cf99" || got[0].Scope != "ssh" {
		t.Fatalf("candidate = %#v", got[0])
	}
}

func TestExtractsHashesFromCommandOutput(t *testing.T) {
	doc := document{Source: "stdin", Raw: []byte("[*] dump\nadmin 5f4dcc3b5aa765d61d8327deb882cf99\nguest:7c222fb2927d828af22f592134e8932480637c0d\n")}
	got := extractCandidates(doc, options{})
	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2: %#v", len(got), got)
	}
	if got[0].Username != "admin" || got[0].Value != "5f4dcc3b5aa765d61d8327deb882cf99" {
		t.Fatalf("first = %#v", got[0])
	}
	if got[1].Username != "guest" || got[1].Value != "7c222fb2927d828af22f592134e8932480637c0d" {
		t.Fatalf("second = %#v", got[1])
	}
}

func TestClassifyAmbiguousMD5(t *testing.T) {
	got := classify("5f4dcc3b5aa765d61d8327deb882cf99")
	if len(got) != 3 || got[0].Kind != "md5" || got[1].Kind != "ntlm" || got[2].Kind != "md4" {
		t.Fatalf("classifications = %#v", got)
	}
}

func TestDecodeInstantBase64AndHex(t *testing.T) {
	if got, kind, ok := decodeInstant("cGFzc3dvcmQ="); !ok || kind != "base64" || got != "password" {
		t.Fatalf("base64 = %q, %q, %v", got, kind, ok)
	}
	if got, kind, ok := decodeInstant("70617373776f7264"); !ok || kind != "hex" || got != "password" {
		t.Fatalf("hex = %q, %q, %v", got, kind, ok)
	}
}

func TestExplicitStringInput(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"xdec", "--string", "cGFzc3dvcmQ="}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "password\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if err := Run([]string{"xdec", "--string", "cGFzc3dvcmQ=", "extra"}, strings.NewReader(""), &out, &out); err == nil {
		t.Fatal("--string with positional input should be rejected")
	}

	var positional bytes.Buffer
	if err := Run([]string{"xdec", "cGFzc3dvcmQ="}, strings.NewReader(""), &positional, &positional); err != nil {
		t.Fatal(err)
	}
	if positional.String() != out.String() {
		t.Fatalf("explicit output = %q, positional output = %q", out.String(), positional.String())
	}
}

func TestRedactArgs(t *testing.T) {
	got := redactArgs([]string{"-f", "hashes.txt", "5f4dcc3b5aa765d61d8327deb882cf99", "--username", "admin"})
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "5f4dcc") || strings.Contains(joined, "admin") || !bytes.Contains([]byte(joined), []byte("<input>")) {
		t.Fatalf("redacted args = %q", joined)
	}
}

func TestWriteResultsIncludesIdentity(t *testing.T) {
	var out bytes.Buffer
	err := writeResults(&out, []result{{Candidate: candidate{Username: "admin", Scope: "ssh"}, Value: "password", Status: "cracked"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "admin: password\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestExtractSSH2JohnOutput(t *testing.T) {
	doc := document{
		Source:   "file",
		Name:     "id_rsa",
		Raw:      []byte("-----BEGIN OPENSSH PRIVATE KEY-----"),
		Analysis: []byte("/tmp/id_rsa:$sshng$1$16$fixture"),
		Kind:     "ssh",
	}
	got := extractCandidates(doc, options{})
	if len(got) != 1 || got[0].Username != "" || got[0].Value != "$sshng$1$16$fixture" {
		t.Fatalf("candidates = %#v", got)
	}
	if got := classify("$sshng$1$16$fixture"); len(got) != 1 || got[0].Kind != "ssh" {
		t.Fatalf("classification = %#v", got)
	}
}

func TestPositionalFileInputAndTrailingFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hashes.txt")
	if err := os.WriteFile(path, []byte("5f4dcc3b5aa765d61d8327deb882cf99\n"), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := readDocument(options{}, []string{path}, strings.NewReader(""))
	if err != nil || doc.Source != "file" || string(doc.Raw) == "" {
		t.Fatalf("document = %#v, err = %v", doc, err)
	}
	got := reorderFlags([]string{path, "--refresh", "-w", "words.txt"})
	if strings.Join(got, " ") != "--refresh -w words.txt "+path {
		t.Fatalf("reordered args = %#v", got)
	}

	explicit, err := readDocument(options{file: path}, nil, strings.NewReader(""))
	if err != nil || string(explicit.Raw) != string(doc.Raw) {
		t.Fatalf("explicit document = %#v, err = %v", explicit, err)
	}

	if err := Run([]string{"xdec", "--file", path, "extra"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("--file with positional input should be rejected")
	}
	if err := Run([]string{"xdec", "first", "second"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("multiple positional inputs should be rejected")
	}
}

func TestOnlineHelp(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"xdec", "--online-help"}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "docs/commands/xdec.md") || !strings.Contains(out.String(), "xdec 1.0.0") {
		t.Fatalf("online help output = %q", out.String())
	}
}

func TestRootAndDecodeHelp(t *testing.T) {
	for _, test := range []struct {
		args  []string
		want  string
		avoid string
	}{
		{[]string{"xdec"}, "usage: xdec [options] [FILE_OR_STRING]", "usage: xdec decode"},
		{[]string{"xdec", "decode"}, "usage: xdec decode", "usage: xdec <subcommand>"},
		{[]string{"xdec", "decode", "--help"}, "usage: xdec decode", ""},
	} {
		var out bytes.Buffer
		if err := Run(test.args, strings.NewReader(""), &out, &out); err != nil {
			t.Fatalf("Run(%v): %v", test.args, err)
		}
		if !strings.Contains(out.String(), test.want) || (test.avoid != "" && strings.Contains(out.String(), test.avoid)) {
			t.Fatalf("Run(%v) output = %q", test.args, out.String())
		}
	}
	if err := Run([]string{"xdec", "decode", "--online-help"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("decode --online-help should be rejected")
	}
	if err := Run([]string{"xdec", "decode", "--version"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
		t.Fatal("decode --version should be rejected")
	}
}

func TestDecodeAndRecoverModesStaySeparate(t *testing.T) {
	const hash = "5f4dcc3b5aa765d61d8327deb882cf99"

	var decoded bytes.Buffer
	if err := Run([]string{"xdec", "decode", hash}, strings.NewReader(""), &decoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if got := decoded.String(); !strings.Contains(got, "recover-required") {
		t.Fatalf("decode output = %q, want recover-required", got)
	}
	if strings.Contains(decoded.String(), "password recovery may take a long time") {
		t.Fatalf("decode unexpectedly started recovery: %q", decoded.String())
	}

	var recovered bytes.Buffer
	if err := Run([]string{"xdec", "recover", "cGFzc3dvcmQ="}, strings.NewReader(""), &recovered, &recovered); err != nil {
		t.Fatal(err)
	}
	if got := recovered.String(); !strings.Contains(got, "not-recoverable") {
		t.Fatalf("recover output = %q, want not-recoverable", got)
	}
}

func TestRecoverHelp(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"xdec", "recover", "--help"}, strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "usage: xdec recover") || !strings.Contains(out.String(), "passwords and key passphrases") {
		t.Fatalf("recover help = %q", out.String())
	}
}

func TestHelpAndVersionSubcommandsMatchFlags(t *testing.T) {
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := Run(append([]string{"xdec"}, args...), strings.NewReader(""), &out, &out); err != nil {
			t.Fatalf("Run(%v): %v", args, err)
		}
		return out.String()
	}

	if got, want := run("help"), run("--help"); got != want {
		t.Fatalf("help output = %q, --help output = %q", got, want)
	}
	if got, want := run("-h"), run("--help"); got != want {
		t.Fatalf("-h output = %q, --help output = %q", got, want)
	}
	if got, want := run("version"), run("--version"); got != want {
		t.Fatalf("version output = %q, --version output = %q", got, want)
	}
	if got, want := run("-V"), run("--version"); got != want {
		t.Fatalf("-V output = %q, --version output = %q", got, want)
	}
	if got, want := run("help", "decode"), run("decode", "--help"); got != want {
		t.Fatalf("help decode output = %q, decode --help output = %q", got, want)
	}
	if got, want := run("decode", "-h"), run("decode", "--help"); got != want {
		t.Fatalf("decode -h output = %q, decode --help output = %q", got, want)
	}
	if got := run("help", "help"); !strings.Contains(got, "usage: xdec help") {
		t.Fatalf("help help output = %q", got)
	}
	if got := run("version", "--help"); !strings.Contains(got, "usage: xdec version") {
		t.Fatalf("version --help output = %q", got)
	}
}
