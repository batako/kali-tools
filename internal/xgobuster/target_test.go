package xgobuster

import (
	"strings"
	"testing"

	"req/internal/ctx"
)

func TestResolveTargetHostUsesManualXHost(t *testing.T) {
	hosts := []ctx.Host{
		{Hostname: "example.test", TargetIP: "10.0.0.1", Source: "manual"},
	}
	if got, err := resolveTargetHost("10.0.0.1", hosts, "", false, strings.NewReader(""), &strings.Builder{}); err != nil || got != "example.test" {
		t.Fatalf("resolveTargetHost() = %q, %v, want xhost hostname", got, err)
	}
}

func TestResolveTargetHostFallsBackToIP(t *testing.T) {
	hosts := []ctx.Host{
		{Hostname: "discovered.test", TargetIP: "10.0.0.1", Source: "scan"},
	}
	if got, err := resolveTargetHost("10.0.0.1", hosts, "", false, strings.NewReader(""), &strings.Builder{}); err != nil || got != "10.0.0.1" {
		t.Fatalf("resolveTargetHost() = %q, %v, want target IP", got, err)
	}
}

func TestResolveTargetHostSelectsIPAmongMultipleHosts(t *testing.T) {
	hosts := []ctx.Host{
		{Hostname: "example.test", TargetIP: "10.0.0.1", Source: "manual"},
		{Hostname: "admin.example.test", TargetIP: "10.0.0.1", Source: "manual"},
	}
	var output strings.Builder
	got, err := resolveTargetHost("10.0.0.1", hosts, "", false, strings.NewReader("3\n"), &output)
	if err != nil || got != "10.0.0.1" {
		t.Fatalf("resolveTargetHost() = %q, %v, want target IP", got, err)
	}
	if !strings.Contains(output.String(), "10.0.0.1 (IP)") {
		t.Fatalf("selection output = %q", output.String())
	}
}

func TestResolveTargetHostHonorsExplicitHostAndIP(t *testing.T) {
	hosts := []ctx.Host{{Hostname: "example.test", TargetIP: "10.0.0.1", Source: "manual"}}
	if got, err := resolveTargetHost("10.0.0.1", hosts, "example.test", false, strings.NewReader(""), &strings.Builder{}); err != nil || got != "example.test" {
		t.Fatalf("explicit host = %q, %v", got, err)
	}
	if got, err := resolveTargetHost("10.0.0.1", hosts, "", true, strings.NewReader(""), &strings.Builder{}); err != nil || got != "10.0.0.1" {
		t.Fatalf("forced IP = %q, %v", got, err)
	}
}

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
	if got, err := resolveURL("10.0.0.1", nil, 0, strings.NewReader(""), &output); err == nil || got != "" || !strings.Contains(err.Error(), "run xscan first") {
		t.Fatalf("resolveURL(without services) = %q, %v", got, err)
	}
	http := "http"
	https := "https"
	got, err := resolveURL("10.0.0.1", []Service{
		{Port: 80, ServiceName: &http},
		{Port: 443, ServiceName: &https},
	}, 0, strings.NewReader("2\n"), &output)
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

func TestResolveURLSelectsServiceByNumber(t *testing.T) {
	http := "http"
	got, err := resolveURL("10.0.0.1", []Service{
		{Port: 80, ServiceName: &http},
		{Port: 62337, ServiceName: &http},
	}, 2, strings.NewReader(""), &strings.Builder{})
	if err != nil {
		t.Fatalf("resolveURL() error = %v", err)
	}
	if got != "http://10.0.0.1:62337" {
		t.Fatalf("resolveURL() = %q, want selected service", got)
	}
}

func TestResolveURLRejectsInvalidServiceNumber(t *testing.T) {
	http := "http"
	_, err := resolveURL("10.0.0.1", []Service{{Port: 80, ServiceName: &http}}, 2, strings.NewReader(""), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "choose 1-1") {
		t.Fatalf("resolveURL() error = %v, want invalid selection", err)
	}
}
