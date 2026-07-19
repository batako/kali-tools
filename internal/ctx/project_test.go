package ctx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectRootStoresAbsolutePathAndCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	chdirForTest(t, base)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "root", "labs"}, &out); err != nil {
		t.Fatalf("Run(ctx project root labs) error = %v", err)
	}

	want := filepath.Join(base, "labs")
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("project root Stat() = %v, %v, want directory", info, err)
	}

	configRoot, err := GetConfigValue(ConfigKeyProjectRoot)
	if err != nil {
		t.Fatalf("GetConfigValue(project.root) error = %v", err)
	}
	if configRoot != want {
		t.Fatalf("config project root = %q, want %q", configRoot, want)
	}
}

func TestProjectRootExpandsHomeAndPrintsConfiguredPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "root", "~/labs"}, &out); err != nil {
		t.Fatalf("Run(ctx project root ~/labs) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "project", "root"}, &out); err != nil {
		t.Fatalf("Run(ctx project root) error = %v", err)
	}

	want := filepath.Join(home, "labs") + "\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestProjectRootUnsetMessages(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "root"}, &out); err != nil {
		t.Fatalf("Run(ctx project root) error = %v", err)
	}
	if !strings.Contains(out.String(), "No projects root configured.") ||
		!strings.Contains(out.String(), "ctx workspace init") {
		t.Fatalf("output = %q, want helpful unset-root message", out.String())
	}

	out.Reset()
	err := Run([]string{"ctx", "project", "new", "alpha"}, &out)
	if err == nil || !strings.Contains(err.Error(), "No projects root configured.") {
		t.Fatalf("Run(ctx project new alpha) error = %v, want unset-root error", err)
	}
}

