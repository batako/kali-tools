package main

import (
	"fmt"
	"os"

	"req/internal/xscp"
)

func main() {
	if err := xscp.Run(os.Args, os.Stdin, os.Stdout, os.Stderr); err != nil {
		if exitErr, ok := err.(interface{ Code() int }); ok {
			os.Exit(exitErr.Code())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
