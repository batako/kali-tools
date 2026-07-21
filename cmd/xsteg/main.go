package main

import (
	"fmt"
	"os"

	"req/internal/xsteg"
)

func main() {
	if err := xsteg.Run(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
