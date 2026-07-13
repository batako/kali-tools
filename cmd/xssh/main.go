package main

import (
	"errors"
	"fmt"
	"os"

	internalxssh "req/internal/xssh"
)

func main() {
	if err := internalxssh.RunWithIO(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		var exitErr internalxssh.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
