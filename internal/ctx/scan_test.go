package ctx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunScanStoresServicesAndArtifacts(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	installFakeNmap(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunScan([]string{"scan"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunScan exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "Saved 2 service record(s)") {
		t.Fatalf("stdout = %q, want service summary", got)
	}
	if got := stderr.String(); !strings.Contains(got, "ports=default") {
		t.Fatalf("stderr = %q, want fake nmap stderr", got)
	}

	target, err := GetPrimaryTarget(workspace)
	if err != nil {
		t.Fatalf("GetPrimaryTarget() error = %v", err)
	}
	services, err := ListServices(workspace, target)
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("services len = %d, want 2", len(services))
	}
	if services[0].Port != 22 || services[0].ServiceName != "ssh" || services[0].State != "open" {
		t.Fatalf("first service = %+v, want ssh open port 22", services[0])
	}
	if services[1].ScriptsJSON != "" || services[1].Hostname != "" {
		t.Fatalf("second service removed fields = scripts %q hostname %q, want empty", services[1].ScriptsJSON, services[1].Hostname)
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 ||
		logs[0].Command != "ctx scan" ||
		!strings.Contains(logs[0].ExpandedCommand, "-oX") ||
		logs[0].Status != "success" {
		t.Fatalf("logs = %+v, want ctx scan invocation and expanded nmap command", logs)
	}

	entries, err := os.ReadDir(filepath.Join(workspace.DataPath, "scans"))
	if err != nil {
		t.Fatalf("ReadDir(scans) error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("scan artifact count = %d, want 2", len(entries))
	}
}

func TestRunScanRecordsXScanInvocation(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	installFakeNmap(t)
	t.Setenv("CTX_INVOKED_AS", "xscan")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := RunScan([]string{"scan", "-p", "22,80"}, &stdout, &stderr); code != 0 {
		t.Fatalf("RunScan exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1", len(logs))
	}
	if logs[0].Command != "xscan -p 22,80" {
		t.Fatalf("command = %q, want xscan invocation", logs[0].Command)
	}
	if !strings.HasPrefix(logs[0].ExpandedCommand, "nmap ") {
		t.Fatalf("expanded command = %q, want nmap command", logs[0].ExpandedCommand)
	}
}

func TestRunScanAddsTargetForExplicitIP(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	installFakeNmap(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunScan([]string{"scan", "10.10.10.20", "-p", "80,443"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunScan exit code = %d, want 0; stderr = %q", code, stderr.String())
	}

	targets, err := ListTargets(workspace)
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets len = %d, want 2", len(targets))
	}
	target, err := GetTargetByIP(workspace, "10.10.10.20")
	if err != nil {
		t.Fatalf("GetTargetByIP() error = %v", err)
	}
	services, err := ListServices(workspace, target)
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("services len = %d, want 2", len(services))
	}
	if got := stderr.String(); !strings.Contains(got, "ports=80,443") {
		t.Fatalf("stderr = %q, want explicit port flag", got)
	}
}

func TestRunScanDryRunPrintsCommand(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := RunScan([]string{"scan", "-n", "-p", "22,80"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("RunScan exit code = %d, want 0; stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, `nmap -Pn -n -sV -oN`) || !strings.Contains(got, `-p 22,80 10.10.10.10`) {
		t.Fatalf("stdout = %q, want rendered nmap command", got)
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("logs len = %d, want 0 for dry-run", len(logs))
	}
}

func TestRunScanSkipsDuplicateWithoutCommandLog(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	installFakeNmap(t)

	for run := 1; run <= 2; run++ {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := RunScan([]string{"scan", "-p", "22,80"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("RunScan run %d exit code = %d, want 0; stderr = %q", run, code, stderr.String())
		}
		if run == 2 && !strings.Contains(stdout.String(), "Skipped duplicate scan") {
			t.Fatalf("second stdout = %q, want duplicate message", stdout.String())
		}
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1 after duplicate scan", len(logs))
	}
	entries, err := os.ReadDir(filepath.Join(workspace.DataPath, "scans"))
	if err != nil {
		t.Fatalf("ReadDir(scans) error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("scan artifact count = %d, want 2 after duplicate scan", len(entries))
	}
}

func TestRunScanSkipsPreviousScanAfterTargetIPChanges(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	installFakeNmap(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := RunScan([]string{"scan", "-p", "22,80"}, &stdout, &stderr); code != 0 {
		t.Fatalf("initial RunScan exit code = %d; stderr = %q", code, stderr.String())
	}
	if _, err := SetPrimaryTargetIP(workspace, "10.10.20.20"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() update error = %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := RunScan([]string{"scan", "-p", "22,80"}, &stdout, &stderr); code != 0 {
		t.Fatalf("updated RunScan exit code = %d; stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Skipped duplicate scan for 10.10.20.20") {
		t.Fatalf("stdout = %q, want scan history reused after IP change", stdout.String())
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logs len = %d, want 1 after reused scan", len(logs))
	}
}

func TestRunScanForceRepeatsDuplicate(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	installFakeNmap(t)

	for _, args := range [][]string{{"scan"}, {"scan", "--force"}} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := RunScan(args, &stdout, &stderr); code != 0 {
			t.Fatalf("RunScan(%v) exit code = %d, want 0; stderr = %q", args, code, stderr.String())
		}
	}

	logs, err := ListCommandLogs(workspace)
	if err != nil {
		t.Fatalf("ListCommandLogs() error = %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("logs len = %d, want 2 after forced scan", len(logs))
	}
	entries, err := os.ReadDir(filepath.Join(workspace.DataPath, "scans"))
	if err != nil {
		t.Fatalf("ReadDir(scans) error = %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("scan artifact count = %d, want 4 after forced scan", len(entries))
	}
}

func installFakeNmap(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "nmap")
	script := `#!/bin/sh
normal=""
xml=""
ports=""
target=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -oN)
      normal="$2"
      shift 2
      ;;
    -oX)
      xml="$2"
      shift 2
      ;;
    -p)
      ports="$2"
      shift 2
      ;;
    -*)
      shift
      ;;
    *)
      target="$1"
      shift
      ;;
  esac
done
printf 'Nmap scan report for %s\n' "$target"
printf 'ports=%s\n' "${ports:-default}" >&2
cat >"$normal" <<'EOF'
Nmap scan report
EOF
cat >"$xml" <<'EOF'
<?xml version="1.0"?>
<nmaprun>
  <host>
    <ports>
      <port protocol="tcp" portid="22">
        <state state="open" reason="syn-ack"/>
        <service name="ssh" product="OpenSSH" version="9.7"/>
      </port>
      <port protocol="tcp" portid="80">
        <state state="open" reason="syn-ack"/>
        <service name="http" product="nginx" version="1.25" extrainfo="Ubuntu">
          <cpe>cpe:/a:nginx:nginx:1.25</cpe>
        </service>
        <script id="http-title" output="hello"/>
      </port>
    </ports>
  </host>
</nmaprun>
EOF
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("WriteFile(fake nmap) error = %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
