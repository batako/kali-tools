package main

import (
	"errors"
	"fmt"
	"os"

	"req/internal/xhydra"
)

func main() {
	if err := xhydra.Run(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		var exitErr xhydra.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
