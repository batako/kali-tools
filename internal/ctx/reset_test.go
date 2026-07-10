package ctx

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResetHostsBlocksRemovesOnlyCtxBlocks(t *testing.T) {
	hostsPath := filepath.Join(t.TempDir(), "hosts")
	content := "127.0.0.1 localhost\n" +
		"\n# >>> ctx: first\n10.10.10.10 first.example\n# <<< ctx: first\n" +
		"\n192.0.2.10 unrelated.example\n" +
		"\n# >>> ctx: second\n10.10.20.20 second.example\n# <<< ctx: second\n"
	if err := os.WriteFile(hostsPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(hosts) error = %v", err)
	}

	ids, err := CtxHostsWorkspaceIDs(hostsPath)
	if err != nil {
		t.Fatalf("CtxHostsWorkspaceIDs() error = %v", err)
	}
	if len(ids) != 2 || ids[0] != "first" || ids[1] != "second" {
		t.Fatalf("ctx hosts ids = %v", ids)
	}
	if err := ResetHostsBlocks(hostsPath, ids); err != nil {
		t.Fatalf("ResetHostsBlocks() error = %v", err)
	}

	updated, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts) error = %v", err)
	}
	got := string(updated)
	for _, want := range []string{"127.0.0.1 localhost", "192.0.2.10 unrelated.example"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hosts = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "# >>> ctx:") || strings.Contains(got, "first.example") || strings.Contains(got, "second.example") {
		t.Fatalf("hosts = %q, ctx blocks remain", got)
	}
}

func TestResetCtxDataRemovesCtxTracesAndPreservesUserData(t *testing.T) {
	home := t.TempDir()
	ctxHome := filepath.Join(home, ".ctx")
	t.Setenv("HOME", home)
	t.Setenv("CTX_HOME", ctxHome)

	firstRoot := filepath.Join(t.TempDir(), "first")
	secondRoot := filepath.Join(t.TempDir(), "second")
	for _, root := range []string{firstRoot, secondRoot} {
		if err := os.Mkdir(root, 0755); err != nil {
			t.Fatalf("Mkdir(%s) error = %v", root, err)
		}
		if err := os.WriteFile(filepath.Join(root, "user-data.txt"), []byte("keep\n"), 0644); err != nil {
			t.Fatalf("WriteFile(user data) error = %v", err)
		}
		if _, err := InitWorkspace(root); err != nil {
			t.Fatalf("InitWorkspace(%s) error = %v", root, err)
		}
	}

	zshrc := "export KEEP_ZSH=1\n\n" + shellBlock("zsh")
	bashrc := "export KEEP_BASH=1\n\n" + shellBlock("bash")
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(zshrc), 0600); err != nil {
		t.Fatalf("WriteFile(.zshrc) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(bashrc), 0640); err != nil {
		t.Fatalf("WriteFile(.bashrc) error = %v", err)
	}
	historyPath := filepath.Join(home, ".zsh_history")
	if err := os.WriteFile(historyPath, []byte("ctx workspace init\n"), 0600); err != nil {
		t.Fatalf("WriteFile(history) error = %v", err)
	}
	unrelatedCtxHomeFile := filepath.Join(ctxHome, "keep.txt")
	if err := os.WriteFile(unrelatedCtxHomeFile, []byte("keep\n"), 0644); err != nil {
		t.Fatalf("WriteFile(ctx home unrelated) error = %v", err)
	}

	records, err := ListWorkspaceRecords()
	if err != nil {
		t.Fatalf("ListWorkspaceRecords() error = %v", err)
	}
	if err := ResetCtxData(records); err != nil {
		t.Fatalf("ResetCtxData() error = %v", err)
	}

	for _, root := range []string{firstRoot, secondRoot} {
		if _, err := os.Stat(root); err != nil {
			t.Fatalf("workspace root %s was removed: %v", root, err)
		}
		if _, err := os.Stat(filepath.Join(root, "user-data.txt")); err != nil {
			t.Fatalf("user data in %s was removed: %v", root, err)
		}
		if _, err := os.Stat(filepath.Join(root, MarkerFile)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("workspace marker in %s still exists: %v", root, err)
		}
	}
	for path, want := range map[string]string{
		filepath.Join(home, ".zshrc"):  "export KEEP_ZSH=1",
		filepath.Join(home, ".bashrc"): "export KEEP_BASH=1",
		historyPath:                    "ctx workspace init",
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if !strings.Contains(string(content), want) {
			t.Fatalf("%s = %q, want %q", path, content, want)
		}
		if path != historyPath && strings.Contains(string(content), shellBlockStart) {
			t.Fatalf("%s still contains ctx shell block: %q", path, content)
		}
	}
	for _, path := range []string{
		ctxHome,
		filepath.Join(ctxHome, "db.sqlite"),
		filepath.Join(ctxHome, "workspaces"),
		unrelatedCtxHomeFile,
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("ctx data %s still exists: %v", path, err)
		}
	}
}

