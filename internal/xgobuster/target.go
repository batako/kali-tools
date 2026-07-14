package xgobuster

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

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
