package xwebshell

import (
	"bufio"
	"strings"
	"testing"
)

func TestCoveredByCatalog(t *testing.T) {
	known := map[string]catalogEntry{
		"seclists/FuzzDB":      {Path: "seclists/FuzzDB", Group: true},
		"php/php-backdoor.php": {Path: "php/php-backdoor.php"},
	}
	if !coveredByCatalog("seclists/FuzzDB/cmd.php", known) {
		t.Fatal("grouped catalog entry should cover child files")
	}
	if coveredByCatalog("php/new.php", known) {
		t.Fatal("unregistered file should be reported as new")
	}
}

func TestRunHelp(t *testing.T) {
	var output strings.Builder
	if err := Run([]string{"xwebshell", "--help"}, &output, &strings.Builder{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(output.String(), "usage: xwebshell [ls|show <ID>|export <ID>]") {
		t.Fatalf("help output = %q", output.String())
	}
	if !strings.Contains(output.String(), "-h, --help") {
		t.Fatalf("help output = %q", output.String())
	}
}

func TestConfigureTemplate(t *testing.T) {
	content := []byte("my $ip = '127.0.0.1';\nmy $port = 1234;\n")
	var prompts strings.Builder
	configured, err := configureTemplate("perl/perl-reverse-shell.pl", content, bufio.NewReader(strings.NewReader("4444\n")), &prompts)
	if err != nil {
		t.Fatalf("configureTemplate() error = %v", err)
	}
	expectedIP := detectCallbackIP()
	if expectedIP == "" {
		expectedIP = "127.0.0.1"
	}
	expected := "my $ip = '" + expectedIP + "';\nmy $port = 4444;\n"
	if got := string(configured); got != expected {
		t.Fatalf("configured content = %q", got)
	}
}

func TestConfigureMagento(t *testing.T) {
	content := []byte("#define('USERNAME','old');\n#define('EMAIL','old@example.com');\n#define('PASSWORD','oldpass');\n")
	configured, err := configureTemplate("seclists/Magento/newadmin-Inchoo.php", content, bufio.NewReader(strings.NewReader("newuser\nnew@example.com\nnewpass\n")), &strings.Builder{})
	if err != nil {
		t.Fatalf("configureTemplate() error = %v", err)
	}
	if got := string(configured); got != "define('USERNAME','newuser');\ndefine('EMAIL','new@example.com');\ndefine('PASSWORD','newpass');\n" {
		t.Fatalf("configured content = %q", got)
	}
}
