package ctx

import (
	"bytes"
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

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.ProjectRoot != want {
		t.Fatalf("config project root = %q, want %q", config.ProjectRoot, want)
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
	if got, want := out.String(), filepath.Join(root, "alpha")+"\n"; got != want {
		t.Fatalf("output = %q, want alpha project", out.String())
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
	if got, want := out.String(), filepath.Join(root, "alpha")+"\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
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
