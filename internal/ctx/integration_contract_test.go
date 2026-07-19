package ctx

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type contractResult struct {
	code   int
	stdout string
	stderr string
}

func runContractCommand(t *testing.T, args []string, input string) contractResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := RunCLI(args, strings.NewReader(input), &stdout, &stderr)
	return contractResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func decodeContractResponse(t *testing.T, output string) APIResponse {
	t.Helper()
	var response APIResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("JSON output = %q, error = %v", output, err)
	}
	return response
}

func TestPublicIntegrationContractJSON(t *testing.T) {
	initXTestWorkspace(t)

	t.Run("feature discovery", func(t *testing.T) {
		result := runContractCommand(t, []string{"ctx", "formats", "--format", "json", "--format-version", "1.0"}, "")
		if result.code != 0 || result.stderr != "" {
			t.Fatalf("result = %+v, want success without stderr", result)
		}
		response := decodeContractResponse(t, result.stdout)
		if !response.Success || response.FormatVersion == nil || *response.FormatVersion != "1.0" {
			t.Fatalf("response = %+v, want successful format 1.0", response)
		}
		formats := responseDataMap(t, response)["formats"].(map[string]any)
		for _, name := range []string{"credential", "formats", "log", "prompt", "service", "web"} {
			versions, ok := formats[name].([]any)
			if !ok || len(versions) != 1 || versions[0] != "1.0" {
				t.Fatalf("formats[%q] = %#v, want [1.0]", name, formats[name])
			}
		}
	})

	t.Run("empty list", func(t *testing.T) {
		result := runContractCommand(t, []string{"ctx", "credential", "ls", "ssh", "--format", "json", "--format-version", "1.0"}, "")
		if result.code != 0 || result.stderr != "" {
			t.Fatalf("result = %+v, want success without stderr", result)
		}
		response := decodeContractResponse(t, result.stdout)
		credentials := responseDataMap(t, response)["credentials"].([]any)
		if len(credentials) != 0 {
			t.Fatalf("credentials = %#v, want []", credentials)
		}
	})

	t.Run("invalid arguments", func(t *testing.T) {
		result := runContractCommand(t, []string{"ctx", "prompt", "--unknown", "--format", "json", "--format-version", "1.0"}, "")
		if result.code != 2 || result.stderr != "" {
			t.Fatalf("result = %+v, want exit 2 without stderr", result)
		}
		response := decodeContractResponse(t, result.stdout)
		if response.Success || response.Error == nil || response.Error.Code != "INVALID_REQUEST" {
			t.Fatalf("response = %+v, want INVALID_REQUEST", response)
		}
	})

	t.Run("unsupported format version", func(t *testing.T) {
		result := runContractCommand(t, []string{"ctx", "formats", "--format", "json", "--format-version", "9.0"}, "")
		if result.code != 2 || result.stderr != "" {
			t.Fatalf("result = %+v, want exit 2 without stderr", result)
		}
		response := decodeContractResponse(t, result.stdout)
		if response.FormatVersion != nil || response.Error == nil || response.Error.Code != apiErrorInvalidRequestFormatVersion {
			t.Fatalf("response = %+v, want unsupported version error", response)
		}
	})

	t.Run("missing resource", func(t *testing.T) {
		result := runContractCommand(t, []string{"ctx", "service", "ls", "--target", "missing", "--format", "json", "--format-version", "1.0"}, "")
		if result.code != 1 || result.stderr != "" {
			t.Fatalf("result = %+v, want exit 1 without stderr", result)
		}
		response := decodeContractResponse(t, result.stdout)
		if response.Error == nil || response.Error.Code != apiErrorNotFoundTarget {
			t.Fatalf("response = %+v, want target not found", response)
		}
	})
}

func TestPublicIntegrationContractRegistrationCommands(t *testing.T) {
	workspace := initXTestWorkspace(t)
	if _, err := SetPrimaryTargetIP(workspace, "10.10.10.10"); err != nil {
		t.Fatalf("SetPrimaryTargetIP() error = %v", err)
	}

	t.Run("host registration is idempotent", func(t *testing.T) {
		args := []string{"ctx", "host", "add", "Admin.Example.THM."}
		for i := 0; i < 2; i++ {
			result := runContractCommand(t, args, "")
			if result.code != 0 || result.stdout == "" || result.stderr != "" {
				t.Fatalf("run %d result = %+v, want stdout success", i+1, result)
			}
		}
		hosts, err := ListHosts(workspace)
		if err != nil {
			t.Fatalf("ListHosts() error = %v", err)
		}
		if len(hosts) != 1 || hosts[0].Hostname != "admin.example.thm" {
			t.Fatalf("hosts = %+v, want one normalized host", hosts)
		}
	})

	t.Run("credential add rejects duplicate", func(t *testing.T) {
		args := []string{"ctx", "credential", "add", "ssh", "root", "toor"}
		first := runContractCommand(t, args, "")
		if first.code != 0 || first.stdout == "" || first.stderr != "" {
			t.Fatalf("first result = %+v, want stdout success", first)
		}
		second := runContractCommand(t, args, "")
		if second.code != 1 || second.stdout != "" || second.stderr == "" {
			t.Fatalf("duplicate result = %+v, want stderr failure", second)
		}
	})

	t.Run("invalid host uses normal command failure", func(t *testing.T) {
		result := runContractCommand(t, []string{"ctx", "host", "add", "bad host"}, "")
		if result.code != 1 || result.stdout != "" || result.stderr == "" {
			t.Fatalf("result = %+v, want exit 1 stderr failure", result)
		}
	})

	t.Run("duplicate notes remain separate", func(t *testing.T) {
		args := []string{"ctx", "note", "same", "finding"}
		for i := 0; i < 2; i++ {
			result := runContractCommand(t, args, "")
			if result.code != 0 || result.stdout == "" || result.stderr != "" {
				t.Fatalf("run %d result = %+v, want stdout success", i+1, result)
			}
		}
		notes, err := ListNotes(workspace)
		if err != nil {
			t.Fatalf("ListNotes() error = %v", err)
		}
		if len(notes) != 2 || notes[0].Body != "same finding" || notes[1].Body != "same finding" {
			t.Fatalf("notes = %+v, want two duplicate notes", notes)
		}
	})
}
