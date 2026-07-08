package ctx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

var (
	hostsFilePath                = "/etc/hosts"
	syncHostsFileFunc            = SyncHostsFile
	reexecHostsSyncWithSudoFunc  = reexecHostsSyncWithSudo
	cleanHostsFileFunc           = CleanHostsFile
	reexecHostsCleanWithSudoFunc = reexecHostsCleanWithSudo
	executableFunc               = os.Executable
	execCommandFunc              = exec.Command
)

const usageText = `usage: ctx <command> [options]

commands:
  init     create a ctx workspace
  status   show the current workspace
  target   manage targets
  ip       show or update the primary target IP
  host     manage hostnames
  hosts    show, sync, or clean /etc/hosts entries

options:
  -h, --help  show this help

Run ctx <command> -h for command-specific help.`

const initUsageText = `usage: ctx init [options]

Create a ctx workspace in the current directory.

options:
  -h, --help  show this help`

const statusUsageText = `usage: ctx status [options]

Show the current ctx workspace.

options:
  -h, --help  show this help`

const targetUsageText = `usage: ctx target <command> [options]

commands:
  set <ip>                 create or update the primary target
  add <ip> [--name <name>] add a target
  update <ip>              update the current primary target IP
  use <name>               make a target primary
  rm <name>                remove a target
  ls                       list targets

options:
  -h, --help               show this help`

const ipUsageText = `usage: ctx ip [ip] [options]

Show or update the primary target IP.

options:
  -h, --help  show this help`

const hostUsageText = `usage: ctx host <command> [options]

commands:
  add <hostname> [--target <name>] add a host
  rm <hostname>                    remove a host
  ls                               list hosts

options:
  -h, --help                       show this help`

const hostsUsageText = `usage: ctx hosts <command> [options]

commands:
  show   show the managed hosts block
  sync   sync the managed block to /etc/hosts
  clean  remove the managed block from /etc/hosts

options:
  -h, --help  show this help`

func Run(args []string, stdout io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: ctx <command>")
	}

	switch args[1] {
	case "init":
		if len(args) == 3 && isHelpArg(args[2]) {
			_, err := fmt.Fprintln(stdout, initUsageText)
			return err
		}
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
		if len(args) == 3 && isHelpArg(args[2]) {
			_, err := fmt.Fprintln(stdout, statusUsageText)
			return err
		}
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
	case "host":
		return runHost(args[2:], stdout)
	case "hosts":
		return runHosts(args[2:], stdout)
	case "-h", "--help":
		_, err := fmt.Fprintln(stdout, usageText)
		return err
	default:
		return fmt.Errorf("unknown ctx command: %s", args[1])
	}
}

func runTarget(args []string, stdout io.Writer) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, targetUsageText)
		return err
	}
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, targetUsageText)
		return err
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
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, ipUsageText)
		return err
	}

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

func runHost(args []string, stdout io.Writer) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, hostUsageText)
		return err
	}
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, hostUsageText)
		return err
	}

	workspace, err := currentWorkspace()
	if err != nil {
		return err
	}

	switch args[0] {
	case "add":
		hostname, targetName, err := parseHostAddArgs(args[1:])
		if err != nil {
			return err
		}
		host, err := AddHost(workspace, hostname, targetName)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "%s %s %s\n", host.Hostname, host.TargetName, host.TargetIP)
		return err
	case "rm":
		if len(args) != 2 {
			return errors.New("usage: ctx host rm <hostname>")
		}
		if err := RemoveHost(workspace, args[1]); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "removed host: %s\n", args[1])
		return err
	case "ls":
		if len(args) != 1 {
			return errors.New("usage: ctx host ls")
		}
		hosts, err := ListHosts(workspace)
		if err != nil {
			return err
		}
		if len(hosts) == 0 {
			_, err = fmt.Fprintln(stdout, "no hosts")
			return err
		}
		for _, host := range hosts {
			if _, err := fmt.Fprintf(stdout, "%s %s %s\n", host.Hostname, host.TargetName, host.TargetIP); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown ctx host command: %s", args[0])
	}
}

