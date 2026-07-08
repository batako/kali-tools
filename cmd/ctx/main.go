package main

import (
	"fmt"
	"os"

	internalctx "req/internal/ctx"
)

func main() {
	if err := internalctx.Run(os.Args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
