package main

import (
	"os"

	internalctx "req/internal/ctx"
)

func main() {
	os.Exit(internalctx.RunCLI(os.Args, os.Stdin, os.Stdout, os.Stderr))
}
