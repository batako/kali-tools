package ctx

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
)

var (
	Version = "0.1.0"

	hostsFilePath                = "/etc/hosts"
	syncHostsFileFunc            = SyncHostsFile
	reexecHostsSyncWithSudoFunc  = reexecHostsSyncWithSudo
	cleanHostsFileFunc           = CleanHostsFile
	reexecHostsCleanWithSudoFunc = reexecHostsCleanWithSudo
	resetHostsBlocksFunc         = ResetHostsBlocks
	reexecResetHostsWithSudoFunc = reexecResetHostsWithSudo
	resetCtxDataFunc             = ResetCtxData
	executableFunc               = os.Executable
	lookPathFunc                 = exec.LookPath
	execCommandFunc              = exec.Command
	workspaceStdin               = io.Reader(os.Stdin)
)

const usageText = `usage: ctx <command> [options]

commands:
  status   show the current workspace
  workspace  initialize, list, or remove workspaces
  project    create and manage projects under the configured root
  target   manage targets
  ip       show or update the primary target IP
  host     manage hostnames
  hosts    show, sync, or clean /etc/hosts entries
  scan     run nmap and save structured service results
  service  show saved port scan results
  credential  manage credentials
  note     add a note to the workspace timeline
  log      show the workspace timeline
  prompt   print data for shell prompts
  x        run a command and save execution logs
  completion  print shell completion script
  init-shell  configure shell integration
  doctor   check ctx environment
  reset    remove all ctx data and configuration

options:
  -h, --help     show this help
  -V, --version  show version

shortcuts (requires ctx init-shell):
  xinit        ctx workspace init
  xworkspace   ctx workspace
  xstatus      ctx status
  xproject     ctx project
  xnew         ctx project new
  xtarget      ctx target
  xip          ctx ip
  xhost        ctx host
  xhosts       ctx hosts
  xscan        ctx scan
  xservice     ctx service
  xcredential  ctx credential
  xnote        ctx note
  xlog         ctx log
  xprompt      ctx prompt
  x            ctx x
  xcompletion  ctx completion
  xdoctor      ctx doctor
  xinit-shell  ctx init-shell
  xreset       ctx reset

extra shortcuts (requires ctx init-shell --extra-shortcuts):
  pj           ctx project
  ta           ctx target
  cr           ctx credential

Run ctx <command> -h for command-specific help.`

const statusUsageText = `usage: ctx status [options]

Show the current ctx workspace.

options:
  -h, --help  show this help`

const workspaceUsageText = `usage: ctx workspace <command> [options]

commands:
  init               create a workspace in the current directory
  ls                 list workspaces
  rm [id] [-y|--yes] remove a workspace and all of its ctx data

options:
  -h, --help         show this help
  -y, --yes          skip removal confirmation`

const projectUsageText = `usage: ctx project [<name> | <command>] [options]

Project is an optional convenience layer for ctx-managed directories under a
configured root. Workspaces remain the core feature and can still be initialized
directly in any directory with ctx workspace init.

commands:
  root [path]       show or set the projects root
  new <name>        create a project and initialize a workspace
  ls                list ctx projects under the configured root
  rm <name> [-y|--yes] remove a project directory

options:
  -y, --yes         skip removal confirmation
  -h, --help        show this help

shorthand:
  ctx project <name>           same as 'ctx project new <name>'`

const targetUsageText = `usage: ctx target [<ip> | <command>] [options]

commands:
  set <ip>                 create or update the primary target
  add <ip> [--name <name>] add a target
  update <ip>              update the current primary target IP
  use <name>               make a target primary
  rm <name>                remove a target
  ls                       list targets

options:
  -h, --help               show this help

shorthand:
  ctx target <ip>          same as 'ctx target set <ip>'`

const ipUsageText = `usage: ctx ip [ip] [options]

Show or update the primary target IP.

options:
  -h, --help  show this help`

const hostUsageText = `usage: ctx host [<hostname> | <command>] [options]

commands:
  add <hostname> [--target <name>] add a host
  rm <hostname>                    remove a host
  ls                               list hosts

options:
  -h, --help                       show this help

shorthand:
  ctx host <hostname>              same as 'ctx host add <hostname>'`

const completionUsageText = `usage: ctx completion <zsh|bash> [options]

Print shell completion script to stdout.

options:
  --extra-shortcuts  include pj, ta, and cr shortcuts
  -h, --help         show this help`

const initShellUsageText = `usage: ctx init-shell [--remove|--extra-shortcuts] [options]

Configure ctx shell integration for the current shell.

options:
  --extra-shortcuts  include pj, ta, and cr shortcuts
  --remove           remove ctx shell integration
  -h, --help         show this help`

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

const scanUsageText = `usage: ctx scan [ip] [options]

Run nmap for the current ctx workspace and save structured service results.

options:
  -p, --ports <ports>  pass an explicit port list/range to nmap
  -n, --dry-run        print the nmap command without running it
  -f, --force          run even if the same scan already succeeded
  -h, --help           show this help`

const serviceUsageText = `usage: ctx service ls [--target <name>] [options]

Show saved port scan results for the primary or selected target.

commands:
  ls  list saved ports and services

options:
  --target <name>  select a target by name
  -h, --help       show this help`

