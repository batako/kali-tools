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

func TestRunDoctorReportsAllChecksOK(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/usr/bin/zsh")
	t.Setenv("CTX_HOME", filepath.Join(home, ".ctx"))
	if _, err := InitWorkspace(root); err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(shellBlock("zsh")), 0644); err != nil {
		t.Fatalf("WriteFile(.zshrc) error = %v", err)
	}
	chdirForTest(t, root)

	executable := filepath.Join(t.TempDir(), "ctx")
	if err := os.WriteFile(executable, []byte("binary"), 0755); err != nil {
		t.Fatalf("WriteFile(executable) error = %v", err)
	}
	setDoctorExecutableForTest(t, executable, executable)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "doctor"}, &out); err != nil {
		t.Fatalf("Run(ctx doctor) error = %v\noutput:\n%s", err, out.String())
	}
	got := out.String()
	for _, check := range []string{"Executable", "PATH", "Shell", "Shell integration", "Workspace"} {
		if !strings.Contains(got, "OK  "+check+"\n") {
			t.Fatalf("doctor output = %q, want OK %s", got, check)
		}
	}
	if !strings.Contains(got, "5 OK, 0 NG") || strings.Contains(got, "Fix:") {
		t.Fatalf("doctor output = %q, want clean summary", got)
	}
}

func TestRunDoctorReportsNGWithFixesWithoutCommandFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/fish")
	t.Setenv("CTX_HOME", filepath.Join(home, ".ctx"))
	chdirForTest(t, t.TempDir())

	runningExecutable := filepath.Join(t.TempDir(), "ctx-running")
	pathExecutable := filepath.Join(t.TempDir(), "ctx-old")
	for _, path := range []string{runningExecutable, pathExecutable} {
		if err := os.WriteFile(path, []byte(path), 0755); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", path, err)
		}
	}
	setDoctorExecutableForTest(t, runningExecutable, pathExecutable)
	oldParentProcessNameFunc := parentProcessNameFunc
	parentProcessNameFunc = func() (string, error) { return "", errors.New("parent unavailable") }
	t.Cleanup(func() { parentProcessNameFunc = oldParentProcessNameFunc })

	var out bytes.Buffer
	err := Run([]string{"ctx", "doctor"}, &out)
	if err != nil {
		t.Fatalf("Run(ctx doctor) error = %v, want successful diagnostic command", err)
	}

	got := out.String()
	for _, check := range []string{"PATH", "Shell", "Workspace"} {
		if !strings.Contains(got, "NG  "+check+"\n") {
			t.Fatalf("doctor output = %q, want NG %s", got, check)
		}
	}
	for _, fix := range []string{
		"Fix: remove the stale ctx binary or update PATH, then restart the shell",
		"Fix: set SHELL to zsh or bash",
		"Fix: ctx workspace init",
	} {
		if !strings.Contains(got, fix) {
			t.Fatalf("doctor output = %q, want %q", got, fix)
		}
	}
	if !strings.Contains(got, "1 OK, 3 NG") {
		t.Fatalf("doctor output = %q, want failure summary", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("doctor buffer output contains ANSI colors: %q", got)
	}
}

func TestWriteDoctorReportUsesColorsForTerminal(t *testing.T) {
	oldColorEnabled := doctorColorEnabled
	doctorColorEnabled = func(io.Writer) bool { return true }
	t.Cleanup(func() { doctorColorEnabled = oldColorEnabled })

	var out bytes.Buffer
	ngCount, err := writeDoctorReport(&out, []doctorCheck{
		{Name: "Healthy", OK: true, Detail: "ready"},
		{Name: "Broken", Detail: "missing", Fix: "repair it"},
	})
	if err != nil {
		t.Fatalf("writeDoctorReport() error = %v", err)
	}
	if ngCount != 1 {
		t.Fatalf("ng count = %d, want 1", ngCount)
	}
	got := out.String()
	for _, want := range []string{
		doctorGreen + "OK" + doctorReset,
		doctorRed + "NG" + doctorReset,
		doctorYellow + "Fix:" + doctorReset,
		doctorGreen + "1 OK" + doctorReset,
		doctorRed + "1 NG" + doctorReset,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("colored doctor output = %q, want %q", got, want)
		}
	}
}

func setDoctorExecutableForTest(t *testing.T, executable, pathExecutable string) {
	t.Helper()
	oldExecutableFunc := executableFunc
	oldLookPathFunc := lookPathFunc
	executableFunc = func() (string, error) { return executable, nil }
	lookPathFunc = func(string) (string, error) { return pathExecutable, nil }
	t.Cleanup(func() {
		executableFunc = oldExecutableFunc
		lookPathFunc = oldLookPathFunc
	})
}
