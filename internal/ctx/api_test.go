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

func requireAPIError(t *testing.T, err error, output []byte, wantExitCode int, wantErrorCode string) APIResponse {
	t.Helper()
	var exitErr ExitCodeError
	if !errors.As(err, &exitErr) || exitErr.Code != wantExitCode {
		t.Fatalf("error = %v, want exit code %d", err, wantExitCode)
	}
	response := decodeAPIResponse(t, output)
	if response.Success || response.Data != nil || response.Error == nil {
		t.Fatalf("response = %+v, want API error", response)
	}
	if response.Error.Code != wantErrorCode {
		t.Fatalf("error code = %q, want %q", response.Error.Code, wantErrorCode)
	}
	return response
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
	wantFormats := map[string]string{
		"credential": "1.0",
		"formats":    "1.0",
		"log":        "1.0",
		"prompt":     "1.0",
		"service":    "1.0",
		"web":        "1.0",
	}
	if len(formats) != len(wantFormats) {
		t.Fatalf("formats = %#v, want exactly %#v", formats, wantFormats)
	}
	for key, wantVersion := range wantFormats {
		versions, ok := formats[key].([]any)
		if !ok || len(versions) != 1 || versions[0] != wantVersion {
			t.Fatalf("formats[%q] = %#v, want [%q]", key, formats[key], wantVersion)
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

func TestWebJSONListsAggregatedDiscoveries(t *testing.T) {
	workspace := initXTestWorkspace(t)
	target, err := SetPrimaryTargetIP(workspace, "10.10.10.10")
	if err != nil {
		t.Fatal(err)
	}
	for _, discovery := range []WebDiscovery{
		{URL: "https://example.test/admin", Path: "/admin", StatusCode: 301, SourceTool: "gobuster", Wordlist: "one"},
		{URL: "https://example.test/admin", Path: "/admin", StatusCode: 200, ContentLength: 512, ContentLengthValid: true, SourceTool: "ffuf", Wordlist: "two"},
	} {
		if _, err := SaveWebDiscovery(workspace, target, discovery); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	if err := Run([]string{"ctx", "web", "ls", "--format", "json", "--format-version", "1"}, &out); err != nil {
		t.Fatalf("Run(web json) error = %v", err)
	}
	response := decodeAPIResponse(t, out.Bytes())
	discoveries, ok := responseDataMap(t, response)["discoveries"].([]any)
	if !ok || len(discoveries) != 1 {
		t.Fatalf("discoveries = %#v, want one aggregated item", response.Data)
	}
	item := discoveries[0].(map[string]any)
	if item["path"] != "/admin" || item["status_code"] != float64(200) || item["content_length"] != float64(512) {
		t.Fatalf("web discovery = %#v", item)
	}
	sources, ok := item["sources"].([]any)
	if !ok || len(sources) != 2 || sources[0] != "ffuf" || sources[1] != "gobuster" {
		t.Fatalf("sources = %#v, want ffuf and gobuster", item["sources"])
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

func TestJSONInvalidArgumentsUseCommonEnvelopeAndExitCode2(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		input string
	}{
		{name: "formats extra argument", args: []string{"ctx", "formats", "extra", "--format", "json"}},
		{name: "formats missing version", args: []string{"ctx", "formats", "--format", "json", "--format-version"}},
		{name: "formats conflicting output", args: []string{"ctx", "formats", "--format", "json", "--format", "yaml"}},
		{name: "prompt unknown option", args: []string{"ctx", "prompt", "--unknown", "--format", "json"}},
		{name: "credential extra scope", args: []string{"ctx", "credential", "ls", "ssh", "extra", "--format", "json"}},
		{name: "credential unsupported operation", args: []string{"ctx", "credential", "set", "ssh", "root", "--format", "json"}},
		{name: "service missing target", args: []string{"ctx", "service", "ls", "--target", "--format", "json"}},
		{name: "web missing target", args: []string{"ctx", "web", "ls", "--target", "--format", "json"}},
		{name: "log unknown operation", args: []string{"ctx", "log", "unknown", "--format", "json"}},
		{name: "log start extra argument", args: []string{"ctx", "log", "start", "extra", "--format", "json"}, input: `{}`},
		{name: "log finish missing id", args: []string{"ctx", "log", "finish", "--format", "json"}, input: `{}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := RunWithInput(test.args, strings.NewReader(test.input), &stdout, &stderr)
			response := requireAPIError(t, err, stdout.Bytes(), 2, "INVALID_REQUEST")
			if response.FormatVersion == nil || *response.FormatVersion != "1.0" {
				t.Fatalf("format version = %#v, want 1.0", response.FormatVersion)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestJSONNotFoundErrorsAreNotInternalErrors(t *testing.T) {
	initXTestWorkspace(t)

	t.Run("primary target", func(t *testing.T) {
		var out bytes.Buffer
		err := Run([]string{"ctx", "service", "ls", "--format", "json"}, &out)
		requireAPIError(t, err, out.Bytes(), 1, apiErrorNotFoundTarget)
	})

	t.Run("named target", func(t *testing.T) {
		var out bytes.Buffer
		err := Run([]string{"ctx", "service", "ls", "--target", "missing", "--format", "json"}, &out)
		requireAPIError(t, err, out.Bytes(), 1, apiErrorNotFoundTarget)
	})

	t.Run("command log", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		input := strings.NewReader(`{"status":"success","exit_code":0}`)
		err := RunWithInput([]string{"ctx", "log", "finish", "999", "--format", "json"}, input, &stdout, &stderr)
		requireAPIError(t, err, stdout.Bytes(), 1, apiErrorNotFoundLog)
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
	})

}
