package ctx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

var (
	Version = "0.1.0"

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
  completion  print shell completion script
  init-shell  configure shell integration
  doctor   check ctx environment

options:
  -h, --help     show this help
  -V, --version  show version

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

const completionUsageText = `usage: ctx completion <zsh|bash> [options]

Print shell completion script to stdout.

options:
  -h, --help  show this help`

const initShellUsageText = `usage: ctx init-shell [--remove] [options]

Configure ctx shell integration for the current shell.

options:
  --remove    remove ctx shell integration
  -h, --help  show this help`

const doctorUsageText = `usage: ctx doctor [options]

Check ctx environment.

options:
  -h, --help  show this help`

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
		if _, err = fmt.Fprintf(stdout, "workspace: %s\nname: %s\nroot: %s\ndata: %s\ndatabase: %s\n", record.ID, record.Name, record.RootPath, workspace.DataPath, workspace.DatabasePath); err != nil {
			return err
		}
		return writeExecutableInfo(stdout)
	case "target":
		return runTarget(args[2:], stdout)
	case "ip":
		return runIP(args[2:], stdout)
	case "host":
		return runHost(args[2:], stdout)
	case "hosts":
		return runHosts(args[2:], stdout)
	case "completion":
		return runCompletion(args[2:], stdout)
	case "init-shell":
		return runInitShell(args[2:], stdout)
	case "doctor":
		return runDoctor(args[2:], stdout)
	case "-h", "--help":
		_, err := fmt.Fprintln(stdout, usageText)
		return err
	case "-V", "--version":
		_, err := fmt.Fprintf(stdout, "ctx %s\n", Version)
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

func runCompletion(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, completionUsageText)
		return err
	}
	if len(args) != 1 {
		return errors.New("usage: ctx completion <zsh|bash>")
	}
	script, err := CompletionScript(args[0])
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, script)
	return err
}

func runInitShell(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, initShellUsageText)
		return err
	}
	remove := false
	switch len(args) {
	case 0:
	case 1:
		if args[0] != "--remove" {
			return errors.New("usage: ctx init-shell [--remove]")
		}
		remove = true
	default:
		return errors.New("usage: ctx init-shell [--remove]")
	}

	if remove {
		config, changed, err := RemoveShellConfig()
		if err != nil {
			return err
		}
		if changed {
			_, err = fmt.Fprintf(stdout, "removed ctx shell integration from %s\n", config.Path)
			return err
		}
		_, err = fmt.Fprintf(stdout, "ctx shell integration not found in %s\n", config.Path)
		return err
	}

	config, changed, err := InstallShellConfig()
	if err != nil {
		return err
	}
	if changed {
		_, err = fmt.Fprintf(stdout, "configured ctx shell integration for %s in %s\n", config.Shell, config.Path)
		return err
	}
	_, err = fmt.Fprintf(stdout, "ctx shell integration already configured in %s\n", config.Path)
	return err
}

func runDoctor(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, doctorUsageText)
		return err
	}
	if len(args) != 0 {
		return errors.New("usage: ctx doctor")
	}

	if _, err := fmt.Fprintf(stdout, "ctx: %s\n", Version); err != nil {
		return err
	}
	if err := writeExecutableInfo(stdout); err != nil {
		return err
	}

	config, shellErr := DetectShell()
	if shellErr != nil {
		if _, err := fmt.Fprintf(stdout, "shell: error (%v)\n", shellErr); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, "fix: set SHELL to zsh or bash, then run ctx init-shell"); err != nil {
			return err
		}
	} else {
		configured, err := CompletionConfigured(config)
		if err != nil {
			return fmt.Errorf("failed to inspect %s: %w", config.Path, err)
		}
		if _, err := fmt.Fprintf(stdout, "shell: %s\nconfig: %s\ncompletion: %t\n", config.Shell, config.Path, configured); err != nil {
			return err
		}
		if !configured {
			if _, err := fmt.Fprintln(stdout, "fix: run ctx init-shell"); err != nil {
				return err
			}
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	workspace, err := FindWorkspace(wd)
	if err != nil {
		if _, writeErr := fmt.Fprintln(stdout, "workspace: not found"); writeErr != nil {
			return writeErr
		}
		_, writeErr := fmt.Fprintln(stdout, "fix: run ctx init in a workspace directory")
		return writeErr
	}
	_, err = fmt.Fprintf(stdout, "workspace: %s\nworkspace_root: %s\n", workspace.ID, workspace.RootPath)
	return err
}

func writeExecutableInfo(stdout io.Writer) error {
	executable, err := executableFunc()
	if err != nil {
		if _, writeErr := fmt.Fprintf(stdout, "executable: error (%v)\n", err); writeErr != nil {
			return writeErr
		}
	} else {
		if _, writeErr := fmt.Fprintf(stdout, "executable: %s\n", executable); writeErr != nil {
			return writeErr
		}
	}
	path, err := exec.LookPath("ctx")
	if err != nil {
		if _, writeErr := fmt.Fprintln(stdout, "path: ctx not found"); writeErr != nil {
			return writeErr
		}
		_, writeErr := fmt.Fprintln(stdout, "fix: add ctx to PATH")
		return writeErr
	}
	_, err = fmt.Fprintf(stdout, "path: %s\n", path)
	return err
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
