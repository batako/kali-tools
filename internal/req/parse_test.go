package req

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFileParsesBasicRequest(t *testing.T) {
	t.Parallel()

	reqText := strings.Join([]string{
		"POST /login HTTP/1.1",
		"Host: example.com",
		"User-Agent: TestAgent/1.0",
		"Content-Type: application/x-www-form-urlencoded",
		"",
		"username=admin&password=test",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if parsed.Method != "POST" {
		t.Fatalf("parsed.Method = %q, want %q", parsed.Method, "POST")
	}
	if parsed.URL.String() != "http://example.com/login" {
		t.Fatalf("parsed.URL.String() = %q, want %q", parsed.URL.String(), "http://example.com/login")
	}
	if parsed.Host != "example.com" {
		t.Fatalf("parsed.Host = %q, want %q", parsed.Host, "example.com")
	}
	if got := parsed.Header.Get("User-Agent"); got != "TestAgent/1.0" {
		t.Fatalf("User-Agent = %q, want %q", got, "TestAgent/1.0")
	}
	if got := parsed.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/x-www-form-urlencoded")
	}
	if string(parsed.Body) != "username=admin&password=test" {
		t.Fatalf("parsed.Body = %q, want %q", string(parsed.Body), "username=admin&password=test")
	}
}

func TestParseFileExcludesAcceptEncodingAndContentLength(t *testing.T) {
	t.Parallel()

	reqText := strings.Join([]string{
		"POST /submit HTTP/1.1",
		"Host: example.com",
		"Accept-Encoding: gzip, deflate",
		"Content-Length: 999",
		"Content-Type: text/plain",
		"",
		"hello",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if got := parsed.Header.Get("Accept-Encoding"); got != "" {
		t.Fatalf("Accept-Encoding = %q, want empty", got)
	}
	if got := parsed.Header.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want empty", got)
	}
	if got := parsed.Header.Get("Content-Type"); got != "text/plain" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/plain")
	}
}

func TestParseFilePreservesMultipartBody(t *testing.T) {
	t.Parallel()

	body := strings.Join([]string{
		"------geckoformboundary571eda73ef295382c50dfbcc925140fe",
		`Content-Disposition: form-data; name="file"; filename="test.php"`,
		"Content-Type: text/php",
		"",
		"GIF89a;<?php echo 'test'; ?>",
		"",
		"------geckoformboundary571eda73ef295382c50dfbcc925140fe--",
		"",
	}, "\n")

	reqText := strings.Join([]string{
		"POST /cv.php HTTP/1.1",
		"Host: 10.144.131.185",
		"Content-Type: multipart/form-data; boundary=----geckoformboundary571eda73ef295382c50dfbcc925140fe",
		"",
		body,
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if string(parsed.Body) != body {
		t.Fatalf("parsed.Body mismatch\n got: %q\nwant: %q", string(parsed.Body), body)
	}
}

func writeTempReqFile(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	filename := filepath.Join(dir, "test.req")
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return filename
}
