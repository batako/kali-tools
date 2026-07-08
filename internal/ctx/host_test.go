package ctx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestHostLifecycle(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddTarget(workspace, "10.10.10.20", "dc"); err != nil {
		t.Fatalf("AddTarget() error = %v", err)
	}

	host, err := AddHost(workspace, "Example.THM", "")
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if host.Hostname != "example.thm" || host.TargetName != "default" || host.TargetIP != "10.10.10.10" {
		t.Fatalf("host = %+v, want example.thm on default", host)
	}

	dcHost, err := AddHost(workspace, "dc01.example.local", "dc")
	if err != nil {
		t.Fatalf("AddHost(target) error = %v", err)
	}
	if dcHost.TargetName != "dc" || dcHost.TargetIP != "10.10.10.20" {
		t.Fatalf("dc host = %+v, want dc target", dcHost)
	}

	hosts, err := ListHosts(workspace)
	if err != nil {
		t.Fatalf("ListHosts() error = %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts len = %d, want 2", len(hosts))
	}

	if err := RemoveHost(workspace, "example.thm"); err != nil {
		t.Fatalf("RemoveHost() error = %v", err)
	}
	hosts, err = ListHosts(workspace)
	if err != nil {
		t.Fatalf("ListHosts() after remove error = %v", err)
	}
	if len(hosts) != 1 || hosts[0].Hostname != "dc01.example.local" {
		t.Fatalf("hosts after remove = %+v, want dc01.example.local only", hosts)
	}
}

