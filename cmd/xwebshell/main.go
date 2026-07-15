package main

import (
	"fmt"
	"os"

	"req/internal/xwebshell"
)

func main() {
	if err := xwebshell.Run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
