package ctx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const xUsageText = `usage: ctx x <command> [args...]

Run a command in the current ctx workspace and save stdout/stderr to ctx logs.`

type ExitCodeError struct {
	Code int
}

func (e ExitCodeError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

var xStdin io.Reader = os.Stdin

func RunX(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, xUsageText)
		return 1
	}

	workspace, err := currentWorkspace()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	originalArgs := args[1:]
	expandedArgs, err := expandCommandArgs(workspace, originalArgs)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	startedAt := time.Now().UTC()
	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	logID, saveErr := StartCommandLog(workspace, CommandLog{
		Command:         commandString(originalArgs),
		ExpandedCommand: commandString(expandedArgs),
		StartedAt:       startedAt.Format(time.RFC3339Nano),
	})
	if saveErr != nil {
		fmt.Fprintln(stderr, saveErr)
		return 1
	}

	exitCode := 0
	cmd := exec.Command(expandedArgs[0], expandedArgs[1:]...)
	cmd.Stdin = xStdin
	cmd.Stdout = io.MultiWriter(stdout, &stdoutBuffer)
	cmd.Stderr = io.MultiWriter(stderr, &stderrBuffer)

	err = cmd.Run()
	if err != nil {
		exitCode = commandExitCode(err)
		if exitCode == 127 {
			message := fmt.Sprintf("x: %v\n", err)
			_, _ = io.WriteString(stderr, message)
			_, _ = stderrBuffer.WriteString(message)
		}
	}
	endedAt := time.Now().UTC()

	status, storedExitCode := commandLogStatus(exitCode)
	if err := FinishCommandLog(workspace, logID, CommandLog{
		Status:   status,
		ExitCode: storedExitCode,
		Stdout:   stdoutBuffer.String(),
		Stderr:   stderrBuffer.String(),
		EndedAt:  endedAt.Format(time.RFC3339Nano),
	}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return exitCode
}

func expandCommandArgs(workspace *Workspace, args []string) ([]string, error) {
	needsIP := false
	for _, arg := range args {
		if strings.Contains(arg, "$IP") || strings.Contains(arg, "${IP}") {
			needsIP = true
			break
		}
	}
	if !needsIP {
		return append([]string(nil), args...), nil
	}

	target, err := GetPrimaryTarget(workspace)
	if err != nil {
		return nil, err
	}

	expanded := make([]string, len(args))
	for i, arg := range args {
		arg = strings.ReplaceAll(arg, "${IP}", target.IP)
		arg = strings.ReplaceAll(arg, "$IP", target.IP)
		expanded[i] = arg
	}
	return expanded, nil
}

func commandExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 127
}

func commandString(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = quoteCommandArg(arg)
	}
	return strings.Join(quoted, " ")
}

func quoteCommandArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if strings.ContainsAny(arg, " \t\n'\"\\$`!*?[]{}()<>|&;") {
		return strconv.Quote(arg)
	}
	return arg
}