const credentialUsageText = `usage: ctx credential [<scope> <username> [password] | <command>] [options]

Manage credentials for the current workspace.

commands:
  ls [scope]                         list credentials
  set <scope> <username> [password]  create or update a credential
  add <scope> <username> [password]  add a credential
  update <scope> <username> [password] update an existing credential
  rm <id> [-y|--yes]                 remove a credential by ID
  rm <scope> <username> [-y|--yes]   remove a credential by scope and username
  rm <username> [-y|--yes]           remove a credential by username

options:
  -y, --yes                          skip removal confirmation
  -h, --help                         show this help

shorthand:
  ctx credential <scope> <username> [password] same as 'ctx credential set <scope> <username> [password]'`

const logUsageText = `usage: ctx log [id] [options]

Show the workspace timeline or command log details by ID.

options:
  -p, --plain        print a compact timeline
  -v, --verbose      print IDs, status, and exit codes
  -i, --interactive  open the interactive timeline
  -h, --help         show this help`

const noteUsageText = `usage: ctx note <text> [options]

Add a note to the current workspace timeline.

options:
  -h, --help  show this help`

const promptUsageText = `usage: ctx prompt [options]

Print workspace, local IP, and target data for shell prompts.

options:
  --format <shell|json>  select output format (default: shell)
  --field <name>         print one field
  -h, --help             show this help

fields:
  active, workspace-id, workspace-name, workspace-path
  local-ip, local-interface, target-name, target-ip`

const resetUsageText = `usage: ctx reset [-y|--yes] [options]

Remove all ctx data and configuration without uninstalling ctx.
Workspace directories and shell history are not removed.

options:
  -y, --yes   skip confirmation
  -h, --help  show this help`

func Run(args []string, stdout io.Writer) error {
	return RunWithIO(args, stdout, stdout)
}

func RunWithIO(args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: ctx <command>")
	}

	switch args[1] {
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
		if _, err = fmt.Fprintf(stdout, "workspace: %s\nname: %s\nroot: %s\ndata: %s\ndatabase: %s\n", record.ID, filepath.Base(record.RootPath), record.RootPath, workspace.DataPath, workspace.DatabasePath); err != nil {
			return err
		}
		return writeExecutableInfo(stdout)
	case "workspace":
		return runWorkspace(args[2:], stdout)
	case "project":
		return runProject(args[2:], stdout)
	case "target":
		return runTarget(args[2:], stdout)
	case "ip":
		return runIP(args[2:], stdout)
	case "host":
		return runHost(args[2:], stdout)
	case "hosts":
		return runHosts(args[2:], stdout)
	case "scan":
		if len(args) == 3 && isHelpArg(args[2]) {
			_, err := fmt.Fprintln(stdout, scanUsageText)
			return err
		}
		code := RunScan(append([]string{"scan"}, args[2:]...), stdout, stderr)
		if code != 0 {
			return ExitCodeError{Code: code}
		}
		return nil
	case "service":
		return runService(args[2:], stdout)
	case "credential":
		return runCredential(args[2:], stdout)
	case "note":
		return runNote(args[2:], stdout)
	case "log":
		return runLog(args[2:], stdout)
	case "prompt":
		return runPrompt(args[2:], stdout)
	case "x":
		if len(args) == 3 && isHelpArg(args[2]) {
			_, err := fmt.Fprintln(stdout, xUsageText)
			return err
		}
		code := RunX(append([]string{"x"}, args[2:]...), stdout, stderr)
		if code != 0 {
			return ExitCodeError{Code: code}
		}
		return nil
	case "completion":
		return runCompletion(args[2:], stdout)
	case "init-shell":
		return runInitShell(args[2:], stdout)
	case "doctor":
		return runDoctor(args[2:], stdout)
	case "reset":
		return runReset(args[2:], stdout)
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

func runService(args []string, stdout io.Writer) error {
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, serviceUsageText)
		return err
	}

	var err error
	var showHelp bool
	args, showHelp, err = resolveResourceCommand("service", args, []string{"ls"}, "", "ls")
	if err != nil {
		return err
	}
	if showHelp {
		_, err := fmt.Fprintln(stdout, serviceUsageText)
		return err
	}

	if args[0] != "ls" {
		return fmt.Errorf("unknown ctx service command: %s", args[0])
	}

	targetName := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--target":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return errors.New("usage: ctx service ls [--target <name>]")
			}
			i++
			targetName = args[i]
		default:
			return errors.New("usage: ctx service ls [--target <name>]")
		}
	}

	workspace, err := currentWorkspace()
	if err != nil {
		return err
	}
	var target *Target
	if targetName == "" {
		target, err = GetPrimaryTarget(workspace)
	} else {
		target, err = GetTargetByName(workspace, targetName)
	}
	if err != nil {
		return err
	}

	services, err := ListServices(workspace, target)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Target: %s (%s)\n", target.Name, target.IP); err != nil {
		return err
	}
	if len(services) == 0 {
		_, err = fmt.Fprintln(stdout, "no scan results")
		return err
	}

	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "PORT\tSTATE\tSERVICE\tPRODUCT\tVERSION"); err != nil {
		return err
	}
	for _, service := range services {
		port := fmt.Sprintf("%d/%s", service.Port, service.Protocol)
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			port, service.State, service.ServiceName, service.Product, service.Version); err != nil {
			return err
		}
	}
	return table.Flush()
}

