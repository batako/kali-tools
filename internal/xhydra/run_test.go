package xhydra

import (
	"net/url"
	"strings"
	"testing"
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

func mustURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}
