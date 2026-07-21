package req

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"

	"req/internal/onlinehelp"
)

var Version = "1.1.0"

const usageText = "usage: req [-S|--https] [-k|--no-tls-validation] [--tls-verify] <REQ_FILE>\n\noptions:\n  -S, --https  force https when the request file does not imply a scheme\n  -k, --no-tls-validation  disable TLS certificate validation\n  --tls-verify  verify TLS certificates for this run\n  -h, --help   show this help\n  -V, --version  show version\n  --online-help  show the versioned online help URL"

func Run(args []string, stdout io.Writer) error {
	forceHTTPS := false
	insecureTLS := false
	verifyTLS := false
	var reqFile string

	for _, arg := range args[1:] {
		switch arg {
		case "-S", "--https":
			forceHTTPS = true
		case "-k", "--no-tls-validation":
			insecureTLS = true
		case "--tls-verify":
			verifyTLS = true
		case "-h", "--help":
			_, err := fmt.Fprintln(stdout, usageText)
			return err
		case "--online-help":
			return onlinehelp.Print(stdout, "req", Version)
		case "-V", "--version":
			_, err := fmt.Fprintf(stdout, "req %s\n", Version)
			return err
		default:
			if reqFile != "" {
				return errors.New("usage: req [-S|--https] [-k|--no-tls-validation] [--tls-verify] <REQ_FILE>")
			}
			reqFile = arg
		}
	}

	if reqFile == "" {
		return errors.New("usage: req [-S|--https] [-k|--no-tls-validation] [--tls-verify] <REQ_FILE>")
	}
	if insecureTLS && verifyTLS {
		return errors.New("usage: -k cannot be combined with --tls-verify")
	}

	parsed, err := ParseFileWithOptions(reqFile, ParseOptions{ForceHTTPS: forceHTTPS})
	if err != nil {
		return err
	}

	resp, err := sendWithOptions(parsed, insecureTLS)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := writeResponse(stdout, resp); err != nil {
		return fmt.Errorf("failed to write response: %w", err)
	}

	return nil
}

func send(parsed *ParsedRequest) (*http.Response, error) {
	return sendWithOptions(parsed, false)
}

func sendWithOptions(parsed *ParsedRequest, insecureTLS bool) (*http.Response, error) {
	req, err := http.NewRequest(parsed.Method, parsed.URL.String(), bytes.NewReader(parsed.Body))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}

	req.Host = parsed.Host
	req.Header = parsed.Header.Clone()
	req.ProtoMajor = parsed.ProtoMajor
	req.ProtoMinor = parsed.ProtoMinor

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if insecureTLS {
		transport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("request failed: unsupported default HTTP transport")
		}
		transport = transport.Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // #nosec G402: explicitly requested for test targets.
		client.Transport = transport
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}
