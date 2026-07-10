package ctx

import (
	"bytes"
	"strings"
	"testing"
)

func TestTargetLifecycle(t *testing.T) {
	workspace := initTestWorkspace(t)

	primary, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if primary.Name != "default" || primary.IP != "10.10.10.10" || !primary.IsPrimary {
		t.Fatalf("primary target = %+v, want default 10.10.10.10 primary", primary)
	}

	updated, err := SetPrimaryTargetIP(workspace, "10.10.20.55")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() update error = %v", err)
	}
	if updated.Name != "default" || updated.IP != "10.10.20.55" {
		t.Fatalf("updated target = %+v, want default 10.10.20.55", updated)
	}

	second, err := AddTarget(workspace, "10.10.10.20", "")
	if err != nil {
		t.Fatalf("AddTarget() error = %v", err)
	}
	if second.Name != "target2" || second.IsPrimary {
		t.Fatalf("second target = %+v, want non-primary target2", second)
	}

	used, err := UseTarget(workspace, "target2")
	if err != nil {
		t.Fatalf("UseTarget() error = %v", err)
	}
	if used.Name != "target2" || !used.IsPrimary {
		t.Fatalf("used target = %+v, want primary target2", used)
	}

	if err := RemoveTarget(workspace, "target2"); err != nil {
		t.Fatalf("RemoveTarget() error = %v", err)
	}
	fallback, err := GetPrimaryTarget(workspace)
	if err != nil {
		t.Fatalf("GetPrimaryTarget() error = %v", err)
	}
	if fallback.Name != "default" {
		t.Fatalf("fallback primary = %+v, want default", fallback)
	}
}

func TestRunTargetAndIPCommands(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(workspace init) error = %v", err)
	}

	out.Reset()
	if err := Run([]string{"ctx", "target", "set", "10.10.10.10"}, &out); err != nil {
		t.Fatalf("Run(target set) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "primary target: default 10.10.10.10") {
		t.Fatalf("target set output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "target", "add", "10.10.10.20", "--name", "dc"}, &out); err != nil {
		t.Fatalf("Run(target add) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "dc 10.10.10.20") {
		t.Fatalf("target add output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "target", "use", "dc"}, &out); err != nil {
		t.Fatalf("Run(target use) error = %v", err)
	}

	out.Reset()
	if err := Run([]string{"ctx", "ip"}, &out); err != nil {
		t.Fatalf("Run(ip) error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "10.10.10.20" {
		t.Fatalf("ip output = %q, want 10.10.10.20", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "target", "ls"}, &out); err != nil {
		t.Fatalf("Run(target ls) error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "  default 10.10.10.10") || !strings.Contains(got, "* dc 10.10.10.20") {
		t.Fatalf("target ls output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "target"}, &out); err != nil {
		t.Fatalf("Run(target default view) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "  default 10.10.10.10") || !strings.Contains(got, "* dc 10.10.10.20") {
		t.Fatalf("target default view output = %q", got)
	}
}

func TestRunTargetShorthandMatchesSet(t *testing.T) {
	explicitOutput, explicitTargets := runTargetCommandInFreshWorkspace(t, []string{"ctx", "target", "set", "10.10.10.10"})
	shorthandOutput, shorthandTargets := runTargetCommandInFreshWorkspace(t, []string{"ctx", "target", "10.10.10.10"})

	if shorthandOutput != explicitOutput {
		t.Fatalf("shorthand output = %q, want %q", shorthandOutput, explicitOutput)
	}
	if len(shorthandTargets) != 1 || len(explicitTargets) != 1 {
		t.Fatalf("targets = %+v, want one target in each workspace", shorthandTargets)
	}
	if shorthandTargets[0].Name != explicitTargets[0].Name ||
		shorthandTargets[0].IP != explicitTargets[0].IP ||
		shorthandTargets[0].IsPrimary != explicitTargets[0].IsPrimary {
		t.Fatalf("shorthand target = %+v, want %+v", shorthandTargets[0], explicitTargets[0])
	}
}

func TestRunTargetShorthandInvalidIPUsesSetValidation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(workspace init) error = %v", err)
	}

	out.Reset()
	err := Run([]string{"ctx", "target", "hoge"}, &out)
	if err == nil || !strings.Contains(err.Error(), "invalid IP address: hoge") {
		t.Fatalf("Run(ctx target hoge) error = %v, want invalid IP validation", err)
	}
}

func TestRunTargetExistingSubcommandsUnaffectedByDefaultAction(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(workspace init) error = %v", err)
	}
	out.Reset()
	if err := Run([]string{"ctx", "target", "set", "10.10.10.10"}, &out); err != nil {
		t.Fatalf("Run(target set) error = %v", err)
	}

	out.Reset()
	if err := Run([]string{"ctx", "target", "add", "10.10.10.20", "--name", "dc"}, &out); err != nil {
		t.Fatalf("Run(target add) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, " dc 10.10.10.20") {
		t.Fatalf("target add output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "target", "use", "dc"}, &out); err != nil {
		t.Fatalf("Run(target use) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "primary target: dc 10.10.10.20") {
		t.Fatalf("target use output = %q", got)
	}

	out.Reset()
	if err := Run([]string{"ctx", "target", "rm", "dc"}, &out); err != nil {
		t.Fatalf("Run(target rm) error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "removed target: dc") {
		t.Fatalf("target rm output = %q", got)
	}
}

func TestSetPrimaryTargetIPRejectsInvalidIP(t *testing.T) {
	workspace := initTestWorkspace(t)

	_, err := SetPrimaryTargetIP(workspace, "not-an-ip")
	if err == nil {
		t.Fatal("SetPrimaryTargetIP() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "invalid IP address") {
		t.Fatalf("error = %q, want invalid IP address", err.Error())
	}
}

func runTargetCommandInFreshWorkspace(t *testing.T, args []string) (string, []Target) {
	t.Helper()

	root := t.TempDir()
	t.Setenv("CTX_HOME", t.TempDir())
	t.Chdir(root)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "workspace", "init"}, &out); err != nil {
		t.Fatalf("Run(workspace init) error = %v", err)
	}
	out.Reset()
	if err := Run(args, &out); err != nil {
		t.Fatalf("Run(%v) error = %v", args, err)
	}
	workspace, err := FindWorkspace(root)
	if err != nil {
		t.Fatalf("FindWorkspace() error = %v", err)
	}
	targets, err := ListTargets(workspace)
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	return out.String(), targets
}

func initTestWorkspace(t *testing.T) *Workspace {
	t.Helper()

	t.Setenv("CTX_HOME", t.TempDir())
	workspace, err := InitWorkspace(t.TempDir())
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
	return workspace
}