func TestRunHostCommands(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "init"}, &out); err != nil {
		t.Fatalf("Run(init) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "target", "set", "10.10.10.10"}, &out); err != nil {
		t.Fatalf("Run(target set) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "target", "add", "10.10.10.20", "--name", "dc"}, &out); err != nil {
		t.Fatalf("Run(target add) error = %v", err)
	}

	out.Reset()
	if err := Run([]string{"ctx", "host", "add", "example.thm"}, &out); err != nil {
		t.Fatalf("Run(host add) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "example.thm default 10.10.10.10") {
		t.Fatalf("host add output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "host", "add", "dc01.example.local", "--target", "dc"}, &out); err != nil {
		t.Fatalf("Run(host add --target) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "dc01.example.local dc 10.10.10.20") {
		t.Fatalf("host add --target output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "host", "ls"}, &out); err != nil {
		t.Fatalf("Run(host ls) error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "example.thm default 10.10.10.10") || !strings.Contains(got, "dc01.example.local dc 10.10.10.20") {
		t.Fatalf("host ls output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "host", "rm", "example.thm"}, &out); err != nil {
		t.Fatalf("Run(host rm) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "removed host: example.thm") {
		t.Fatalf("host rm output = %q", got)
	}
}

func TestAddHostRequiresPrimaryTarget(t *testing.T) {
	workspace := initTestWorkspace(t)

	_, err := AddHost(workspace, "example.thm", "")
	if err == nil {
		t.Fatal("AddHost() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "primary target not set") {
		t.Fatalf("error = %q, want primary target not set", err.Error())
	}
}

func TestAddHostRejectsInvalidHostname(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	_, err := AddHost(workspace, "bad host", "")
	if err == nil {
		t.Fatal("AddHost() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid hostname") {
		t.Fatalf("error = %q, want invalid hostname", err.Error())
	}
}

func TestRenderHostsBlockGroupsHostsByTarget(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddTarget(workspace, "10.10.10.20", "dc"); err != nil {
		t.Fatalf("AddTarget() error = %v", err)
	}
	if _, err := AddHost(workspace, "admin.example.thm", ""); err != nil {
		t.Fatalf("AddHost(admin) error = %v", err)
	}
	if _, err := AddHost(workspace, "example.thm", ""); err != nil {
		t.Fatalf("AddHost(example) error = %v", err)
	}
	if _, err := AddHost(workspace, "dc01.example.local", "dc"); err != nil {
		t.Fatalf("AddHost(dc01) error = %v", err)
	}

	block, err := RenderHostsBlock(workspace)
	if err != nil {
		t.Fatalf("RenderHostsBlock() error = %v", err)
	}
	want := "# >>> ctx: " + workspace.ID + "\n" +
		"10.10.10.10 admin.example.thm example.thm\n" +
		"10.10.10.20 dc01.example.local\n" +
		"# <<< ctx: " + workspace.ID + "\n"
	if block != want {
		t.Fatalf("hosts block = %q, want %q", block, want)
	}
}

func TestRunHostsShow(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "init"}, &out); err != nil {
		t.Fatalf("Run(init) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "target", "set", "10.10.10.10"}, &out); err != nil {
		t.Fatalf("Run(target set) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "host", "add", "example.thm"}, &out); err != nil {
		t.Fatalf("Run(host add) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "hosts", "show"}, &out); err != nil {
		t.Fatalf("Run(hosts show) error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "# >>> ctx: ") || !strings.Contains(got, "10.10.10.10 example.thm") || !strings.Contains(got, "# <<< ctx: ") {
		t.Fatalf("hosts show output = %q", got)
	}
}

func TestSyncHostsFileAddsAndReplacesWorkspaceBlock(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddHost(workspace, "example.thm", ""); err != nil {
		t.Fatalf("AddHost(example) error = %v", err)
	}

	hostsPath := t.TempDir() + "/hosts"
	initial := strings.Join([]string{
		"127.0.0.1 localhost",
		"# >>> ctx: ctx-other",
		"10.10.99.99 other.example",
		"# <<< ctx: ctx-other",
		"",
	}, "\n")
	if err := os.WriteFile(hostsPath, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile(initial hosts) error = %v", err)
	}

	if err := SyncHostsFile(workspace, hostsPath); err != nil {
		t.Fatalf("SyncHostsFile() error = %v", err)
	}
	content, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts) error = %v", err)
	}
	got := string(content)
	if !strings.Contains(got, "127.0.0.1 localhost\n") {
		t.Fatalf("hosts content lost original line: %q", got)
	}
	if !strings.Contains(got, "# >>> ctx: ctx-other\n10.10.99.99 other.example\n# <<< ctx: ctx-other") {
		t.Fatalf("hosts content lost other workspace block: %q", got)
	}
	if !strings.Contains(got, "# >>> ctx: "+workspace.ID+"\n10.10.10.10 example.thm\n# <<< ctx: "+workspace.ID) {
		t.Fatalf("hosts content missing ctx block: %q", got)
	}

	if _, err := AddHost(workspace, "admin.example.thm", ""); err != nil {
		t.Fatalf("AddHost(admin) error = %v", err)
	}
	if err := SyncHostsFile(workspace, hostsPath); err != nil {
		t.Fatalf("SyncHostsFile() replace error = %v", err)
	}
	content, err = os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts replaced) error = %v", err)
	}
	got = string(content)
	if strings.Count(got, "# >>> ctx: "+workspace.ID) != 1 {
		t.Fatalf("hosts content should contain one ctx block, got %q", got)
	}
	if !strings.Contains(got, "10.10.10.10 admin.example.thm example.thm") {
		t.Fatalf("hosts content did not replace ctx block: %q", got)
	}

	if err := RemoveHost(workspace, "example.thm"); err != nil {
		t.Fatalf("RemoveHost(example) error = %v", err)
	}
	if err := SyncHostsFile(workspace, hostsPath); err != nil {
		t.Fatalf("SyncHostsFile() after host removal error = %v", err)
	}
	content, err = os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts after removal) error = %v", err)
	}
	got = string(content)
	if strings.Contains(got, " admin.example.thm example.thm") || strings.Contains(got, " example.thm\n") {
		t.Fatalf("removed host remained after re-sync: %q", got)
	}
	if !strings.Contains(got, "10.10.10.10 admin.example.thm") {
		t.Fatalf("remaining host missing after re-sync: %q", got)
	}
}

func TestSyncHostsFileRejectsBrokenManagedBlock(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddHost(workspace, "example.thm", ""); err != nil {
		t.Fatalf("AddHost(example) error = %v", err)
	}

	hostsPath := t.TempDir() + "/hosts"
	broken := "127.0.0.1 localhost\n# >>> ctx: " + workspace.ID + "\n10.10.10.10 stale.example\n"
	if err := os.WriteFile(hostsPath, []byte(broken), 0644); err != nil {
		t.Fatalf("WriteFile(broken hosts) error = %v", err)
	}

	err := SyncHostsFile(workspace, hostsPath)
	if err == nil {
		t.Fatal("SyncHostsFile() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unterminated ctx hosts block") {
		t.Fatalf("error = %q, want unterminated ctx hosts block", err.Error())
	}
}

func TestRunHostsSyncWritesConfiguredHostsFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	hostsPath := t.TempDir() + "/hosts"
	oldHostsFilePath := hostsFilePath
	hostsFilePath = hostsPath
	t.Cleanup(func() { hostsFilePath = oldHostsFilePath })

	var out bytes.Buffer
	if err := Run([]string{"ctx", "init"}, &out); err != nil {
		t.Fatalf("Run(init) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "target", "set", "10.10.10.10"}, &out); err != nil {
		t.Fatalf("Run(target set) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "host", "add", "example.thm"}, &out); err != nil {
		t.Fatalf("Run(host add) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "hosts", "sync"}, &out); err != nil {
		t.Fatalf("Run(hosts sync) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "synced hosts") {
		t.Fatalf("hosts sync output = %q, want synced hosts", got)
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts) error = %v", err)
	}
	if got := string(content); !strings.Contains(got, "10.10.10.10 example.thm") {
		t.Fatalf("hosts content = %q, want example.thm mapping", got)
	}
}

func TestRunHostsSyncPermissionDeniedTriggersSudoReexec(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "init"}, &out); err != nil {
		t.Fatalf("Run(init) error = %v", err)
	}

	oldSyncHostsFileFunc := syncHostsFileFunc
	oldReexecHostsSyncWithSudoFunc := reexecHostsSyncWithSudoFunc
	syncHostsFileFunc = func(*Workspace, string) error {
		return &os.PathError{Op: "write", Path: "/etc/hosts", Err: os.ErrPermission}
	}
	called := false
	reexecHostsSyncWithSudoFunc = func(stdout io.Writer) error {
		called = true
		_, err := fmt.Fprintln(stdout, "sudo called")
		return err
	}
	t.Cleanup(func() {
		syncHostsFileFunc = oldSyncHostsFileFunc
		reexecHostsSyncWithSudoFunc = oldReexecHostsSyncWithSudoFunc
	})

	out.Reset()
	if err := Run([]string{"ctx", "hosts", "sync"}, &out); err != nil {
		t.Fatalf("Run(hosts sync) error = %v", err)
	}
	if !called {
		t.Fatal("sudo reexec was not called")
	}
	got := out.String()
	if !strings.Contains(got, "Need administrator privileges to update /etc/hosts.") {
		t.Fatalf("hosts sync output missing privilege message: %q", got)
	}
	if !strings.Contains(got, "Re-running hosts sync with sudo...") {
		t.Fatalf("hosts sync output missing sudo message: %q", got)
	}
}

func TestRunHostsSyncInternalDoesNotTriggerSudoReexec(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "init"}, &out); err != nil {
		t.Fatalf("Run(init) error = %v", err)
	}

	oldSyncHostsFileFunc := syncHostsFileFunc
	oldReexecHostsSyncWithSudoFunc := reexecHostsSyncWithSudoFunc
	syncHostsFileFunc = func(*Workspace, string) error {
		return &os.PathError{Op: "write", Path: "/etc/hosts", Err: os.ErrPermission}
	}
	reexecHostsSyncWithSudoFunc = func(io.Writer) error {
		t.Fatal("sudo reexec should not be called for --internal")
		return nil
	}
	t.Cleanup(func() {
		syncHostsFileFunc = oldSyncHostsFileFunc
		reexecHostsSyncWithSudoFunc = oldReexecHostsSyncWithSudoFunc
	})

	err := Run([]string{"ctx", "hosts", "sync", "--internal"}, &out)
	if err == nil {
		t.Fatal("Run(hosts sync --internal) error = nil, want permission error")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error = %v, want permission error", err)
	}
}

func TestRenderHostsBlockReturnsEmptyWhenNoHosts(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	block, err := RenderHostsBlock(workspace)
	if err != nil {
		t.Fatalf("RenderHostsBlock() error = %v", err)
	}
	if block != "" {
		t.Fatalf("hosts block = %q, want empty", block)
	}
}

func TestSyncHostsFileRemovesWorkspaceBlockWhenNoHostsRemain(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddHost(workspace, "example.thm", ""); err != nil {
		t.Fatalf("AddHost(example) error = %v", err)
	}

	hostsPath := t.TempDir() + "/hosts"
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644); err != nil {
		t.Fatalf("WriteFile(hosts) error = %v", err)
	}
	if err := SyncHostsFile(workspace, hostsPath); err != nil {
		t.Fatalf("SyncHostsFile() error = %v", err)
	}
	if err := RemoveHost(workspace, "example.thm"); err != nil {
		t.Fatalf("RemoveHost(example) error = %v", err)
	}
	if err := SyncHostsFile(workspace, hostsPath); err != nil {
		t.Fatalf("SyncHostsFile() after removing all hosts error = %v", err)
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts) error = %v", err)
	}
	got := string(content)
	if strings.Contains(got, "# >>> ctx: ") || strings.Contains(got, "# <<< ctx: ") {
		t.Fatalf("empty ctx block remained after re-sync: %q", got)
	}
	if got != "127.0.0.1 localhost\n" {
		t.Fatalf("hosts content = %q, want original line only", got)
	}
}

func TestSyncHostsFileWithNoHostsDoesNotAppendBlock(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	hostsPath := t.TempDir() + "/hosts"
	if err := os.WriteFile(hostsPath, []byte("127.0.0.1 localhost\n"), 0644); err != nil {
		t.Fatalf("WriteFile(hosts) error = %v", err)
	}
	if err := SyncHostsFile(workspace, hostsPath); err != nil {
		t.Fatalf("SyncHostsFile() error = %v", err)
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts) error = %v", err)
	}
	got := string(content)
	if strings.Contains(got, "# >>> ctx: ") || strings.Contains(got, "# <<< ctx: ") {
		t.Fatalf("ctx block was appended with no hosts: %q", got)
	}
	if got != "127.0.0.1 localhost\n" {
		t.Fatalf("hosts content = %q, want original line only", got)
	}
}

func TestCleanHostsFileRemovesOnlyWorkspaceBlock(t *testing.T) {
	workspace := initTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddHost(workspace, "example.thm", ""); err != nil {
		t.Fatalf("AddHost(example) error = %v", err)
	}

	hostsPath := t.TempDir() + "/hosts"
	initial := strings.Join([]string{
		"127.0.0.1 localhost",
		"# >>> ctx: ctx-other",
		"10.10.99.99 other.example",
		"# <<< ctx: ctx-other",
		"",
	}, "\n")
	if err := os.WriteFile(hostsPath, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile(initial hosts) error = %v", err)
	}
	if err := SyncHostsFile(workspace, hostsPath); err != nil {
		t.Fatalf("SyncHostsFile() error = %v", err)
	}
	if err := CleanHostsFile(workspace, hostsPath); err != nil {
		t.Fatalf("CleanHostsFile() error = %v", err)
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts) error = %v", err)
	}
	got := string(content)
	if strings.Contains(got, "# >>> ctx: "+workspace.ID) || strings.Contains(got, "example.thm") {
		t.Fatalf("workspace block remained after clean: %q", got)
	}
	if !strings.Contains(got, "127.0.0.1 localhost\n") {
		t.Fatalf("hosts content lost original line: %q", got)
	}
	if !strings.Contains(got, "# >>> ctx: ctx-other\n10.10.99.99 other.example\n# <<< ctx: ctx-other") {
		t.Fatalf("hosts content lost other workspace block: %q", got)
	}
}

func TestRunHostsCleanWritesConfiguredHostsFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	hostsPath := t.TempDir() + "/hosts"
	oldHostsFilePath := hostsFilePath
	hostsFilePath = hostsPath
	t.Cleanup(func() { hostsFilePath = oldHostsFilePath })

	var out bytes.Buffer
	if err := Run([]string{"ctx", "init"}, &out); err != nil {
		t.Fatalf("Run(init) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "target", "set", "10.10.10.10"}, &out); err != nil {
		t.Fatalf("Run(target set) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "host", "add", "example.thm"}, &out); err != nil {
		t.Fatalf("Run(host add) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "hosts", "sync"}, &out); err != nil {
		t.Fatalf("Run(hosts sync) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "hosts", "clean"}, &out); err != nil {
		t.Fatalf("Run(hosts clean) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "cleaned hosts") {
		t.Fatalf("hosts clean output = %q, want cleaned hosts", got)
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts) error = %v", err)
	}
	if got := string(content); strings.Contains(got, "# >>> ctx: ") || strings.Contains(got, "example.thm") {
		t.Fatalf("hosts content still has ctx block: %q", got)
	}
}

func TestRunHostsCleanPermissionDeniedTriggersSudoReexec(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "init"}, &out); err != nil {
		t.Fatalf("Run(init) error = %v", err)
	}

	oldCleanHostsFileFunc := cleanHostsFileFunc
	oldReexecHostsCleanWithSudoFunc := reexecHostsCleanWithSudoFunc
	cleanHostsFileFunc = func(*Workspace, string) error {
		return &os.PathError{Op: "write", Path: "/etc/hosts", Err: os.ErrPermission}
	}
	called := false
	reexecHostsCleanWithSudoFunc = func(stdout io.Writer) error {
		called = true
		_, err := fmt.Fprintln(stdout, "sudo clean called")
		return err
	}
	t.Cleanup(func() {
		cleanHostsFileFunc = oldCleanHostsFileFunc
		reexecHostsCleanWithSudoFunc = oldReexecHostsCleanWithSudoFunc
	})

	out.Reset()
	if err := Run([]string{"ctx", "hosts", "clean"}, &out); err != nil {
		t.Fatalf("Run(hosts clean) error = %v", err)
	}
	if !called {
		t.Fatal("sudo clean reexec was not called")
	}
	got := out.String()
	if !strings.Contains(got, "Need administrator privileges to update /etc/hosts.") {
		t.Fatalf("hosts clean output missing privilege message: %q", got)
	}
	if !strings.Contains(got, "Re-running hosts clean with sudo...") {
		t.Fatalf("hosts clean output missing sudo message: %q", got)
	}
}

func TestRunHostsCleanInternalDoesNotTriggerSudoReexec(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "init"}, &out); err != nil {
		t.Fatalf("Run(init) error = %v", err)
	}

	oldCleanHostsFileFunc := cleanHostsFileFunc
	oldReexecHostsCleanWithSudoFunc := reexecHostsCleanWithSudoFunc
	cleanHostsFileFunc = func(*Workspace, string) error {
		return &os.PathError{Op: "write", Path: "/etc/hosts", Err: os.ErrPermission}
	}
	reexecHostsCleanWithSudoFunc = func(io.Writer) error {
		t.Fatal("sudo clean reexec should not be called for --internal")
		return nil
	}
	t.Cleanup(func() {
		cleanHostsFileFunc = oldCleanHostsFileFunc
		reexecHostsCleanWithSudoFunc = oldReexecHostsCleanWithSudoFunc
	})

	err := Run([]string{"ctx", "hosts", "clean", "--internal"}, &out)
	if err == nil {
		t.Fatal("Run(hosts clean --internal) error = nil, want permission error")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("error = %v, want permission error", err)
	}
}
