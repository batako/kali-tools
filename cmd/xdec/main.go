package main

import (
	"fmt"
	"os"

	"req/internal/xdec"
)

func main() {
	if err := xdec.Run(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
