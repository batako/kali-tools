package ctx

import (
	"errors"
	"fmt"
	"io"
	"os"
)

const usageText = `usage: ctx <command>

commands:
  init                 create a ctx workspace in the current directory
  status               show the current ctx workspace
  target set <ip>      create or update the primary target
  target add <ip>      add a target
  target update <ip>   update the current primary target IP
  target use <name>    make a target primary
  target rm <name>     remove a target
  target ls            list targets
  ip [ip]              show or update the primary target IP
  help                 show this help`

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
		workspace, err := currentWorkspace()
		if err != nil {
			return err
		}
		record, err := GetWorkspaceRecord(workspace)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "workspace: %s\nname: %s\nroot: %s\ndata: %s\ndatabase: %s\n", record.ID, record.Name, record.RootPath, workspace.DataPath, workspace.DatabasePath)
		return err
	case "target":
		return runTarget(args[2:], stdout)
	case "ip":
		return runIP(args[2:], stdout)
	case "help", "-h", "--help":
		_, err := fmt.Fprintln(stdout, usageText)
		return err
	default:
		return fmt.Errorf("unknown ctx command: %s", args[1])
	}
}

func runTarget(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: ctx target <set|add|update|use|rm|ls>")
	}

	workspace, err := currentWorkspace()
	if err != nil {
		return err
	}

	switch args[0] {
	case "set", "update":
		if len(args) != 2 {
			return fmt.Errorf("usage: ctx target %s <ip>", args[0])
		}
		target, err := SetPrimaryTargetIP(workspace, args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "primary target: %s %s\n", target.Name, target.IP)
		return err
	case "add":
		ip, name, err := parseTargetAddArgs(args[1:])
		if err != nil {
			return err
		}
		target, err := AddTarget(workspace, ip, name)
		if err != nil {
			return err
		}
		marker := " "
		if target.IsPrimary {
			marker = "*"
		}
		_, err = fmt.Fprintf(stdout, "%s %s %s\n", marker, target.Name, target.IP)
		return err
	case "use":
		if len(args) != 2 {
			return errors.New("usage: ctx target use <name>")
		}
		target, err := UseTarget(workspace, args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "primary target: %s %s\n", target.Name, target.IP)
		return err
	case "rm":
		if len(args) != 2 {
			return errors.New("usage: ctx target rm <name>")
		}
		if err := RemoveTarget(workspace, args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "removed target: %s\n", args[1])
		return err
	case "ls":
		if len(args) != 1 {
			return errors.New("usage: ctx target ls")
		}
		targets, err := ListTargets(workspace)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			_, err = fmt.Fprintln(stdout, "no targets")
			return err
		}
		for _, target := range targets {
			marker := " "
			if target.IsPrimary {
				marker = "*"
			}
			if _, err := fmt.Fprintf(stdout, "%s %s %s\n", marker, target.Name, target.IP); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown ctx target command: %s", args[0])
	}
}

func runIP(args []string, stdout io.Writer) error {
	workspace, err := currentWorkspace()
	if err != nil {
		return err
	}

	switch len(args) {
	case 0:
		target, err := GetPrimaryTarget(workspace)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, target.IP)
		return err
	case 1:
		target, err := SetPrimaryTargetIP(workspace, args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "primary target: %s %s\n", target.Name, target.IP)
		return err
	default:
		return errors.New("usage: ctx ip [ip]")
	}
}

func currentWorkspace() (*Workspace, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}
	return FindWorkspace(wd)
}

func parseTargetAddArgs(args []string) (string, string, error) {
	if len(args) < 1 {
		return "", "", errors.New("usage: ctx target add <ip> [--name <name>]")
	}

	ip := args[0]
	var name string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return "", "", errors.New("usage: ctx target add <ip> [--name <name>]")
			}
			name = args[i+1]
			i++
		default:
			return "", "", fmt.Errorf("unknown ctx target add option: %s", args[i])
		}
	}

	return ip, name, nil
}