func runCredential(args []string, stdout io.Writer) error {
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, credentialUsageText)
		return err
	}

	var err error
	var showHelp bool
	args, showHelp, err = resolveResourceCommand("credential", args, []string{"ls", "set", "add", "update", "rm"}, "set", "ls")
	if err != nil {
		return err
	}
	if showHelp {
		_, err := fmt.Fprintln(stdout, credentialUsageText)
		return err
	}

	workspace, err := currentWorkspace()
	if err != nil {
		return err
	}

	switch args[0] {
	case "ls":
		scope, err := parseCredentialListArgs(args[1:])
		if err != nil {
			return err
		}
		credentials, err := ListCredentials(workspace, scope)
		if err != nil {
			return err
		}
		if len(credentials) == 0 {
			_, err = fmt.Fprintln(stdout, "no credentials")
			return err
		}
		return writeCredentialTable(stdout, credentials)
	case "set":
		scope, username, password, err := parseCredentialSaveArgs(args[1:], "set")
		if err != nil {
			return err
		}
		credential, err := SetCredential(workspace, scope, username, password)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "credential: [%d] %s %s %s\n", credential.ID, credential.Scope, credential.Username, credential.Password)
		return err
	case "add":
		scope, username, password, err := parseCredentialSaveArgs(args[1:], "add")
		if err != nil {
			return err
		}
		credential, err := AddCredential(workspace, scope, username, password)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "credential: [%d] %s %s %s\n", credential.ID, credential.Scope, credential.Username, credential.Password)
		return err
	case "update":
		scope, username, password, err := parseCredentialSaveArgs(args[1:], "update")
		if err != nil {
			return err
		}
		credential, err := UpdateCredential(workspace, scope, username, password)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "credential: [%d] %s %s %s\n", credential.ID, credential.Scope, credential.Username, credential.Password)
		return err
	case "rm":
		selector, yes, err := parseCredentialRemoveArgs(args[1:])
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(workspaceStdin)
		credential, err := selectCredentialForRemoval(workspace, selector, scanner, stdout)
		if err != nil {
			return err
		}
		if !yes {
			ok, err := confirmCredentialRemoval(stdout, scanner, credential)
			if err != nil || !ok {
				return err
			}
		}
		if err := RemoveCredential(workspace, credential.ID); err != nil {
			return err
		}
		return writeRemovedCredential(stdout, credential)
	default:
		return fmt.Errorf("unknown ctx credential command: %s", args[0])
	}
}

func runReset(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, resetUsageText)
		return err
	}
	if len(args) >= 1 && args[0] == "--internal-hosts" {
		if len(args) == 1 {
			return errors.New("reset hosts cleanup requires at least one workspace id")
		}
		return resetHostsBlocksFunc(hostsFilePath, args[1:])
	}

	yes := false
	switch len(args) {
	case 0:
	case 1:
		if args[0] != "--yes" && args[0] != "-y" {
			return errors.New("usage: ctx reset [-y|--yes]")
		}
		yes = true
	default:
		return errors.New("usage: ctx reset [-y|--yes]")
	}

	if !yes {
		if _, err := fmt.Fprint(stdout, "Remove all ctx data and configuration? [y/N] "); err != nil {
			return err
		}
		scanner := bufio.NewScanner(workspaceStdin)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("failed to read reset confirmation: %w", err)
			}
			_, err := fmt.Fprintln(stdout, "\ncancelled")
			return err
		}
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if answer != "y" && answer != "yes" {
			_, err := fmt.Fprintln(stdout, "cancelled")
			return err
		}
	}

	records, err := ListWorkspaceRecords()
	if err != nil {
		return err
	}
	workspaceIDs, err := CtxHostsWorkspaceIDs(hostsFilePath)
	if err != nil {
		return err
	}
	workspaceIDs = mergeWorkspaceIDs(workspaceIDs, records)

	if err := resetHostsBlocksFunc(hostsFilePath, workspaceIDs); err != nil {
		if errors.Is(err, os.ErrPermission) {
			if _, writeErr := fmt.Fprintln(stdout, "Need administrator privileges to remove ctx entries from /etc/hosts."); writeErr != nil {
				return writeErr
			}
			if _, writeErr := fmt.Fprintln(stdout, "Re-running ctx hosts cleanup with sudo..."); writeErr != nil {
				return writeErr
			}
			if err := reexecResetHostsWithSudoFunc(workspaceIDs, stdout); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if err := resetCtxDataFunc(records); err != nil {
		return err
	}
	if _, err = fmt.Fprintln(stdout, "ctx data and configuration removed"); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "Restart the current shell to unload ctx helper functions.")
	return err
}

