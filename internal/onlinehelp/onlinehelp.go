package onlinehelp

import (
	"fmt"
	"io"
)

const repositoryURL = "https://github.com/batako/kali-tools"

func URL(command, version string) string {
	return fmt.Sprintf("%s/blob/%s/v%s/docs/commands/%s.md", repositoryURL, command, version, command)
}

func Print(stdout io.Writer, command, version string) error {
	_, err := fmt.Fprintf(stdout, "Access the online help page for %s %s:\n%s\n", command, version, URL(command, version))
	return err
}
