package main

import (
	"errors"
	"fmt"
	"os"

	internalctx "req/internal/ctx"
)

func main() {
	if err := internalctx.RunWithIO(os.Args, os.Stdout, os.Stderr); err != nil {
		var exitErr internalctx.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