func mergeWorkspaceIDs(ids []string, records []WorkspaceRecord) []string {
	seen := make(map[string]struct{}, len(ids)+len(records))
	merged := make([]string, 0, len(ids)+len(records))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		merged = append(merged, id)
	}
	for _, record := range records {
		if _, exists := seen[record.ID]; exists {
			continue
		}
		seen[record.ID] = struct{}{}
		merged = append(merged, record.ID)
	}
	return merged
}

func runPrompt(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, promptUsageText)
		return err
	}

	format, field, err := parsePromptArgs(args)
	if err != nil {
		return err
	}
	data, err := currentPromptData()
	if err != nil {
		return err
	}
	return WritePromptData(stdout, data, format, field)
}

func parsePromptArgs(args []string) (string, string, error) {
	format := "shell"
	var field string
	var formatSet bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if formatSet || i+1 >= len(args) {
				return "", "", errors.New("usage: ctx prompt [--format <shell|json>] [--field <name>]")
			}
			i++
			format = args[i]
			formatSet = true
		case "--field":
			if field != "" || i+1 >= len(args) {
				return "", "", errors.New("usage: ctx prompt [--format <shell|json>] [--field <name>]")
			}
			i++
			field = args[i]
		default:
			return "", "", fmt.Errorf("unknown ctx prompt option: %s", args[i])
		}
	}
	if field != "" && formatSet {
		return "", "", errors.New("--field and --format cannot be used together")
	}
	if format != "shell" && format != "json" {
		return "", "", fmt.Errorf("unsupported prompt format: %s", format)
	}
	return format, field, nil
}

func runNote(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, noteUsageText)
		return err
	}
	if len(args) == 0 {
		return errors.New("usage: ctx note <text>")
	}

	body := strings.TrimSpace(strings.Join(args, " "))
	if body == "" {
		return errors.New("note must not be empty")
	}
	workspace, err := currentWorkspace()
	if err != nil {
		return err
	}
	note, err := SaveNote(workspace, body)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "saved note: note:%d\n", note.ID)
	return err
}

func runWorkspace(args []string, stdout io.Writer) error {
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, workspaceUsageText)
		return err
	}

	var err error
	var showHelp bool
	args, showHelp, err = resolveResourceCommand("workspace", args, []string{"init", "ls", "rm"}, "", "init")
	if err != nil {
		return err
	}
	if showHelp {
		_, err := fmt.Fprintln(stdout, workspaceUsageText)
		return err
	}

	switch args[0] {
	case "init":
		if len(args) != 1 {
			return errors.New("usage: ctx workspace init")
		}
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		workspace, status, err := InitWorkspaceWithStatus(wd)
		if err != nil {
			return err
		}
		switch status {
		case WorkspaceUpdated:
			_, err = fmt.Fprintf(stdout, "updated ctx workspace %s\n", workspace.ID)
			return err
		case WorkspaceUnchanged:
			_, err = fmt.Fprintf(stdout, "ctx workspace already initialized %s\n", workspace.ID)
			return err
		}
		_, err = fmt.Fprintf(stdout, "initialized ctx workspace %s\n", workspace.ID)
		return err
	case "ls":
		if len(args) != 1 {
			return errors.New("usage: ctx workspace ls")
		}
		records, err := ListWorkspaceRecords()
		if err != nil {
			return err
		}
		return printWorkspaceRecords(stdout, records, false)
	case "rm":
		id, yes, err := parseWorkspaceRemoveArgs(args[1:])
		if err != nil {
			return err
		}
		records, err := ListWorkspaceRecords()
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return errors.New("no workspaces")
		}

		scanner := bufio.NewScanner(workspaceStdin)
		record, err := selectWorkspaceForRemoval(id, records, scanner, stdout)
		if err != nil {
			return err
		}
		if !yes {
			fmt.Fprintf(stdout, "Remove workspace %s (%s) and all ctx data? [y/N] ", record.ID, record.RootPath)
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("failed to read confirmation: %w", err)
				}
				_, err := fmt.Fprintln(stdout, "\ncancelled")
				return err
			}
			answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if answer != "y" && answer != "yes" {
				_, err := fmt.Fprintln(stdout, "cancelled")
				return err
			}
		}
		if err := RemoveWorkspace(record); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "removed workspace: %s %s\n", record.ID, record.RootPath)
		return err
	default:
		return fmt.Errorf("unknown ctx workspace command: %s", args[0])
	}
}