func TestNamedProjectRootsSwitchAndKeepProjectsSeparated(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	thmPath := filepath.Join(base, "thm")
	htbPath := filepath.Join(base, "hackthebox")

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "root", "add", thmPath}, &out); err != nil {
		t.Fatalf("Run(ctx project root add thm) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "project", "root", "add", htbPath, "--name", "hackthebox"}, &out); err != nil {
		t.Fatalf("Run(ctx project root add hackthebox) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "project", "root", "add", filepath.Join(base, "duplicate"), "--name", "thm"}, &out); err == nil || !strings.Contains(err.Error(), "project root already exists: thm") {
		t.Fatalf("Run(ctx project root add duplicate thm) error = %v, want duplicate-name error", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "project", "root", "use", "thm"}, &out); err != nil {
		t.Fatalf("Run(ctx project root use thm) error = %v", err)
	}
	if _, err := CreateProject("room-one"); err != nil {
		t.Fatalf("CreateProject(room-one) error = %v", err)
	}
	if _, err := UseProjectRoot("hackthebox"); err != nil {
		t.Fatalf("UseProjectRoot(hackthebox) error = %v", err)
	}
	if _, err := CreateProject("machine-one"); err != nil {
		t.Fatalf("CreateProject(machine-one) error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(thmPath, "room-one", MarkerFile)); err != nil {
		t.Fatalf("THM project marker missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(htbPath, "machine-one", MarkerFile)); err != nil {
		t.Fatalf("Hack The Box project marker missing: %v", err)
	}
	projects, err := ListProjects()
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "machine-one" {
		t.Fatalf("ListProjects() = %+v, want only active-root project", projects)
	}

	out.Reset()
	if err := Run([]string{"ctx", "project", "root", "ls"}, &out); err != nil {
		t.Fatalf("Run(ctx project root ls) error = %v", err)
	}
	for _, want := range []string{"ACTIVE", "NAME", "PATH", "thm", "hackthebox", "*"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("root ls output = %q, want %q", out.String(), want)
		}
	}

	if _, err := RemoveProjectRoot("hackthebox"); err == nil || !strings.Contains(err.Error(), "cannot remove active") {
		t.Fatalf("RemoveProjectRoot(active) error = %v, want active-root error", err)
	}
	removed, err := RemoveProjectRoot("thm")
	if err != nil {
		t.Fatalf("RemoveProjectRoot(thm) error = %v", err)
	}
	if removed.Path != thmPath {
		t.Fatalf("RemoveProjectRoot(thm).Path = %q, want %q", removed.Path, thmPath)
	}
	if _, err := os.Stat(thmPath); err != nil {
		t.Fatalf("removed root directory was modified: %v", err)
	}
}

func TestProjectRootMoveMovesProjectsAndPreservesWorkspaceData(t *testing.T) {
	base := t.TempDir()
	ctxHome := filepath.Join(t.TempDir(), ".ctx")
	t.Setenv("CTX_HOME", ctxHome)
	sourcePath := filepath.Join(base, "thm")
	targetPath := filepath.Join(base, "hackthebox")
	if _, err := AddProjectRoot(sourcePath, "thm"); err != nil {
		t.Fatalf("AddProjectRoot(thm) error = %v", err)
	}
	if _, err := AddProjectRoot(targetPath, "hackthebox"); err != nil {
		t.Fatalf("AddProjectRoot(hackthebox) error = %v", err)
	}
	for _, name := range []string{"room-two", "room-one"} {
		if _, err := CreateProject(name); err != nil {
			t.Fatalf("CreateProject(%s) error = %v", name, err)
		}
	}
	recordsBefore, err := ListWorkspaceRecords()
	if err != nil {
		t.Fatalf("ListWorkspaceRecords() before move error = %v", err)
	}
	beforeByUUID := make(map[string]WorkspaceRecord, len(recordsBefore))
	for _, record := range recordsBefore {
		beforeByUUID[record.UUID] = record
		if _, err := os.Stat(filepath.Join(ctxHome, "workspaces", record.UUID)); err != nil {
			t.Fatalf("workspace data before move Stat() error = %v", err)
		}
	}
	chdirForTest(t, t.TempDir())

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "root", "move", "thm", "hackthebox", "--dry-run"}, &out); err != nil {
		t.Fatalf("Run(project root move --dry-run) error = %v", err)
	}
	if !strings.Contains(out.String(), "dry run; no changes made") {
		t.Fatalf("dry-run output = %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(sourcePath, "room-one")); err != nil {
		t.Fatalf("dry run changed source project: %v", err)
	}

	out.Reset()
	if err := Run([]string{"ctx", "project", "root", "move", "thm", "hackthebox", "--yes"}, &out); err != nil {
		t.Fatalf("Run(project root move --yes) error = %v", err)
	}
	if !strings.Contains(out.String(), "moved 2 project(s) from thm to hackthebox") {
		t.Fatalf("move output = %q", out.String())
	}
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.ActiveProjectRoot != "hackthebox" || config.ProjectRoot != targetPath {
		t.Fatalf("active project root = %q %q, want hackthebox %q", config.ActiveProjectRoot, config.ProjectRoot, targetPath)
	}

	recordsAfter, err := ListWorkspaceRecords()
	if err != nil {
		t.Fatalf("ListWorkspaceRecords() after move error = %v", err)
	}
	if len(recordsAfter) != len(recordsBefore) {
		t.Fatalf("workspace count after move = %d, want %d", len(recordsAfter), len(recordsBefore))
	}
	for _, record := range recordsAfter {
		before, ok := beforeByUUID[record.UUID]
		if !ok || before.ID != record.ID {
			t.Fatalf("workspace identity changed after move: before=%+v after=%+v", before, record)
		}
		wantPath := filepath.Join(targetPath, filepath.Base(before.RootPath))
		if record.RootPath != wantPath {
			t.Fatalf("workspace path = %q, want %q", record.RootPath, wantPath)
		}
		if _, err := os.Stat(filepath.Join(record.RootPath, MarkerFile)); err != nil {
			t.Fatalf("moved workspace marker Stat() error = %v", err)
		}
		if _, err := os.Stat(filepath.Join(ctxHome, "workspaces", record.UUID)); err != nil {
			t.Fatalf("workspace data after move Stat() error = %v", err)
		}
		if _, err := os.Stat(before.RootPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old project path still exists or Stat failed unexpectedly: %v", err)
		}
	}
}

func TestProjectRootMoveRejectsDestinationCollisionBeforeMoving(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	sourcePath := filepath.Join(base, "source")
	targetPath := filepath.Join(base, "target")
	if _, err := AddProjectRoot(sourcePath, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddProjectRoot(targetPath, "target"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateProject("alpha"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(targetPath, "alpha"), 0755); err != nil {
		t.Fatal(err)
	}
	chdirForTest(t, t.TempDir())

	_, err := PlanProjectRootMove("source", "target")
	if err == nil || !strings.Contains(err.Error(), "destination already exists") {
		t.Fatalf("PlanProjectRootMove() error = %v, want destination collision", err)
	}
	if _, err := os.Stat(filepath.Join(sourcePath, "alpha", MarkerFile)); err != nil {
		t.Fatalf("source project changed after rejected plan: %v", err)
	}
}

func TestProjectRootMoveRollsBackFilesystemAndDatabase(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	sourcePath := filepath.Join(base, "source")
	targetPath := filepath.Join(base, "target")
	if _, err := AddProjectRoot(sourcePath, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddProjectRoot(targetPath, "target"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, err := CreateProject(name); err != nil {
			t.Fatal(err)
		}
	}
	chdirForTest(t, t.TempDir())
	plan, err := PlanProjectRootMove("source", "target")
	if err != nil {
		t.Fatal(err)
	}

	originalRename := renameProjectRootPath
	calls := 0
	renameProjectRootPath = func(oldPath, newPath string) error {
		calls++
		if calls == 2 {
			return errors.New("injected move failure")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { renameProjectRootPath = originalRename })
	if err := MoveProjectRootProjects(plan); err == nil || !strings.Contains(err.Error(), "injected move failure") {
		t.Fatalf("MoveProjectRootProjects() error = %v, want injected failure", err)
	}

	records, err := ListWorkspaceRecords()
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if filepath.Dir(record.RootPath) != sourcePath {
			t.Fatalf("workspace path after rollback = %q, want source root %q", record.RootPath, sourcePath)
		}
		if _, err := os.Stat(filepath.Join(record.RootPath, MarkerFile)); err != nil {
			t.Fatalf("source project missing after rollback: %v", err)
		}
		if _, err := os.Stat(filepath.Join(targetPath, filepath.Base(record.RootPath))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("destination remains after rollback: %v", err)
		}
	}
}

func TestProjectNewCreatesDirectoryAndInitializesWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "new", "alpha"}, &out); err != nil {
		t.Fatalf("Run(ctx project new alpha) error = %v", err)
	}

	projectPath := filepath.Join(root, "alpha")
	if got := strings.TrimSpace(out.String()); got != projectPath {
		t.Fatalf("output = %q, want %q", got, projectPath)
	}
	if _, err := os.Stat(filepath.Join(projectPath, MarkerFile)); err != nil {
		t.Fatalf("project workspace marker missing: %v", err)
	}
	if _, err := FindWorkspace(projectPath); err != nil {
		t.Fatalf("FindWorkspace(projectPath) error = %v", err)
	}
}

func TestProjectShorthandCreatesDirectoryAndInitializesWorkspace(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "alpha"}, &out); err != nil {
		t.Fatalf("Run(ctx project alpha) error = %v", err)
	}

	projectPath := filepath.Join(root, "alpha")
	if got := strings.TrimSpace(out.String()); got != projectPath {
		t.Fatalf("output = %q, want %q", got, projectPath)
	}
	if _, err := os.Stat(filepath.Join(projectPath, MarkerFile)); err != nil {
		t.Fatalf("project workspace marker missing: %v", err)
	}
	if _, err := FindWorkspace(projectPath); err != nil {
		t.Fatalf("FindWorkspace(projectPath) error = %v", err)
	}
}

