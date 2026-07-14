package req

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSendWithHTTPTestServer(t *testing.T) {
	t.Parallel()

	var receivedMethod string
	var receivedPath string
	var receivedHost string
	var receivedUserAgent string
	var receivedBody string
	var receivedBodyReadErr error

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.RequestURI()
		receivedHost = r.Host
		receivedUserAgent = r.Header.Get("User-Agent")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			receivedBodyReadErr = err
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		receivedBody = string(body)

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	reqText := strings.Join([]string{
		"POST /submit?x=1 HTTP/1.1",
		"Host: " + host,
		"User-Agent: ReqTest/1.0",
		"Accept-Encoding: gzip",
		"Content-Length: 123",
		"Content-Type: text/plain",
		"",
		"payload-body",
	}, "\n")

	filename := writeTempReqFile(t, reqText)
	parsed, err := ParseFile(filename)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	resp, err := send(parsed)
	if err != nil {
		t.Fatalf("send() error = %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if receivedMethod != "POST" {
		t.Fatalf("receivedMethod = %q, want %q", receivedMethod, "POST")
	}
	if receivedPath != "/submit?x=1" {
		t.Fatalf("receivedPath = %q, want %q", receivedPath, "/submit?x=1")
	}
	if receivedHost != host {
		t.Fatalf("receivedHost = %q, want %q", receivedHost, host)
	}
	if receivedUserAgent != "ReqTest/1.0" {
		t.Fatalf("receivedUserAgent = %q, want %q", receivedUserAgent, "ReqTest/1.0")
	}
	if receivedBody != "payload-body" {
		t.Fatalf("receivedBody = %q, want %q", receivedBody, "payload-body")
	}
	if receivedBodyReadErr != nil {
		t.Fatalf("receivedBodyReadErr = %v, want nil", receivedBodyReadErr)
	}
	if string(respBody) != "ok" {
		t.Fatalf("response body = %q, want %q", string(respBody), "ok")
	}
}

func TestSendWithOptionsCanSkipTLSVerification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	parsed := &ParsedRequest{
		Method: http.MethodGet,
		URL:    parsedURL,
		Host:   host,
		Header: make(http.Header),
	}
	resp, err := sendWithOptions(parsed, true)
	if err != nil {
		t.Fatalf("sendWithOptions() error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "ok" {
		t.Fatalf("response body = %q, %v", body, err)
	}
}

func TestWriteResponseFormatsHTTPResponse(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
			"X-Test":       []string{"yes"},
		},
		Body: io.NopCloser(strings.NewReader("hello")),
	}

	var buf bytes.Buffer
	if err := writeResponse(&buf, resp); err != nil {
		t.Fatalf("writeResponse() error = %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "HTTP/1.1 200 OK\r\n") {
		t.Fatalf("output prefix = %q", output)
	}
	if !strings.Contains(output, "Content-Type: text/plain\r\n") {
		t.Fatalf("output missing Content-Type header: %q", output)
	}
	if !strings.Contains(output, "X-Test: yes\r\n") {
		t.Fatalf("output missing X-Test header: %q", output)
	}
	if !strings.HasSuffix(output, "\r\nhello") {
		t.Fatalf("output suffix = %q", output)
	}
}

func TestRunReturnsUsageErrorWhenArgumentMissing(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run([]string{"req"}, &stdout)
	if err == nil {
		t.Fatal("Run() error = nil, want usage error")
	}
	wantUsage := "usage: req [-S|--https] [-k|--no-tls-validation] [--tls-verify] <REQ_FILE>"
	if err.Error() != wantUsage {
		t.Fatalf("Run() error = %q, want %q", err.Error(), wantUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunWritesVersion(t *testing.T) {
	oldVersion := Version
	Version = "2.3.4"
	t.Cleanup(func() { Version = oldVersion })

	var stdout bytes.Buffer
	for _, arg := range []string{"-V", "--version"} {
		stdout.Reset()
		if err := Run([]string{"req", arg}, &stdout); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if got, want := stdout.String(), "req 2.3.4\n"; got != want {
			t.Fatalf("version output = %q, want %q", got, want)
		}
	}
}

func TestRunWritesHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	if err := Run([]string{"req", "-h"}, &stdout); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "usage: req [-S|--https] [-k|--no-tls-validation] [--tls-verify] <REQ_FILE>") {
		t.Fatalf("help output = %q", output)
	}
	if !strings.Contains(output, "-S, --https") {
		t.Fatalf("help output missing -S description: %q", output)
	}
	if !strings.Contains(output, "-h, --help") {
		t.Fatalf("help output missing help description: %q", output)
	}
	if !strings.Contains(output, "-V, --version") {
		t.Fatalf("help output missing version description: %q", output)
	}
	if !strings.Contains(output, "-k, --no-tls-validation") || !strings.Contains(output, "--tls-verify") {
		t.Fatalf("help output missing TLS options: %q", output)
	}
}

func TestRunWritesFormattedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "response-body")
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	reqText := strings.Join([]string{
		"GET / HTTP/1.1",
		"Host: " + host,
		"",
		"",
	}, "\n")

	filename := writeTempReqFile(t, reqText)

	var stdout bytes.Buffer
	if err := Run([]string{"req", filename}, &stdout); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	output := stdout.String()
	if !strings.HasPrefix(output, "HTTP/1.1 200 OK\r\n") {
		t.Fatalf("output prefix = %q", output)
	}
	if !strings.Contains(output, "Content-Type: text/plain\r\n") {
		t.Fatalf("output missing Content-Type header: %q", output)
	}
	if !strings.HasSuffix(output, "\r\nresponse-body") {
		t.Fatalf("output suffix = %q", output)
	}
}