func runProject(args []string, stdout io.Writer) error {
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, projectUsageText)
		return err
	}

	var err error
	var showHelp bool
	args, showHelp, err = resolveResourceCommand("project", args, []string{"root", "new", "ls", "rm"}, "new", "ls")
	if err != nil {
		return err
	}
	if showHelp {
		_, err := fmt.Fprintln(stdout, projectUsageText)
		return err
	}

	switch args[0] {
	case "root":
		switch len(args) {
		case 1:
			root, err := GetProjectRoot()
			if err != nil {
				return err
			}
			if root == "" {
				_, err = fmt.Fprintln(stdout, projectRootUnsetMessage)
				return err
			}
			_, err = fmt.Fprintln(stdout, root)
			return err
		case 2:
			root, err := SetProjectRoot(args[1])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(stdout, root)
			return err
		default:
			return errors.New("usage: ctx project root [path]")
		}
	case "new":
		if len(args) != 2 {
			return errors.New("usage: ctx project new <name>")
		}
		path, err := CreateProject(args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, path)
		return err
	case "ls":
		if len(args) != 1 {
			return errors.New("usage: ctx project ls")
		}
		projects, err := ListProjects()
		if err != nil {
			if errors.Is(err, ErrProjectRootUnset) {
				_, writeErr := fmt.Fprintln(stdout, projectRootUnsetMessage)
				return writeErr
			}
			return err
		}
		if len(projects) == 0 {
			_, err = fmt.Fprintln(stdout, "no projects")
			return err
		}
		for _, project := range projects {
			if _, err := fmt.Fprintln(stdout, project.Path); err != nil {
				return err
			}
		}
		return nil
	case "rm":
		name, yes, err := parseProjectRemoveArgs(args[1:])
		if err != nil {
			return err
		}
		root, err := requiredProjectRoot()
		if err != nil {
			return err
		}
		path, err := projectPath(root, name)
		if err != nil {
			return err
		}
		if !yes {
			ok, err := confirmProjectRemoval(stdout, bufio.NewScanner(workspaceStdin), name, path)
			if err != nil || !ok {
				return err
			}
		}
		removedPath, err := RemoveProject(name)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "removed project: %s\n", removedPath)
		return err
	default:
		return fmt.Errorf("unknown ctx project command: %s", args[0])
	}
}

func parseProjectRemoveArgs(args []string) (string, bool, error) {
	var name string
	var yes bool
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			if yes {
				return "", false, errors.New("usage: ctx project rm <name> [-y|--yes]")
			}
			yes = true
		default:
			if strings.HasPrefix(arg, "-") || name != "" {
				return "", false, errors.New("usage: ctx project rm <name> [-y|--yes]")
			}
			name = arg
		}
	}
	if name == "" {
		return "", false, errors.New("usage: ctx project rm <name> [-y|--yes]")
	}
	return name, yes, nil
}

func parseWorkspaceRemoveArgs(args []string) (string, bool, error) {
	var id string
	var yes bool
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			if yes {
				return "", false, errors.New("usage: ctx workspace rm [id] [-y|--yes]")
			}
			yes = true
		default:
			if strings.HasPrefix(arg, "-") || id != "" {
				return "", false, errors.New("usage: ctx workspace rm [id] [-y|--yes]")
			}
			id = arg
		}
	}
	return id, yes, nil
}

func selectWorkspaceForRemoval(id string, records []WorkspaceRecord, scanner *bufio.Scanner, stdout io.Writer) (WorkspaceRecord, error) {
	if id != "" {
		for _, record := range records {
			if record.ID == id {
				return record, nil
			}
		}
		return WorkspaceRecord{}, fmt.Errorf("workspace not found: %s", id)
	}

	wd, err := os.Getwd()
	if err != nil {
		return WorkspaceRecord{}, fmt.Errorf("failed to get current directory: %w", err)
	}
	current, err := FindWorkspace(wd)
	if err == nil {
		for _, record := range records {
			if record.ID == current.ID {
				return record, nil
			}
		}
		return WorkspaceRecord{}, fmt.Errorf("workspace not found in database: %s", current.ID)
	}
	if !errors.Is(err, ErrWorkspaceNotFound) {
		return WorkspaceRecord{}, err
	}

	if err := printWorkspaceRecords(stdout, records, true); err != nil {
		return WorkspaceRecord{}, err
	}
	fmt.Fprintf(stdout, "Select workspace to remove [1-%d]: ", len(records))
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return WorkspaceRecord{}, fmt.Errorf("failed to read workspace selection: %w", err)
		}
		return WorkspaceRecord{}, errors.New("workspace selection required")
	}
	selection, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || selection < 1 || selection > len(records) {
		return WorkspaceRecord{}, fmt.Errorf("invalid workspace selection: %s", scanner.Text())
	}
	return records[selection-1], nil
}

func printWorkspaceRecords(stdout io.Writer, records []WorkspaceRecord, numbered bool) error {
	if len(records) == 0 {
		_, err := fmt.Fprintln(stdout, "no workspaces")
		return err
	}
	for i, record := range records {
		if numbered {
			if _, err := fmt.Fprintf(stdout, "%d  %s  %s\n", i+1, record.ID, record.RootPath); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(stdout, "%s  %s\n", record.ID, record.RootPath); err != nil {
			return err
		}
	}
	return nil
}

func runLog(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, logUsageText)
		return err
	}
	id, mode, err := parseLogArgs(args)
	if err != nil {
		return err
	}

	workspace, err := currentWorkspace()
	if err != nil {
		return err
	}

	if id == "" {
		entries, err := ListTimeline(workspace)
		if err != nil {
			return err
		}
		if len(entries) == 0 && mode != logDisplayInteractive && !(mode == logDisplayAuto && logIsTerminal(stdout)) {
			_, err = fmt.Fprintln(stdout, "no logs")
			return err
		}
		if mode == logDisplayInteractive || (mode == logDisplayAuto && logIsTerminal(stdout)) {
			return runLogTUI(workspace, entries, stdout)
		}
		if mode == logDisplayVerbose {
			return writeVerboseTimeline(stdout, entries)
		}
		return writePlainTimeline(stdout, entries)
	}

	log, err := GetCommandLog(workspace, id)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "id: %d\ncommand: %s\nexpanded_command: %s\nstatus: %s\nexit_code: %d\nstarted_at: %s\nended_at: %s\n\n", log.ID, log.Command, log.ExpandedCommand, log.Status, log.ExitCode, log.StartedAt, log.EndedAt); err != nil {
		return err
	}
	_, err = io.WriteString(stdout, commandOutputSections(log.Stdout, log.Stderr))
	return err
}

