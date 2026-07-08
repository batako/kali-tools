package ctx

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const usageText = "usage: ctx <command>\n\ncommands:\n  init    create a ctx workspace in the current directory\n  status  show the current ctx workspace\n  help    show this help"

func Run(args []string, stdout io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: ctx <command>")
	}

	switch args[1] {
	case "init":
		if len(args) != 2 {
			return errors.New("usage: ctx init")
		}
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		workspace, err := InitWorkspace(wd)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "initialized ctx workspace %s\n", workspace.ID)
		return err
	case "status":
		if len(args) != 2 {
			return errors.New("usage: ctx status")
		}
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		workspace, err := FindWorkspace(wd)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "workspace: %s\nroot: %s\ndata: %s\n", workspace.ID, workspace.RootPath, workspace.DataPath)
		return err
	case "help", "-h", "--help":
		_, err := fmt.Fprintln(stdout, usageText)
		return err
	default:
		return fmt.Errorf("unknown ctx command: %s", args[1])
	}
}
