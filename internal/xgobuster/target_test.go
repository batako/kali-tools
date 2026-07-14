package xgobuster

import (
	"strings"
	"testing"
)

func TestServiceURL(t *testing.T) {
	https := "https"
	if got := serviceURL("10.0.0.1", Service{Port: 443, ServiceName: &https}); got != "https://10.0.0.1" {
		t.Fatalf("serviceURL(https) = %q", got)
	}
	http := "http"
	if got := serviceURL("10.0.0.1", Service{Port: 8080, ServiceName: &http}); got != "http://10.0.0.1:8080" {
		t.Fatalf("serviceURL(http:8080) = %q", got)
	}
}

func TestResolveURLDefaultsAndSelectsWebService(t *testing.T) {
	var output strings.Builder
	if got, err := resolveURL("10.0.0.1", nil, strings.NewReader(""), &output); err == nil || got != "" || !strings.Contains(err.Error(), "run xscan first") {
		t.Fatalf("resolveURL(without services) = %q, %v", got, err)
	}
	http := "http"
	https := "https"
	got, err := resolveURL("10.0.0.1", []Service{
		{Port: 80, ServiceName: &http},
		{Port: 443, ServiceName: &https},
	}, strings.NewReader("2\n"), &output)
	if err != nil {
		t.Fatalf("resolveURL(selection) error = %v", err)
	}
	if got != "https://10.0.0.1" {
		t.Fatalf("resolveURL(selection) = %q", got)
	}
	if !strings.Contains(output.String(), "Select a web service:") {
		t.Fatalf("selection output = %q", output.String())
	}
}