func parseLogArgs(args []string) (string, logDisplayMode, error) {
	var id string
	mode := logDisplayAuto
	for _, arg := range args {
		var requested logDisplayMode
		switch arg {
		case "-p", "--plain":
			requested = logDisplayPlain
		case "-v", "--verbose":
			requested = logDisplayVerbose
		case "-i", "--interactive":
			requested = logDisplayInteractive
		default:
			if strings.HasPrefix(arg, "-") || id != "" {
				return "", logDisplayAuto, errors.New("usage: ctx log [id] [--plain|--verbose|--interactive]")
			}
			id = arg
			continue
		}
		if mode != logDisplayAuto {
			return "", logDisplayAuto, errors.New("usage: ctx log [id] [--plain|--verbose|--interactive]")
		}
		mode = requested
	}
	if id != "" && mode != logDisplayAuto {
		return "", logDisplayAuto, errors.New("display options cannot be used with a log id")
	}
	return id, mode, nil
}

func resolveResourceCommand(resource string, args []string, subcommands []string, defaultAction, defaultView string) ([]string, bool, error) {
	if len(args) == 0 {
		if defaultView == "" || defaultView == "help" {
			return nil, true, nil
		}
		resolved, err := resolveConfiguredResourceAction(resource, subcommands, defaultView)
		return resolved, false, err
	}
	if isHelpArg(args[0]) {
		return nil, true, nil
	}

	resolved, err := resolveResourceAction(resource, args, subcommands, defaultAction)
	return resolved, false, err
}

func resolveResourceAction(resource string, args []string, subcommands []string, defaultAction string) ([]string, error) {
	known := make(map[string]struct{}, len(subcommands))
	for _, subcommand := range subcommands {
		known[subcommand] = struct{}{}
	}
	if _, ok := known[args[0]]; ok {
		return args, nil
	}
	if defaultAction == "" {
		return nil, fmt.Errorf("unknown ctx %s command: %s", resource, args[0])
	}
	if isDestructiveDefaultAction(defaultAction) {
		return nil, fmt.Errorf("invalid default action for ctx %s: %s", resource, defaultAction)
	}
	if _, ok := known[defaultAction]; !ok {
		return nil, fmt.Errorf("invalid default action for ctx %s: %s", resource, defaultAction)
	}

	resolved := make([]string, 0, len(args)+1)
	resolved = append(resolved, defaultAction)
	resolved = append(resolved, args...)
	return resolved, nil
}

func resolveConfiguredResourceAction(resource string, subcommands []string, action string) ([]string, error) {
	if isDestructiveDefaultAction(action) {
		return nil, fmt.Errorf("invalid default view for ctx %s: %s", resource, action)
	}
	for _, subcommand := range subcommands {
		if action == subcommand {
			return []string{action}, nil
		}
	}
	return nil, fmt.Errorf("invalid default view for ctx %s: %s", resource, action)
}

func isDestructiveDefaultAction(action string) bool {
	switch action {
	case "rm", "remove", "delete", "reset", "clean", "overwrite", "purge", "drop":
		return true
	default:
		return false
	}
}

func runTarget(args []string, stdout io.Writer) error {
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, targetUsageText)
		return err
	}

	args, showHelp, err := resolveResourceCommand("target", args, []string{"set", "add", "update", "use", "rm", "ls"}, "set", "ls")
	if err != nil {
		return err
	}
	if showHelp {
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
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, hostUsageText)
		return err
	}

	args, showHelp, err := resolveResourceCommand("host", args, []string{"add", "rm", "ls"}, "add", "ls")
	if err != nil {
		return err
	}
	if showHelp {
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
	if len(args) > 1 && isHelpArg(args[1]) {
		_, err := fmt.Fprintln(stdout, hostsUsageText)
		return err
	}

	var err error
	var showHelp bool
	args, showHelp, err = resolveResourceCommand("hosts", args, []string{"show", "sync", "clean"}, "", "show")
	if err != nil {
		return err
	}
	if showHelp {
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
	if len(args) == 2 && args[0] == "values" {
		values, err := completionValues(args[1])
		if err != nil {
			return err
		}
		for _, value := range values {
			if _, err := fmt.Fprintln(stdout, value); err != nil {
				return err
			}
		}
		return nil
	}
	if len(args) == 2 && args[0] == "descriptions" {
		values, err := completionDescriptions(args[1])
		if err != nil {
			return err
		}
		for _, value := range values {
			if _, err := fmt.Fprintln(stdout, value); err != nil {
				return err
			}
		}
		return nil
	}
	includeExtraShortcuts := false
	switch len(args) {
	case 1:
	case 2:
		if args[1] != "--extra-shortcuts" {
			return errors.New("usage: ctx completion <zsh|bash> [--extra-shortcuts]")
		}
		includeExtraShortcuts = true
	default:
		return errors.New("usage: ctx completion <zsh|bash> [--extra-shortcuts]")
	}
	script, err := CompletionScript(args[0], CompletionOptions{ExtraShortcuts: includeExtraShortcuts})
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, script)
	return err
}

