package xgobuster

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"req/internal/ctx"
)

func resolveTargetHost(targetIP string, hosts []ctx.Host, requested string, forceIP bool, stdin io.Reader, stdout io.Writer) (string, error) {
	if forceIP {
		return targetIP, nil
	}

	registered := make([]string, 0, len(hosts))
	seen := make(map[string]struct{})
	for _, host := range hosts {
		if host.TargetIP != targetIP || host.Source != "manual" || strings.TrimSpace(host.Hostname) == "" {
			continue
		}
		name := host.Hostname
		key := strings.ToLower(strings.TrimSuffix(name, "."))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		registered = append(registered, name)
	}
	if requested != "" {
		requestedKey := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(requested), "."))
		for _, name := range registered {
			if strings.ToLower(strings.TrimSuffix(name, ".")) == requestedKey {
				return name, nil
			}
		}
		return "", fmt.Errorf("xhost hostname not found for target %s: %s", targetIP, requested)
	}
	if len(registered) == 0 || len(registered) == 1 {
		if len(registered) == 1 {
			return registered[0], nil
		}
		return targetIP, nil
	}

	_, _ = fmt.Fprintln(stdout, "Select a target host:")
	_, _ = fmt.Fprintln(stdout)
	for i, name := range registered {
		_, _ = fmt.Fprintf(stdout, "  %d) %s\n", i+1, name)
	}
	ipChoice := len(registered) + 1
	_, _ = fmt.Fprintf(stdout, "  %d) %s (IP)\n", ipChoice, targetIP)
	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintf(stdout, "Select [1-%d]: ", ipChoice)
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && len(strings.TrimSpace(line)) == 0 {
		return "", fmt.Errorf("cancelled")
	}
	index, convErr := strconv.Atoi(strings.TrimSpace(line))
	if convErr != nil || index < 1 || index > ipChoice {
		return "", fmt.Errorf("invalid target host selection")
	}
	if index == ipChoice {
		return targetIP, nil
	}
	return registered[index-1], nil
}

func resolveURL(targetIP string, services []Service, stdin io.Reader, stdout io.Writer) (string, error) {
	var candidates []Service
	for _, service := range services {
		name := ""
		if service.ServiceName != nil {
			name = strings.ToLower(*service.ServiceName)
		}
		if strings.Contains(name, "http") || service.Port == 80 || service.Port == 443 {
			candidates = append(candidates, service)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no HTTP service found; run xscan first or specify -u <url>")
	}
	if len(candidates) > 1 {
		_, _ = fmt.Fprintln(stdout, "Select a web service:")
		_, _ = fmt.Fprintln(stdout)
		for i, service := range candidates {
			_, _ = fmt.Fprintf(stdout, "  %d) %s\n", i+1, serviceURL(targetIP, service))
		}
		_, _ = fmt.Fprintln(stdout)
		_, _ = fmt.Fprint(stdout, "Select [1-", len(candidates), "]: ")
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && len(strings.TrimSpace(line)) == 0 {
			return "", fmt.Errorf("cancelled")
		}
		index, convErr := strconv.Atoi(strings.TrimSpace(line))
		if convErr != nil || index < 1 || index > len(candidates) {
			return "", fmt.Errorf("invalid web service selection")
		}
		return serviceURL(targetIP, candidates[index-1]), nil
	}
	return serviceURL(targetIP, candidates[0]), nil
}

func serviceURL(targetIP string, service Service) string {
	scheme := "http"
	name := ""
	if service.ServiceName != nil {
		name = strings.ToLower(*service.ServiceName)
	}
	if service.Port == 443 || strings.Contains(name, "https") {
		scheme = "https"
	}
	host := targetIP
	if parsed, err := url.Parse(scheme + "://" + targetIP); err == nil && parsed.Host != "" {
		host = parsed.Host
	}
	if (scheme == "http" && service.Port == 80) || (scheme == "https" && service.Port == 443) || service.Port == 0 {
		return scheme + "://" + host
	}
	return scheme + "://" + host + ":" + strconv.Itoa(service.Port)
}