func TestResetCtxDataRemovesMarkerWhenDatabaseContainsStaleIDs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	root := t.TempDir()
	workspace, err := InitWorkspace(root)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	err = ResetCtxData([]WorkspaceRecord{
		{ID: "stale-id-1", RootPath: root},
		{ID: "stale-id-2", RootPath: root},
		{ID: workspace.ID, RootPath: root},
	})
	if err != nil {
		t.Fatalf("ResetCtxData() error = %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("workspace root was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, MarkerFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace marker still exists: %v", err)
	}
}

func TestRunResetPermissionDeniedElevatesOnlyHostsCleanup(t *testing.T) {
	hostsPath := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hostsPath, []byte("# >>> ctx: orphan\n10.0.0.1 host\n# <<< ctx: orphan\n"), 0644); err != nil {
		t.Fatalf("WriteFile(hosts) error = %v", err)
	}

	oldHostsPath := hostsFilePath
	oldResetHosts := resetHostsBlocksFunc
	oldReexec := reexecResetHostsWithSudoFunc
	oldResetData := resetCtxDataFunc
	hostsFilePath = hostsPath
	resetHostsBlocksFunc = func(string, []string) error { return os.ErrPermission }
	var elevatedIDs []string
	reexecResetHostsWithSudoFunc = func(ids []string, _ io.Writer) error {
		elevatedIDs = append([]string(nil), ids...)
		return nil
	}
	dataReset := false
	resetCtxDataFunc = func([]WorkspaceRecord) error {
		dataReset = true
		return nil
	}
	t.Cleanup(func() {
		hostsFilePath = oldHostsPath
		resetHostsBlocksFunc = oldResetHosts
		reexecResetHostsWithSudoFunc = oldReexec
		resetCtxDataFunc = oldResetData
	})
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	var out bytes.Buffer
	if err := Run([]string{"ctx", "reset", "--yes"}, &out); err != nil {
		t.Fatalf("Run(ctx reset --yes) error = %v", err)
	}
	if len(elevatedIDs) != 1 || elevatedIDs[0] != "orphan" {
		t.Fatalf("elevated workspace ids = %v", elevatedIDs)
	}
	if !dataReset {
		t.Fatal("ctx data reset was not called after hosts cleanup")
	}
	for _, want := range []string{"Need administrator privileges", "sudo", "ctx data and configuration removed"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("reset output = %q, want %q", out.String(), want)
		}
	}
}

func TestRunResetShortYesSkipsConfirmation(t *testing.T) {
	oldHostsPath := hostsFilePath
	oldResetHosts := resetHostsBlocksFunc
	oldResetData := resetCtxDataFunc
	hostsFilePath = filepath.Join(t.TempDir(), "hosts")
	resetHostsBlocksFunc = func(string, []string) error { return nil }
	resetCtxDataFunc = func([]WorkspaceRecord) error { return nil }
	t.Cleanup(func() {
		hostsFilePath = oldHostsPath
		resetHostsBlocksFunc = oldResetHosts
		resetCtxDataFunc = oldResetData
	})
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	setWorkspaceInputForTest(t, "n\n")

	var out bytes.Buffer
	if err := Run([]string{"ctx", "reset", "-y"}, &out); err != nil {
		t.Fatalf("Run(ctx reset -y) error = %v", err)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("output = %q, -y should skip confirmation", out.String())
	}
}
