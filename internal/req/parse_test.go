
package req

import (
	"os"
	"strings"
	"testing"
)

func writeTempReqFile(t *testing.T, content string) string {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "*.req")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}

	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	return file.Name()
}

func TestParseFileParsesBasicRequest(t *testing.T) {
	t.Parallel()

	reqText := strings.Join([]string{
		"POST /submit?x=1 HTTP/1.1",
		"Host: example.com",
		"User-Agent: ReqTest/1.0",
		"Content-Type: text/plain",
		"Accept-Encoding: gzip, deflate",
		"Content-Length: 999",
		"Connection: keep-alive",
		"",
		"hello",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if parsed.Method != "POST" {
		t.Fatalf("Method = %q, want %q", parsed.Method, "POST")
	}
	if parsed.URL.String() != "http://example.com/submit?x=1" {
		t.Fatalf("URL = %q, want %q", parsed.URL.String(), "http://example.com/submit?x=1")
	}
	if parsed.Host != "example.com" {
		t.Fatalf("Host = %q, want %q", parsed.Host, "example.com")
	}
	if parsed.Header.Get("User-Agent") != "ReqTest/1.0" {
		t.Fatalf("User-Agent = %q, want %q", parsed.Header.Get("User-Agent"), "ReqTest/1.0")
	}
	if parsed.Header.Get("Content-Type") != "text/plain" {
		t.Fatalf("Content-Type = %q, want %q", parsed.Header.Get("Content-Type"), "text/plain")
	}
	if parsed.Header.Get("Accept-Encoding") != "" {
		t.Fatalf("Accept-Encoding = %q, want empty", parsed.Header.Get("Accept-Encoding"))
	}
	if parsed.Header.Get("Content-Length") != "" {
		t.Fatalf("Content-Length = %q, want empty", parsed.Header.Get("Content-Length"))
	}
	if parsed.Header.Get("Connection") != "" {
		t.Fatalf("Connection = %q, want empty", parsed.Header.Get("Connection"))
	}
	if string(parsed.Body) != "hello" {
		t.Fatalf("Body = %q, want %q", string(parsed.Body), "hello")
	}
	if parsed.ProtoMajor != 1 || parsed.ProtoMinor != 1 {
		t.Fatalf("protocol = HTTP/%d.%d, want HTTP/1.1", parsed.ProtoMajor, parsed.ProtoMinor)
	}
}

func TestParseFileKeepsMultipartBody(t *testing.T) {
	t.Parallel()

	body := strings.Join([]string{
		"------boundary",
		`Content-Disposition: form-data; name="file"; filename="test.php"`,
		"Content-Type: text/php",
		"",
		"GIF89a;<?php echo 'test'; ?>",
		"------boundary--",
		"",
	}, "\n")

	reqText := strings.Join([]string{
		"POST /upload HTTP/1.1",
		"Host: example.com",
		"Content-Type: multipart/form-data; boundary=----boundary",
		"",
		body,
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if string(parsed.Body) != body {
		t.Fatalf("multipart body changed\n got: %q\nwant: %q", string(parsed.Body), body)
	}
}

func TestParseFileAcceptsHTTP2RequestLine(t *testing.T) {
	t.Parallel()

	reqText := strings.Join([]string{
		"GET / HTTP/2",
		"Host: example.com",
		"",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if parsed.ProtoMajor != 2 || parsed.ProtoMinor != 0 {
		t.Fatalf("protocol = HTTP/%d.%d, want HTTP/2.0", parsed.ProtoMajor, parsed.ProtoMinor)
	}
}

func TestParseFileAcceptsHTTP20RequestLine(t *testing.T) {
	t.Parallel()

	reqText := strings.Join([]string{
		"GET / HTTP/2.0",
		"Host: example.com",
		"",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if parsed.ProtoMajor != 2 || parsed.ProtoMinor != 0 {
		t.Fatalf("protocol = HTTP/%d.%d, want HTTP/2.0", parsed.ProtoMajor, parsed.ProtoMinor)
	}
}

func TestParseFileUsesOriginScheme(t *testing.T) {
	t.Parallel()

	reqText := strings.Join([]string{
		"GET / HTTP/2",
		"Host: example.com",
		"Origin: https://example.com",
		"",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if parsed.URL.String() != "https://example.com/" {
		t.Fatalf("URL = %q, want %q", parsed.URL.String(), "https://example.com/")
	}
}

func TestParseFileUsesRefererScheme(t *testing.T) {
	t.Parallel()

	reqText := strings.Join([]string{
		"GET /path HTTP/2",
		"Host: example.com",
		"Referer: https://example.com/from",
		"",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if parsed.URL.String() != "https://example.com/path" {
		t.Fatalf("URL = %q, want %q", parsed.URL.String(), "https://example.com/path")
	}
}

func TestParseFileWithOptionsForceHTTPS(t *testing.T) {
	t.Parallel()

	reqText := strings.Join([]string{
		"GET / HTTP/2",
		"Host: example.com",
		"",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	parsed, err := ParseFileWithOptions(filename, ParseOptions{ForceHTTPS: true})
	if err != nil {
		t.Fatalf("ParseFileWithOptions() error = %v", err)
	}

	if parsed.URL.String() != "https://example.com/" {
		t.Fatalf("URL = %q, want %q", parsed.URL.String(), "https://example.com/")
	}
}