func completionValues(kind string) ([]string, error) {
	if kind == "workspace" {
		records, err := ListWorkspaceRecords()
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(records))
		for _, record := range records {
			values = append(values, record.ID)
		}
		return values, nil
	}
	if kind == "project" {
		projects, err := ListProjects()
		if err != nil {
			if errors.Is(err, ErrProjectRootUnset) {
				return nil, nil
			}
			return nil, err
		}
		values := make([]string, 0, len(projects))
		for _, project := range projects {
			values = append(values, project.Name)
		}
		return values, nil
	}

	workspace, err := currentWorkspace()
	if errors.Is(err, ErrWorkspaceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	switch kind {
	case "target":
		targets, err := ListTargets(workspace)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(targets))
		for _, target := range targets {
			values = append(values, target.Name)
		}
		return values, nil
	case "host":
		hosts, err := ListHosts(workspace)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(hosts))
		for _, host := range hosts {
			values = append(values, host.Hostname)
		}
		return values, nil
	case "log":
		logs, err := ListCommandLogs(workspace)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(logs))
		for _, log := range logs {
			values = append(values, strconv.FormatInt(log.ID, 10))
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unknown completion value kind: %s", kind)
	}
}

func completionDescriptions(kind string) ([]string, error) {
	if kind == "workspace" {
		records, err := ListWorkspaceRecords()
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(records))
		for _, record := range records {
			values = append(values, zshCompletionSpec(record.ID, record.RootPath))
		}
		return values, nil
	}
	if kind == "project" {
		projects, err := ListProjects()
		if err != nil {
			if errors.Is(err, ErrProjectRootUnset) {
				return nil, nil
			}
			return nil, err
		}
		values := make([]string, 0, len(projects))
		for _, project := range projects {
			values = append(values, zshCompletionSpec(project.Name, project.Path))
		}
		return values, nil
	}

	workspace, err := currentWorkspace()
	if errors.Is(err, ErrWorkspaceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	switch kind {
	case "target":
		targets, err := ListTargets(workspace)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(targets))
		for _, target := range targets {
			description := target.IP
			if target.IsPrimary {
				description += " (primary)"
			}
			values = append(values, zshCompletionSpec(target.Name, description))
		}
		return values, nil
	case "host":
		hosts, err := ListHosts(workspace)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(hosts))
		for _, host := range hosts {
			values = append(values, zshCompletionSpec(host.Hostname, strings.TrimSpace(host.TargetName+"  "+host.TargetIP)))
		}
		return values, nil
	case "log":
		logs, err := ListCommandLogs(workspace)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(logs))
		for _, log := range logs {
			values = append(values, zshCompletionSpec(strconv.FormatInt(log.ID, 10), oneLine(log.Command)))
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unknown completion description kind: %s", kind)
	}
}

func zshCompletionSpec(value, description string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ":", `\:`)
	description = oneLine(description)
	return value + ":" + description
}

func runInitShell(args []string, stdout io.Writer) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := fmt.Fprintln(stdout, initShellUsageText)
		return err
	}
	remove := false
	extraShortcuts := false
	for _, arg := range args {
		switch arg {
		case "--remove":
			remove = true
		case "--extra-shortcuts":
			extraShortcuts = true
		default:
			return errors.New("usage: ctx init-shell [--remove|--extra-shortcuts]")
		}
	}
	if remove && extraShortcuts {
		return errors.New("usage: ctx init-shell [--remove|--extra-shortcuts]")
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

	config, changed, err := InstallShellConfig(ShellIntegrationOptions{ExtraShortcuts: extraShortcuts})
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
	_, err := writeDoctorReport(stdout, collectDoctorChecks())
	if err != nil {
		return err
	}
	return nil
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

func parseCredentialListArgs(args []string) (string, error) {
	switch len(args) {
	case 0:
		return "", nil
	case 1:
		if strings.HasPrefix(args[0], "-") {
			return "", errors.New("usage: ctx credential ls [scope]")
		}
		return args[0], nil
	default:
		return "", errors.New("usage: ctx credential ls [scope]")
	}
}

func parseCredentialSaveArgs(args []string, action string) (string, string, string, error) {
	if len(args) < 2 || len(args) > 3 {
		return "", "", "", fmt.Errorf("usage: ctx credential %s <scope> <username> [password]", action)
	}
	if strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return "", "", "", fmt.Errorf("usage: ctx credential %s <scope> <username> [password]", action)
	}
	password := ""
	if len(args) == 3 {
		password = args[2]
	}
	return args[0], args[1], password, nil
}

type credentialRemoveSelector struct {
	HasID    bool
	ID       int64
	Scope    string
	Username string
}

