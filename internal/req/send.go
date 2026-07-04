package req

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
)

func Run(args []string, stdout io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: req <REQ_FILE>")
	}

	parsed, err := ParseFile(args[1])
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
