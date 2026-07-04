package req

import (
	"fmt"
	"io"
	"net/http"
)

func writeResponse(w io.Writer, resp *http.Response) error {
	if _, err := fmt.Fprintf(w, "HTTP/%d.%d %s\r\n", resp.ProtoMajor, resp.ProtoMinor, resp.Status); err != nil {
		return err
	}

	if err := resp.Header.Write(w); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "\r\n"); err != nil {
		return err
	}

	_, err := io.Copy(w, resp.Body)
	return err
}
