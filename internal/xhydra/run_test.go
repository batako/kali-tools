package xhydra

import (
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"req/internal/ctx"
)

func TestParseOptions(t *testing.T) {
	options, err := parseOptions([]string{
		"http", "-u", "john", "-r", "login.req",
		"--user-field", "username", "--password-field", "password",
		"--fail-json", "status=error", "-P", "/tmp/passwords.txt",
	})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if options.Mode != "http" || options.Username != "john" || options.RequestFile != "login.req" || options.UserField != "username" || options.PasswordField != "password" || options.FailJSON != "status=error" || options.PasswordList != "/tmp/passwords.txt" {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseServiceOptions(t *testing.T) {
	options, err := parseOptions([]string{"ssh", "-u", "john", "--host", "10.0.0.5", "--port", "2222", "--service", "2", "--tasks", "8", "-P", "/tmp/passwords.txt"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if options.Mode != "ssh" || options.Username != "john" || options.Host != "10.0.0.5" || options.Port != "2222" || options.Service != "2" || options.Tasks != 8 || options.PasswordList != "/tmp/passwords.txt" {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseTasksRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"0", "-1", "many"} {
		if _, err := parseOptions([]string{"ssh", "-u", "john", "-t", value}); err == nil || !strings.Contains(err.Error(), "positive integer") {
			t.Fatalf("parseOptions(-t %s) error = %v, want positive integer error", value, err)
		}
	}
}

func TestTasksAreRestrictedToSupportedExecutionModes(t *testing.T) {
	for _, args := range [][]string{{"xhydra", "smb", "-u", "john", "-t", "8"}, {"xhydra", "ssh", "-u", "john", "--tasks", "8", "--status"}} {
		err := newApp(nil, strings.NewReader(""), &strings.Builder{}, &strings.Builder{}).run(args)
		if err == nil || !strings.Contains(err.Error(), "--tasks") {
			t.Fatalf("run(%#v) error = %v, want tasks usage error", args, err)
		}
	}
}

func TestBuildFTPServiceHydraArgsUsesConservativeTasks(t *testing.T) {
	args := buildServiceHydraArgs("ftp", "10.0.0.5", 21, "/tmp/passwords.txt", "john", 0)
	want := []string{"-l", "john", "-P", "/tmp/passwords.txt", "-t", "4", "-f", "-s", "21", "10.0.0.5", "ftp"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}

	args = buildServiceHydraArgs("ftp", "10.0.0.5", 21, "/tmp/passwords.txt", "john", 2)
	want[5] = "2"
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("override args = %#v, want %#v", args, want)
	}
}

func TestParseClearCacheOption(t *testing.T) {
	options, err := parseOptions([]string{"ssh", "-u", "kali", "--clear-cache"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if !options.ClearCache {
		t.Fatalf("options.ClearCache = false, want true")
	}
}

func TestParseOptionsRejectsRemovedForce(t *testing.T) {
	if _, err := parseOptions([]string{"ssh", "-u", "kali", "--force"}); err == nil || !strings.Contains(err.Error(), "was removed") {
		t.Fatalf("parseOptions(--force) error = %v, want removed option error", err)
	}
}

func TestMatchingServices(t *testing.T) {
	services := matchingServices([]ctx.Service{
		{Port: 22, Protocol: "tcp", ServiceName: "ssh"},
		{Port: 2222, Protocol: "tcp", ServiceName: "ssh"},
		{Port: 445, Protocol: "tcp", ServiceName: "microsoft-ds"},
		{Port: 80, Protocol: "tcp", ServiceName: "http"},
	}, "ssh")
	if len(services) != 2 || services[0].Port != 22 || services[1].Port != 2222 {
		t.Fatalf("services = %+v", services)
	}
}

func TestBuildServiceHydraArgs(t *testing.T) {
	args := buildServiceHydraArgs("smb", "10.0.0.5", 1445, "/tmp/passwords.txt", "john", 0)
	want := []string{"-l", "john", "-P", "/tmp/passwords.txt", "-f", "-s", "1445", "10.0.0.5", "smb2"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestBuildSSHServiceHydraArgsUsesRecommendedTasks(t *testing.T) {
	args := buildServiceHydraArgs("ssh", "10.0.0.5", 22, "/tmp/passwords.txt", "john", 0)
	want := []string{"-l", "john", "-P", "/tmp/passwords.txt", "-t", "4", "-f", "-s", "22", "10.0.0.5", "ssh"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}

	args = buildServiceCredentialArgs("ssh", "10.0.0.5", 22, "", "secret", "/tmp/users.txt", "", 8)
	want = []string{"-L", "/tmp/users.txt", "-p", "secret", "-t", "8", "-f", "-s", "22", "10.0.0.5", "ssh"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("username search args = %#v, want %#v", args, want)
	}
}

func TestFilteredPasswordWordlistSkipsSharedWords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "passwords.txt")
	if err := os.WriteFile(path, []byte("secret\npassword\nsecret\n\n"), 0600); err != nil {
		t.Fatal(err)
	}
	words, err := filteredPasswordWordlist(path, map[string]struct{}{"password": {}})
	if err != nil {
		t.Fatalf("filteredPasswordWordlist() error = %v", err)
	}
	if !reflect.DeepEqual(words, []string{"secret"}) {
		t.Fatalf("words = %#v", words)
	}
}

func TestFormSpecReplacesOnlySelectedFields(t *testing.T) {
	form, err := formSpec("username=john&password=aaa&theme=default&language=ja", "username", "password")
	if err != nil {
		t.Fatalf("formSpec() error = %v", err)
	}
	if form.Body != "language=ja&password=^PASS^&theme=default&username=^USER^" {
		t.Fatalf("form body = %q", form.Body)
	}
}

func TestFormSpecUsesEmbeddedMarkers(t *testing.T) {
	form, err := formSpec("username=^USER^&password=^PASS^&theme=default", "", "")
	if err != nil {
		t.Fatalf("formSpec() error = %v", err)
	}
	if form.Body != "password=^PASS^&theme=default&username=^USER^" {
		t.Fatalf("form body = %q", form.Body)
	}
}

func TestFormSpecBuildsCredentialsOnlyBody(t *testing.T) {
	form, err := formSpec("", "login", "secret")
	if err != nil {
		t.Fatalf("formSpec() error = %v", err)
	}
	if form.Body != "login=^USER^&secret=^PASS^" {
		t.Fatalf("form body = %q", form.Body)
	}
}

func TestFormSpecRequiresMarkersWhenFieldsAreOmitted(t *testing.T) {
	if _, err := formSpec("username=john&password=secret", "", ""); err == nil {
		t.Fatal("formSpec() error = nil")
	}
}

func TestJSONFailureMarker(t *testing.T) {
	marker, err := jsonFailureMarker("status=error")
	if err != nil {
		t.Fatalf("jsonFailureMarker() error = %v", err)
	}
	if marker != `F="status"\:"error"` {
		t.Fatalf("marker = %q", marker)
	}
}

func TestJSONFailureMarkerRejectsInvalidInput(t *testing.T) {
	for _, value := range []string{"status", "=error", "status=", "status:error"} {
		if _, err := jsonFailureMarker(value); err == nil {
			t.Fatalf("jsonFailureMarker(%q) error = nil", value)
		}
	}
}

func TestHydraSuccessPassword(t *testing.T) {
	password, ok := hydraSuccessPassword("[80][http-post-form] host login:john password:s3cret\n")
	if !ok || password != "s3cret" {
		t.Fatalf("hydraSuccessPassword() = %q, %v", password, ok)
	}
	if _, ok := hydraSuccessPassword("no valid password\n"); ok {
		t.Fatal("hydraSuccessPassword() found a password in failed output")
	}
}

func TestBuildHydraArgsUsesRequestHeaders(t *testing.T) {
	template := requestTemplate{
		Method: "POST",
		URL:    mustURL("http://10.0.0.1:8080/login"),
		Header: map[string][]string{
			"Cookie":     {"sid=abc"},
			"User-Agent": {"Mozilla/5.0 (rv:152.0)"},
		},
	}
	args := buildHydraArgs(template, formTemplate{Body: "password=%5EPASS%5E&username=%5EUSER%5E"}, `F="status"\:"error"`, nil, "/tmp/list", "john")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "http-post-form") ||
		!strings.Contains(joined, "H=Cookie\\: sid=abc") ||
		!strings.Contains(joined, "H=User-Agent\\: Mozilla/5.0 (rv\\:152.0)") {
		t.Fatalf("hydra args = %#v", args)
	}
	if strings.Index(joined, `H=Cookie\: sid=abc`) > strings.Index(joined, `F="status"\:"error"`) {
		t.Fatalf("failure condition must be last: %q", joined)
	}
}

func TestResponseCondition(t *testing.T) {
	condition, optional, err := responseCondition(parsedOptions{SuccessJSON: "status=success"})
	if err != nil {
		t.Fatalf("responseCondition() error = %v", err)
	}
	if condition != `S="status"\:"success"` || len(optional) != 0 {
		t.Fatalf("condition = %q, optional = %#v", condition, optional)
	}

	condition, optional, err = responseCondition(parsedOptions{FailStatus: "401", SuccessRedirect: true})
	if err != nil {
		t.Fatalf("responseCondition() error = %v", err)
	}
	if condition != "" || strings.Join(optional, ":") != "1=:2=" {
		t.Fatalf("condition = %q, optional = %#v", condition, optional)
	}
}

func TestResponseConditionRejectsMultipleBodyMatchers(t *testing.T) {
	if _, _, err := responseCondition(parsedOptions{FailBody: "invalid", SuccessBody: "welcome"}); err == nil {
		t.Fatal("responseCondition() error = nil")
	}
}

func TestAutomaticWordlistsUseCtxRecommendation(t *testing.T) {
	original := recommendWordlists
	t.Cleanup(func() { recommendWordlists = original })
	var requested []string
	recommendWordlists = func(kind string) ([]ctx.WordlistSelection, error) {
		requested = append(requested, kind)
		return []ctx.WordlistSelection{{Path: "/lists/" + kind + ".txt"}}, nil
	}
	password, _, err := resolvePasswordList("")
	if err != nil {
		t.Fatal(err)
	}
	users, err := usernameWordlistCandidates("")
	if err != nil {
		t.Fatal(err)
	}
	if password != "/lists/password.txt" || len(users) != 1 || users[0].Path != "/lists/username.txt" || strings.Join(requested, ",") != "password,username" {
		t.Fatalf("password = %q, users = %+v, requested = %+v", password, users, requested)
	}
}

func mustURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}
