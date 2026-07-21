package ctx

import (
	"io"

	"req/internal/onlinehelp"
)

var printOnlineHelpFunc = printOnlineHelp

func onlineHelpURL() string {
	return onlinehelp.URL("ctx", Version)
}

func printOnlineHelp(stdout io.Writer) error {
	return onlinehelp.Print(stdout, "ctx", Version)
}