func TestProjectListShowsOnlyCtxProjectDirectories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}
	if _, err := CreateProject("alpha"); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "plain"), 0755); err != nil {
		t.Fatalf("Mkdir(plain) error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "ls"}, &out); err != nil {
		t.Fatalf("Run(ctx project ls) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "ID") ||
		!strings.Contains(got, "NAME") ||
		!strings.Contains(got, "1") ||
		!strings.Contains(got, "alpha") {
		t.Fatalf("output = %q, want ID and alpha project name", out.String())
	}
	if strings.Contains(out.String(), "PATH") || strings.Contains(out.String(), filepath.Join(root, "alpha")) {
		t.Fatalf("output = %q, should not include project path", out.String())
	}
	if strings.Contains(out.String(), "plain") {
		t.Fatalf("output = %q, should not include non-ctx directory", out.String())
	}
}

func TestProjectDefaultViewListsProjects(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}
	if _, err := CreateProject("alpha"); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project"}, &out); err != nil {
		t.Fatalf("Run(ctx project) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "ID") ||
		!strings.Contains(got, "NAME") ||
		!strings.Contains(got, "alpha") {
		t.Fatalf("output = %q, want ID and alpha project name", got)
	}
	if strings.Contains(out.String(), "PATH") || strings.Contains(out.String(), filepath.Join(root, "alpha")) {
		t.Fatalf("output = %q, should not include project path", out.String())
	}
}

func TestProjectRemoveDeletesProjectUnderRootOnly(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "labs")
	outside := filepath.Join(base, "outside")
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if err := os.Mkdir(outside, 0755); err != nil {
		t.Fatalf("Mkdir(outside) error = %v", err)
	}
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}
	if _, err := CreateProject("alpha"); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "rm", "alpha", "--yes"}, &out); err != nil {
		t.Fatalf("Run(ctx project rm alpha --yes) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("alpha Stat() error = %v, want not exist", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside directory was touched: %v", err)
	}
}

