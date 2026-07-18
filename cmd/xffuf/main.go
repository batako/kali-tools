package main

import (
	"errors"
	"fmt"
	"os"

	internalxffuf "req/internal/xffuf"
)

func main() {
	if err := internalxffuf.RunWithIO(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		var exitErr internalxffuf.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
