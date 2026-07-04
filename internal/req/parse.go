package req

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"
)

var allowedMethods = map[string]struct{}{
	http.MethodGet:     {},
	http.MethodPost:    {},
	http.MethodPut:     {},
	http.MethodDelete:  {},
	http.MethodPatch:   {},
	http.MethodOptions: {},
	http.MethodHead:    {},
}

type ParsedRequest struct {
	Method     string
	URL        *url.URL
	Header     http.Header
	Host       string
	Body       []byte
	ProtoMajor int
	ProtoMinor int
}

type ParseOptions struct {
	ForceHTTPS bool
}

func ParseFile(filename string) (*ParsedRequest, error) {
	return ParseFileWithOptions(filename, ParseOptions{})
}

func ParseFileWithOptions(filename string, opts ParseOptions) (*ParsedRequest, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filename, err)
	}

	raw := strings.ReplaceAll(string(content), "\r\n", "\n")
	parts := strings.SplitN(raw, "\n\n", 2)
	headerPart := parts[0]
	bodyPart := ""
	if len(parts) == 2 {
		bodyPart = parts[1]
	}

	scanner := bufio.NewScanner(strings.NewReader(headerPart))
	if !scanner.Scan() {
		return nil, errors.New("invalid request file: missing request line")
	}

	requestLine := strings.TrimSpace(scanner.Text())
	method, target, protoMajor, protoMinor, err := parseRequestLine(requestLine)
	if err != nil {
		return nil, err
	}

	header := make(http.Header)
	var hostHeader string
	var originHeader string
	var refererHeader string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header line: %s", line)
		}

		name = textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
		value = strings.TrimSpace(value)

		switch strings.ToLower(name) {
		case "host":
			hostHeader = value
		case "origin":
			originHeader = value
			header.Add(name, value)
		case "referer":
			refererHeader = value
			header.Add(name, value)
		case "accept-encoding", "content-length":
			continue
		case "proxy-connection", "connection", "if-modified-since", "if-none-match":
			continue
		default:
			header.Add(name, value)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse request file: %w", err)
	}

	reqURL, resolvedHost, err := buildURL(target, hostHeader, originHeader, refererHeader, opts.ForceHTTPS)
	if err != nil {
		return nil, err
	}

	if hostHeader == "" {
		hostHeader = resolvedHost
	}

	return &ParsedRequest{
		Method:     method,
		URL:        reqURL,
		Header:     header,
		Host:       hostHeader,
		Body:       []byte(bodyPart),
		ProtoMajor: protoMajor,
		ProtoMinor: protoMinor,
	}, nil
}

func parseRequestLine(line string) (string, string, int, int, error) {
	fields := strings.Fields(line)
	if len(fields) != 3 {
		return "", "", 0, 0, fmt.Errorf("invalid request line: %s", line)
	}

	method := strings.ToUpper(fields[0])
	if _, ok := allowedMethods[method]; !ok {
		return "", "", 0, 0, fmt.Errorf("unsupported method: %s", method)
	}

	version := fields[2]
	if !strings.HasPrefix(version, "HTTP/") {
		return "", "", 0, 0, fmt.Errorf("unsupported protocol version: %s", version)
	}

	switch version {
	case "HTTP/1.0":
		return method, fields[1], 1, 0, nil
	case "HTTP/1.1":
		return method, fields[1], 1, 1, nil
	case "HTTP/2", "HTTP/2.0":
		return method, fields[1], 2, 0, nil
	default:
		return "", "", 0, 0, fmt.Errorf("unsupported protocol version: %s", version)
	}
}

func buildURL(target, hostHeader, originHeader, refererHeader string, forceHTTPS bool) (*url.URL, string, error) {
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		parsed, err := url.Parse(target)
		if err != nil {
			return nil, "", fmt.Errorf("invalid target URL: %w", err)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, "", fmt.Errorf("unsupported URL scheme: %s", parsed.Scheme)
		}
		if parsed.Host == "" {
			return nil, "", errors.New("invalid target URL: missing host")
		}
		if parsed.Path == "" {
			parsed.Path = "/"
		}
		return parsed, parsed.Host, nil
	}

	if hostHeader == "" {
		return nil, "", errors.New("invalid request file: missing Host header")
	}

	scheme := resolveScheme(hostHeader, originHeader, refererHeader, forceHTTPS)

	if target == "*" {
		return &url.URL{Scheme: scheme, Host: hostHeader, Path: "*"}, hostHeader, nil
	}

	if !strings.HasPrefix(target, "/") {
		target = "/" + strings.TrimLeft(target, "/")
	}

	u, err := url.Parse(fmt.Sprintf("%s://%s%s", scheme, hostHeader, target))
	if err != nil {
		return nil, "", fmt.Errorf("invalid target URL: %w", err)
	}
	if u.Path == "" {
		u.Path = "/"
	}

	return u, hostHeader, nil
}

func resolveScheme(hostHeader, originHeader, refererHeader string, forceHTTPS bool) string {
	if forceHTTPS {
		return "https"
	}

	if scheme := schemeFromURL(originHeader); scheme != "" {
		return scheme
	}

	if scheme := schemeFromURL(refererHeader); scheme != "" {
		return scheme
	}

	if _, portProvided := splitHostPortLikeSQLMap(hostHeader); portProvided == "443" {
		return "https"
	}

	return "http"
}

func schemeFromURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		return parsed.Scheme
	}

	return ""
}

func splitHostPortLikeSQLMap(host string) (string, string) {
	if strings.HasPrefix(host, "[") {
		end := strings.LastIndex(host, "]")
		if end == -1 {
			return host, ""
		}
		if len(host) > end+1 && host[end+1] == ':' {
			return host[:end+1], host[end+2:]
		}
		return host, ""
	}

	lastColon := strings.LastIndex(host, ":")
	if lastColon == -1 {
		return host, ""
	}

	if strings.Count(host, ":") > 1 {
		return host, ""
	}

	return host[:lastColon], host[lastColon+1:]
}
