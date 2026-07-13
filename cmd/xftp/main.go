package main

import (
	"errors"
	"fmt"
	"os"

	internalxftp "req/internal/xftp"
)

func main() {
	if err := internalxftp.RunWithIO(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		var exitErr internalxftp.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
