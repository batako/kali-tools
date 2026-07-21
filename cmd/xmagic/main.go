package main

import (
	"fmt"
	"os"

	"req/internal/xmagic"
)

func main() {
	if err := xmagic.Run(os.Args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
