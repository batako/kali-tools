package req

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var Version = "dev"

const usageText = "usage: req [-S|--https] <REQ_FILE>\n\noptions:\n  -S, --https  force https when the request file does not imply a scheme\n  -h, --help   show this help\n  -V, --version  show version"

func Run(args []string, stdout io.Writer) error {
	forceHTTPS := false
	var reqFile string

	for _, arg := range args[1:] {
		switch arg {
		case "-S", "--https":
			forceHTTPS = true
		case "-h", "--help":
			_, err := fmt.Fprintln(stdout, usageText)
			return err
		case "-V", "--version":
			_, err := fmt.Fprintf(stdout, "req %s\n", Version)
			return err
		default:
			if reqFile != "" {
				return errors.New("usage: req [-S|--https] <REQ_FILE>")
			}
			reqFile = arg
		}
	}

	if reqFile == "" {
		return errors.New("usage: req [-S|--https] <REQ_FILE>")
	}

	parsed, err := ParseFileWithOptions(reqFile, ParseOptions{ForceHTTPS: forceHTTPS})
	if err != nil {
		return err
	}

	resp, err := send(parsed)
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

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}