func parseCredentialRemoveArgs(args []string) (credentialRemoveSelector, bool, error) {
	var values []string
	var yes bool
	for _, arg := range args {
		switch arg {
		case "-y", "--yes":
			if yes {
				return credentialRemoveSelector{}, false, errors.New("usage: ctx credential rm <id|username|scope username> [-y|--yes]")
			}
			yes = true
		default:
			if strings.HasPrefix(arg, "-") {
				return credentialRemoveSelector{}, false, errors.New("usage: ctx credential rm <id|username|scope username> [-y|--yes]")
			}
			values = append(values, arg)
		}
	}
	switch len(values) {
	case 1:
		if id, err := strconv.ParseInt(values[0], 10, 64); err == nil {
			return credentialRemoveSelector{HasID: true, ID: id}, yes, nil
		}
		return credentialRemoveSelector{Username: values[0]}, yes, nil
	case 2:
		return credentialRemoveSelector{Scope: values[0], Username: values[1]}, yes, nil
	default:
		return credentialRemoveSelector{}, false, errors.New("usage: ctx credential rm <id|username|scope username> [-y|--yes]")
	}
}

func selectCredentialForRemoval(workspace *Workspace, selector credentialRemoveSelector, scanner *bufio.Scanner, stdout io.Writer) (*Credential, error) {
	if selector.HasID {
		return GetCredentialByID(workspace, selector.ID)
	}

	var credentials []Credential
	var err error
	if selector.Scope != "" {
		credentials, err = FindCredentialsByScopeUsername(workspace, selector.Scope, selector.Username)
	} else {
		credentials, err = FindCredentialsByUsername(workspace, selector.Username)
	}
	if err != nil {
		return nil, err
	}
	switch len(credentials) {
	case 0:
		if selector.Scope != "" {
			return nil, fmt.Errorf("credential not found: %s %s", selector.Scope, selector.Username)
		}
		return nil, fmt.Errorf("credential not found: %s", selector.Username)
	case 1:
		return &credentials[0], nil
	}

	if err := writeCredentialCandidates(stdout, credentials); err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(stdout, "Select credential to remove [1-%d]: ", len(credentials)); err != nil {
		return nil, err
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read credential selection: %w", err)
		}
		return nil, errors.New("credential selection required")
	}
	selectionText := strings.TrimSpace(scanner.Text())
	selection, err := strconv.Atoi(selectionText)
	if err != nil || selection < 1 || selection > len(credentials) {
		return nil, fmt.Errorf("invalid credential selection: %s", selectionText)
	}
	return &credentials[selection-1], nil
}

func confirmCredentialRemoval(stdout io.Writer, scanner *bufio.Scanner, credential *Credential) (bool, error) {
	if _, err := fmt.Fprintln(stdout, "Remove credential?"); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return false, err
	}
	if err := writeCredentialDetails(stdout, credential, true); err != nil {
		return false, err
	}
	if _, err := fmt.Fprint(stdout, "\n[y/N]: "); err != nil {
		return false, err
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("failed to read credential removal confirmation: %w", err)
		}
		_, err := fmt.Fprintln(stdout, "\ncancelled")
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	if answer != "y" && answer != "yes" {
		_, err := fmt.Fprintln(stdout, "cancelled")
		return false, err
	}
	return true, nil
}

func writeCredentialTable(stdout io.Writer, credentials []Credential) error {
	table := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "ID\tScope\tUsername\tPassword"); err != nil {
		return err
	}
	for _, credential := range credentials {
		if _, err := fmt.Fprintf(table, "%d\t%s\t%s\t%s\n", credential.ID, credential.Scope, credential.Username, credential.Password); err != nil {
			return err
		}
	}
	return table.Flush()
}

func writeCredentialCandidates(stdout io.Writer, credentials []Credential) error {
	for i, credential := range credentials {
		if _, err := fmt.Fprintf(stdout, "%d) [%d] %s\t%s\t%s\n", i+1, credential.ID, credential.Scope, credential.Username, credential.Password); err != nil {
			return err
		}
	}
	return nil
}

func writeCredentialDetails(stdout io.Writer, credential *Credential, includeID bool) error {
	if includeID {
		if _, err := fmt.Fprintf(stdout, "  ID:       %d\n", credential.ID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "  Scope:    %s\n", credential.Scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "  Username: %s\n", credential.Username); err != nil {
		return err
	}
	_, err := fmt.Fprintf(stdout, "  Password: %s\n", credential.Password)
	return err
}

func writeRemovedCredential(stdout io.Writer, credential *Credential) error {
	if _, err := fmt.Fprintln(stdout, "Removed credential:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	return writeCredentialDetails(stdout, credential, false)
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

func reexecResetHostsWithSudo(workspaceIDs []string, stdout io.Writer) error {
	if len(workspaceIDs) == 0 {
		return nil
	}
	executable, err := executableFunc()
	if err != nil {
		return fmt.Errorf("failed to locate ctx executable: %w", err)
	}

	args := []string{executable, "reset", "--internal-hosts"}
	args = append(args, workspaceIDs...)
	cmd := execCommandFunc("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sudo ctx hosts cleanup failed: %w", err)
	}
	return nil
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help"
}
