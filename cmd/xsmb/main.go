package main

import (
	"errors"
	"fmt"
	"os"

	internalxsmb "req/internal/xsmb"
)

func main() {
	if err := internalxsmb.RunWithIO(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		var exitErr internalxsmb.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
