package ctx

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func decodeAPIResponse(t *testing.T, output []byte) APIResponse {
	t.Helper()
	var response APIResponse
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", string(output), err)
	}
	return response
}

func responseDataMap(t *testing.T, response APIResponse) map[string]any {
	t.Helper()
	data, ok := response.Data.(map[string]any)
	if !ok {
		t.Fatalf("response data = %#v, want object", response.Data)
	}
	return data
}

func TestFormatsJSON(t *testing.T) {
	var out bytes.Buffer
	if err := Run([]string{"ctx", "formats", "--format", "json", "--format-version", "1"}, &out); err != nil {
		t.Fatalf("Run(formats json) error = %v", err)
	}
	response := decodeAPIResponse(t, out.Bytes())
	if !response.Success || response.FormatVersion == nil || *response.FormatVersion != "1.0" || response.Error != nil {
		t.Fatalf("response = %+v, want successful v1.0", response)
	}
	formats, ok := responseDataMap(t, response)["formats"].(map[string]any)
	if !ok {
		t.Fatalf("formats data = %#v, want object", response.Data)
	}
	for _, key := range []string{"formats", "prompt", "credential", "service"} {
		if _, ok := formats[key]; !ok {
			t.Fatalf("formats missing %s: %#v", key, formats)
		}
	}
}

func TestPromptJSONWrapsCommonResponseAndUsesNullsWhenInactive(t *testing.T) {
	t.Setenv("CTX_HOME", t.TempDir())
	root := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})

	var out bytes.Buffer
	if err := Run([]string{"ctx", "prompt", "--format", "json"}, &out); err != nil {
		t.Fatalf("Run(prompt json inactive) error = %v", err)
	}
	response := decodeAPIResponse(t, out.Bytes())
	data := responseDataMap(t, response)
	if data["active"] != false {
		t.Fatalf("active = %#v, want false", data["active"])
	}
	for _, key := range []string{"workspace_id", "workspace_name", "workspace_path", "local_ip", "local_interface", "target_name", "target_ip"} {
		if data[key] != nil {
			t.Fatalf("%s = %#v, want nil", key, data[key])
		}
	}
}

func TestCredentialJSONListsEmptyAndValues(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := AddCredential(workspace, "ssh", "root", "toor"); err != nil {
		t.Fatalf("AddCredential(root) error = %v", err)
	}
	if _, err := AddCredential(workspace, "ssh", "guest", ""); err != nil {
		t.Fatalf("AddCredential(guest) error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "credential", "ls", "ssh", "--format", "json"}, &out); err != nil {
		t.Fatalf("Run(credential json) error = %v", err)
	}
	response := decodeAPIResponse(t, out.Bytes())
	credentials, ok := responseDataMap(t, response)["credentials"].([]any)
	if !ok || len(credentials) != 2 {
		t.Fatalf("credentials = %#v, want two items", response.Data)
	}
	first := credentials[0].(map[string]any)
	if first["scope"] != "ssh" || first["username"] != "guest" || first["password"] != nil {
		t.Fatalf("first credential = %#v, want guest with null password", first)
	}

	out.Reset()
	if err := Run([]string{"ctx", "credential", "ls", "ftp", "--format", "json"}, &out); err != nil {
		t.Fatalf("Run(credential json empty) error = %v", err)
	}
	response = decodeAPIResponse(t, out.Bytes())
	empty := responseDataMap(t, response)["credentials"].([]any)
	if len(empty) != 0 {
		t.Fatalf("empty credentials len = %d, want 0", len(empty))
	}
}

func TestServiceJSONListsEmptyAndValues(t *testing.T) {
	workspace := initXTestWorkspace(t)
	target, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}
	if _, err := UpsertService(workspace, target, Service{
		Port: 22, Protocol: "tcp", State: "open", ServiceName: "ssh",
		Product: "OpenSSH", LastSeen: time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertService() error = %v", err)
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "service", "ls", "--format", "json"}, &out); err != nil {
		t.Fatalf("Run(service json) error = %v", err)
	}
	response := decodeAPIResponse(t, out.Bytes())
	services, ok := responseDataMap(t, response)["services"].([]any)
	if !ok || len(services) != 1 {
		t.Fatalf("services = %#v, want one item", response.Data)
	}
	service := services[0].(map[string]any)
	if service["protocol"] != "tcp" || service["port"] != float64(22) || service["service_name"] != "ssh" {
		t.Fatalf("service = %#v, want ssh tcp/22", service)
	}
	if _, ok := service["extrainfo"]; !ok {
		t.Fatalf("service missing extrainfo field: %#v", service)
	}
}

func TestJSONUnsupportedVersionReturnsJSONAndExitCode2(t *testing.T) {
	var out bytes.Buffer
	err := Run([]string{"ctx", "formats", "--format", "json", "--format-version", "9"}, &out)
	var exitErr ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("error = %v, want exit code 2", err)
	}
	response := decodeAPIResponse(t, out.Bytes())
	if response.Success || response.FormatVersion != nil || response.Error == nil {
		t.Fatalf("response = %+v, want JSON error", response)
	}
	if response.Error.Code != apiErrorInvalidRequestFormatVersion {
		t.Fatalf("error code = %s, want %s", response.Error.Code, apiErrorInvalidRequestFormatVersion)
	}
	if strings.Contains(out.String(), "usage:") {
		t.Fatalf("JSON output contains usage text: %q", out.String())
	}
}
