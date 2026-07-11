package ctx

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestRunCredentialSetListUpdateAndScopeFilter(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "credential", "set", "ssh", "root", "toor"}, &out); err != nil {
		t.Fatalf("Run(credential set) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "ssh root toor") {
		t.Fatalf("set output = %q, want saved credential", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "credential", "add", "wordpress", "root", "password123"}, &out); err != nil {
		t.Fatalf("Run(credential add) error = %v", err)
	}

	out.Reset()
	if err := Run([]string{"ctx", "credential"}, &out); err != nil {
		t.Fatalf("Run(credential default view) error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"ID", "Scope", "Username", "Password", "ssh", "root", "toor", "wordpress", "password123"} {
		if !strings.Contains(got, want) {
			t.Fatalf("credential list output = %q, want %q", got, want)
		}
	}

	out.Reset()
	if err := Run([]string{"ctx", "credential", "ls", "ssh"}, &out); err != nil {
		t.Fatalf("Run(credential ls scope) error = %v", err)
	}
	got = out.String()
	if !strings.Contains(got, "toor") || strings.Contains(got, "password123") {
		t.Fatalf("scoped credential list output = %q, want only ssh credential", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "credential", "update", "ssh", "root", "newpass"}, &out); err != nil {
		t.Fatalf("Run(credential update) error = %v", err)
	}
	credentials, err := ListCredentials(workspace, "ssh")
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(credentials) != 1 || credentials[0].Password != "newpass" {
		t.Fatalf("credentials = %+v, want updated password", credentials)
	}
}

func TestRunCredentialShorthandMatchesSet(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "credential", "ssh", "hoge", "password"}, &out); err != nil {
		t.Fatalf("Run(credential shorthand) error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "ssh hoge password") {
		t.Fatalf("shorthand output = %q, want saved credential", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "credential", "ssh", "hoge", "updated"}, &out); err != nil {
		t.Fatalf("Run(credential shorthand update) error = %v", err)
	}
	credentials, err := ListCredentials(workspace, "ssh")
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(credentials) != 1 || credentials[0].Username != "hoge" || credentials[0].Password != "updated" {
		t.Fatalf("credentials = %+v, want shorthand to update via set", credentials)
	}
}

func TestRunCredentialAddDuplicateShowsFriendlyError(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddCredential(workspace, "ssh", "hoge", "password"); err != nil {
		t.Fatalf("AddCredential() error = %v", err)
	}

	var out bytes.Buffer
	err := Run([]string{"ctx", "credential", "add", "ssh", "hoge", "password"}, &out)
	if err == nil {
		t.Fatal("Run(duplicate credential add) error = nil, want error")
	}
	message := err.Error()
	if !strings.Contains(message, "credential already exists: ssh hoge") {
		t.Fatalf("error = %q, want friendly duplicate message", message)
	}
	if strings.Contains(message, "UNIQUE constraint failed") || strings.Contains(message, "constraint failed") {
		t.Fatalf("error = %q, should not expose sqlite constraint details", message)
	}
}

func TestRunCredentialRemoveByIDSkipsConfirmationWithYes(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	credential, err := AddCredential(workspace, "ssh", "root", "toor")
	if err != nil {
		t.Fatalf("AddCredential() error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "credential", "rm", "-y", strconv.FormatInt(credential.ID, 10)}, &out); err != nil {
		t.Fatalf("Run(credential rm -y id) error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Removed credential:", "Scope:    ssh", "Username: root", "Password: toor"} {
		if !strings.Contains(got, want) {
			t.Fatalf("remove output = %q, want %q", got, want)
		}
	}

	if _, err := GetCredentialByID(workspace, credential.ID); err == nil {
		t.Fatal("credential still exists after removal")
	}
}

func TestRunCredentialRemoveByUsernameSelectsCandidateAndConfirms(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddCredential(workspace, "ssh", "root", "toor"); err != nil {
		t.Fatalf("AddCredential(ssh) error = %v", err)
	}
	if _, err := AddCredential(workspace, "wordpress", "root", "password123"); err != nil {
		t.Fatalf("AddCredential(wordpress) error = %v", err)
	}

	oldStdin := workspaceStdin
	workspaceStdin = strings.NewReader("2\ny\n")
	t.Cleanup(func() { workspaceStdin = oldStdin })

	var out bytes.Buffer
	if err := Run([]string{"ctx", "credential", "rm", "root"}, &out); err != nil {
		t.Fatalf("Run(credential rm username) error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"1) [",
		"ssh",
		"2) [",
		"wordpress",
		"Remove credential?",
		"Removed credential:",
		"Password: password123",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("remove selection output = %q, want %q", got, want)
		}
	}

	credentials, err := ListCredentials(workspace, "")
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(credentials) != 1 || credentials[0].Scope != "ssh" {
		t.Fatalf("credentials after selected remove = %+v, want only ssh credential", credentials)
	}
}

func TestRunCredentialRemoveConfirmationCancel(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddCredential(workspace, "ssh", "root", "toor"); err != nil {
		t.Fatalf("AddCredential() error = %v", err)
	}

	oldStdin := workspaceStdin
	workspaceStdin = strings.NewReader("\n")
	t.Cleanup(func() { workspaceStdin = oldStdin })

	var out bytes.Buffer
	if err := Run([]string{"ctx", "credential", "rm", "ssh", "root"}, &out); err != nil {
		t.Fatalf("Run(credential rm cancel) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Remove credential?") || !strings.Contains(got, "cancelled") {
		t.Fatalf("cancel output = %q, want confirmation and cancelled", got)
	}
	credentials, err := ListCredentials(workspace, "")
	if err != nil {
		t.Fatalf("ListCredentials() error = %v", err)
	}
	if len(credentials) != 1 {
		t.Fatalf("credentials len after cancel = %d, want 1", len(credentials))
	}
}
