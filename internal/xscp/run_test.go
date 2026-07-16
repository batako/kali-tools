package xscp

import "testing"

func TestParseOptions(t *testing.T) {
	options, err := parseOptions([]string{"upload", "./local.txt", "--port", "2222", "--service", "2"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if options.Action != "upload" || options.Source != "./local.txt" || options.Destination != "local.txt" || options.Port != "2222" || options.Service != "2" {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseOptionsUsesExplicitDestination(t *testing.T) {
	options, err := parseOptions([]string{"download", "/tmp/remote.txt", "./copy.txt", "root"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if options.Destination != "./copy.txt" || options.Credential != "root" {
		t.Fatalf("options = %+v", options)
	}
}

func TestSCPArgsUpload(t *testing.T) {
	password := "secret"
	credential := &Credential{Username: "kali", Password: &password}
	got := (&App{}).scpArgs("upload", "./local.txt", "/tmp/remote.txt", credential, "10.0.0.5", 2222)
	want := []string{"-P", "2222", "./local.txt", "kali@10.0.0.5:/tmp/remote.txt"}
	if len(got) != len(want) {
		t.Fatalf("scpArgs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scpArgs() = %#v, want %#v", got, want)
		}
	}
}

func TestSCPArgsDownloadWithoutCredential(t *testing.T) {
	got := (&App{}).scpArgs("download", "./local.txt", "/tmp/remote.txt", nil, "10.0.0.5", 22)
	want := []string{"-P", "22", "10.0.0.5:/tmp/remote.txt", "./local.txt"}
	if len(got) != len(want) {
		t.Fatalf("scpArgs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scpArgs() = %#v, want %#v", got, want)
		}
	}
}

func TestSCPLogCommandDoesNotIncludePassword(t *testing.T) {
	password := "secret"
	command := scpLogCommand("upload", "./local.txt", "/tmp/remote.txt", &Credential{Username: "kali", Password: &password}, "10.0.0.5", 22)
	if command != "scp -P 22 ./local.txt kali@10.0.0.5:/tmp/remote.txt" {
		t.Fatalf("scpLogCommand() = %q", command)
	}
	if command == "secret" {
		t.Fatalf("scpLogCommand() leaked password")
	}
}