func TestProjectRemoveRejectsPathTraversal(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "labs")
	outside := filepath.Join(base, "outside")
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if err := os.Mkdir(outside, 0755); err != nil {
		t.Fatalf("Mkdir(outside) error = %v", err)
	}
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}

	var out bytes.Buffer
	err := Run([]string{"ctx", "project", "rm", "../outside", "--yes"}, &out)
	if err == nil || !strings.Contains(err.Error(), "invalid project name") {
		t.Fatalf("Run(ctx project rm ../outside --yes) error = %v, want invalid project name", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside directory was touched: %v", err)
	}
}

func TestProjectRemoveYesSkipsConfirmation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}
	if _, err := CreateProject("alpha"); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	setWorkspaceInputForTest(t, "n\n")

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "rm", "alpha", "--yes"}, &out); err != nil {
		t.Fatalf("Run(ctx project rm alpha --yes) error = %v", err)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("output = %q, --yes should skip confirmation", out.String())
	}
}

func TestProjectRemoveByID(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}
	if _, err := CreateProject("alpha"); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	projects, err := ListProjects()
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects) != 1 || projects[0].ID == 0 {
		t.Fatalf("projects = %+v, want one project with ID", projects)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "rm", workspaceIDString(projects[0].ID), "--yes"}, &out); err != nil {
		t.Fatalf("Run(ctx project rm <id> --yes) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("alpha Stat() error = %v, want not exist", err)
	}
	if !strings.Contains(out.String(), "removed project: "+filepath.Join(root, "alpha")) {
		t.Fatalf("output = %q, want removed project path", out.String())
	}
}

func TestProjectRemoveShortYesSkipsConfirmation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}
	if _, err := CreateProject("alpha"); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	setWorkspaceInputForTest(t, "n\n")

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "rm", "alpha", "-y"}, &out); err != nil {
		t.Fatalf("Run(ctx project rm alpha -y) error = %v", err)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Fatalf("output = %q, -y should skip confirmation", out.String())
	}
}

func TestProjectRemoveRequiresConfirmationByDefault(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	if _, err := SetProjectRoot(root); err != nil {
		t.Fatalf("SetProjectRoot() error = %v", err)
	}
	if _, err := CreateProject("alpha"); err != nil {
		t.Fatalf("CreateProject(alpha) error = %v", err)
	}
	setWorkspaceInputForTest(t, "n\n")

	var out bytes.Buffer
	if err := Run([]string{"ctx", "project", "rm", "alpha"}, &out); err != nil {
		t.Fatalf("Run(ctx project rm alpha) error = %v", err)
	}
	if !strings.Contains(out.String(), "Remove project alpha") ||
		!strings.Contains(out.String(), "cancelled") {
		t.Fatalf("output = %q, want confirmation and cancellation", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "alpha")); err != nil {
		t.Fatalf("alpha should remain after cancellation: %v", err)
	}
}
