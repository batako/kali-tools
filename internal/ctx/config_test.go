package ctx

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValueProjectRoot(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	chdirForTest(t, base)

	want := filepath.Join(base, "cases")
	got, err := SetConfigValue(ConfigKeyProjectRoot, "cases")
	if err != nil {
		t.Fatalf("SetConfigValue(project.root) error = %v", err)
	}
	if got != want {
		t.Fatalf("SetConfigValue(project.root) = %q, want %q", got, want)
	}

	got, err = GetConfigValue(ConfigKeyProjectRoot)
	if err != nil {
		t.Fatalf("GetConfigValue(project.root) error = %v", err)
	}
	if got != want {
		t.Fatalf("GetConfigValue(project.root) = %q, want %q", got, want)
	}

	entries, err := ListConfigValues()
	if err != nil {
		t.Fatalf("ListConfigValues() error = %v", err)
	}
	if len(entries) != 9 || entries[0].Key != ConfigKeyProjectRoot || entries[0].Value != want || entries[0].DefaultValue != "-" ||
		entries[1].Key != ConfigKeyDirectoryMaxRequests || entries[1].Value != "1000000" || entries[1].DefaultValue != "1000000" ||
		entries[2].Key != ConfigKeyFileMaxRequests || entries[2].Value != "200000" || entries[2].DefaultValue != "200000" ||
		entries[3].Key != ConfigKeyVHostMaxRequests || entries[3].Value != "10000" || entries[3].DefaultValue != "10000" ||
		entries[4].Key != ConfigKeyVHostCalibrationSamples || entries[4].Value != "10" || entries[4].DefaultValue != "10" ||
		entries[5].Key != ConfigKeyVHostCalibrationConfidence || entries[5].Value != "90" || entries[5].DefaultValue != "90" ||
		entries[6].Key != ConfigKeyPasswordMaxRequests || entries[6].Value != "10000" || entries[6].DefaultValue != "10000" ||
		entries[7].Key != ConfigKeyDNSMaxQueries || entries[7].Value != "10000" || entries[7].DefaultValue != "10000" ||
		entries[8].Key != ConfigKeyTLSVerify || entries[8].Value != "true" || entries[8].DefaultValue != "true" {
		t.Fatalf("ListConfigValues() = %+v, want project.root and request limits", entries)
	}
}

func TestConfigValueTLSVerify(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	got, err := SetConfigValue(ConfigKeyTLSVerify, "false")
	if err != nil || got != "false" {
		t.Fatalf("SetConfigValue(web.tls.verify) = %q, %v", got, err)
	}
	got, err = GetConfigValue(ConfigKeyTLSVerify)
	if err != nil || got != "false" {
		t.Fatalf("GetConfigValue(web.tls.verify) = %q, %v", got, err)
	}
	config, err := LoadConfig()
	if err != nil || config.TLSVerify {
		t.Fatalf("LoadConfig().TLSVerify = %v, %v, want false", config.TLSVerify, err)
	}
}

func TestConfigValueRequestLimits(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	got, err := SetConfigValue(ConfigKeyDirectoryMaxRequests, "250")
	if err != nil || got != "250" {
		t.Fatalf("SetConfigValue(escalation limit) = %q, %v", got, err)
	}
	got, err = GetConfigValue(ConfigKeyDirectoryMaxRequests)
	if err != nil || got != "250" {
		t.Fatalf("GetConfigValue(escalation limit) = %q, %v", got, err)
	}
	got, err = SetConfigValue(ConfigKeyFileMaxRequests, "100")
	if err != nil || got != "100" {
		t.Fatalf("SetConfigValue(file request limit) = %q, %v", got, err)
	}
	got, err = SetConfigValue(ConfigKeyVHostMaxRequests, "7500")
	if err != nil || got != "7500" {
		t.Fatalf("SetConfigValue(vhost request limit) = %q, %v", got, err)
	}
	got, err = SetConfigValue(ConfigKeyVHostCalibrationSamples, "12")
	if err != nil || got != "12" {
		t.Fatalf("SetConfigValue(vhost calibration samples) = %q, %v", got, err)
	}
	got, err = SetConfigValue(ConfigKeyVHostCalibrationConfidence, "95")
	if err != nil || got != "95" {
		t.Fatalf("SetConfigValue(vhost calibration confidence) = %q, %v", got, err)
	}
	got, err = SetConfigValue(ConfigKeyDNSMaxQueries, "5000")
	if err != nil || got != "5000" {
		t.Fatalf("SetConfigValue(DNS query limit) = %q, %v", got, err)
	}
	got, err = GetConfigValue(ConfigKeyDNSMaxQueries)
	if err != nil || got != "5000" {
		t.Fatalf("GetConfigValue(DNS query limit) = %q, %v", got, err)
	}
}

func TestConfigValueRejectsUnknownKey(t *testing.T) {
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))

	if _, err := GetConfigValue("project.unknown"); err == nil {
		t.Fatal("GetConfigValue(project.unknown) error = nil, want error")
	}
	if _, err := SetConfigValue("project.unknown", "value"); err == nil {
		t.Fatal("SetConfigValue(project.unknown) error = nil, want error")
	}
}

func TestRunConfigGetSetProjectRoot(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CTX_HOME", filepath.Join(t.TempDir(), ".ctx"))
	chdirForTest(t, base)

	var out bytes.Buffer
	if err := Run([]string{"ctx", "config", "set", ConfigKeyProjectRoot, "cases"}, &out); err != nil {
		t.Fatalf("Run(ctx config set project.root cases) error = %v", err)
	}
	want := filepath.Join(base, "cases")
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("set output = %q, want %q", got, want)
	}

	out.Reset()
	if err := Run([]string{"ctx", "config", "get", ConfigKeyProjectRoot}, &out); err != nil {
		t.Fatalf("Run(ctx config get project.root) error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != want {
		t.Fatalf("get output = %q, want %q", got, want)
	}

	for _, args := range [][]string{
		{"ctx", "config"},
		{"ctx", "config", "ls"},
	} {
		out.Reset()
		if err := Run(args, &out); err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		text := out.String()
		for _, wantText := range []string{"KEY", "VALUE", ConfigKeyProjectRoot, want} {
			if !strings.Contains(text, wantText) {
				t.Fatalf("Run(%v) output = %q, want %q", args, text, wantText)
			}
		}
	}
}
