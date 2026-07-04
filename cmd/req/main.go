package main

import (
	"fmt"
	"os"

	internalreq "req/internal/req"
)

func main() {
	if err := internalreq.Run(os.Args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
