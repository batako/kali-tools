package req

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
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
	if err.Error() != "usage: req <REQ_FILE>" {
		t.Fatalf("Run() error = %q, want %q", err.Error(), "usage: req <REQ_FILE>")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
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