func runHosts(args []string, stdout io.Writer) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, hostsUsageText)
		return err
	}
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, hostsUsageText)
		return err
	}

	workspace, err := currentWorkspace()
	if err != nil {
		return err
	}

	switch args[0] {
	case "show":
		if len(args) != 1 {
			return errors.New("usage: ctx hosts show")
		}
		block, err := RenderHostsBlock(workspace)
		if err != nil {
			return err
		}
		_, err = io.WriteString(stdout, block)
		return err
	case "sync":
		internal, err := parseHostsSyncArgs(args[1:])
		if err != nil {
			return err
		}
		if err := syncHostsFileFunc(workspace, hostsFilePath); err != nil {
			if !internal && errors.Is(err, os.ErrPermission) {
				if _, writeErr := fmt.Fprintln(stdout, "Need administrator privileges to update /etc/hosts."); writeErr != nil {
					return writeErr
				}
				if _, writeErr := fmt.Fprintln(stdout, "Re-running hosts sync with sudo..."); writeErr != nil {
					return writeErr
				}
				return reexecHostsSyncWithSudoFunc(stdout)
			}
			return err
		}
		_, err = fmt.Fprintln(stdout, "synced hosts")
		return err
	case "clean":
		internal, err := parseHostsCleanArgs(args[1:])
		if err != nil {
			return err
		}
		if err := cleanHostsFileFunc(workspace, hostsFilePath); err != nil {
			if !internal && errors.Is(err, os.ErrPermission) {
				if _, writeErr := fmt.Fprintln(stdout, "Need administrator privileges to update /etc/hosts."); writeErr != nil {
					return writeErr
				}
				if _, writeErr := fmt.Fprintln(stdout, "Re-running hosts clean with sudo..."); writeErr != nil {
					return writeErr
				}
				return reexecHostsCleanWithSudoFunc(stdout)
			}
			return err
		}
		_, err = fmt.Fprintln(stdout, "cleaned hosts")
		return err
	default:
		return fmt.Errorf("unknown ctx hosts command: %s", args[0])
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

func parseHostAddArgs(args []string) (string, string, error) {
	if len(args) < 1 {
		return "", "", errors.New("usage: ctx host add <hostname> [--target <name>]")
	}

	hostname := args[0]
	var targetName string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 >= len(args) {
				return "", "", errors.New("usage: ctx host add <hostname> [--target <name>]")
			}
			targetName = args[i+1]
			i++
		default:
			return "", "", fmt.Errorf("unknown ctx host add option: %s", args[i])
		}
	}

	return hostname, targetName, nil
}

func parseHostsSyncArgs(args []string) (bool, error) {
	switch len(args) {
	case 0:
		return false, nil
	case 1:
		if args[0] == "--internal" {
			return true, nil
		}
	}
	return false, errors.New("usage: ctx hosts sync [--internal]")
}

func reexecHostsSyncWithSudo(stdout io.Writer) error {
	executable, err := executableFunc()
	if err != nil {
		return fmt.Errorf("failed to locate ctx executable: %w", err)
	}

	args := []string{"env", "CTX_HOME=" + dataRoot()}
	args = append(args, executable, "hosts", "sync", "--internal")

	cmd := execCommandFunc("sudo", args...)
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo hosts sync failed: %w", err)
	}
	return nil
}

func parseHostsCleanArgs(args []string) (bool, error) {
	switch len(args) {
	case 0:
		return false, nil
	case 1:
		if args[0] == "--internal" {
			return true, nil
		}
	}
	return false, errors.New("usage: ctx hosts clean [--internal]")
}

func reexecHostsCleanWithSudo(stdout io.Writer) error {
	executable, err := executableFunc()
	if err != nil {
		return fmt.Errorf("failed to locate ctx executable: %w", err)
	}

	args := []string{"env", "CTX_HOME=" + dataRoot()}
	args = append(args, executable, "hosts", "clean", "--internal")

	cmd := execCommandFunc("sudo", args...)
	if wd, err := os.Getwd(); err == nil {
		cmd.Dir = wd
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo hosts clean failed: %w", err)
	}
	return nil
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help"
}
