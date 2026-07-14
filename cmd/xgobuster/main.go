package main

import (
	"errors"
	"fmt"
	"os"

	internalxgobuster "req/internal/xgobuster"
)

func main() {
	if err := internalxgobuster.RunWithIO(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		var exitErr internalxgobuster.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
