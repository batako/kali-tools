package ctx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mattn/go-isatty"
)

const (
	doctorGreen  = "\x1b[32m"
	doctorRed    = "\x1b[31m"
	doctorYellow = "\x1b[33m"
	doctorReset  = "\x1b[0m"
)

var doctorColorEnabled = func(stdout io.Writer) bool {
	file, ok := stdout.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}

type doctorCheck struct {
	Name   string
	OK     bool
	Detail string
	Fix    string
}

func collectDoctorChecks() []doctorCheck {
	checks := make([]doctorCheck, 0, 6)

	executable, executableErr := executableFunc()
	if executableErr != nil {
		checks = append(checks, doctorCheck{
			Name:   "Executable",
			Detail: executableErr.Error(),
			Fix:    "reinstall ctx",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:   "Executable",
			OK:     true,
			Detail: fmt.Sprintf("%s (ctx %s)", executable, Version),
		})
	}

	path, pathErr := lookPathFunc("ctx")
	switch {
	case pathErr != nil:
		checks = append(checks, doctorCheck{
			Name:   "PATH",
			Detail: "ctx is not available on PATH",
			Fix:    "add the ctx installation directory to PATH",
		})
	case executableErr == nil && !sameExecutable(executable, path):
		checks = append(checks, doctorCheck{
			Name:   "PATH",
			Detail: fmt.Sprintf("PATH resolves to %s, but this process is %s", path, executable),
			Fix:    "remove the stale ctx binary or update PATH, then restart the shell",
		})
	default:
		checks = append(checks, doctorCheck{
			Name:   "PATH",
			OK:     true,
			Detail: path,
		})
	}

	config, shellErr := DetectShell()
	if shellErr != nil {
		checks = append(checks, doctorCheck{
			Name:   "Shell",
			Detail: shellErr.Error(),
			Fix:    "set SHELL to zsh or bash",
		})
	} else {
		checks = append(checks, doctorCheck{
			Name:   "Shell",
			OK:     true,
			Detail: fmt.Sprintf("%s (%s)", config.Shell, config.Path),
		})

		configured, err := CompletionConfigured(config)
		switch {
		case err != nil:
			checks = append(checks, doctorCheck{
				Name:   "Shell integration",
				Detail: err.Error(),
				Fix:    "check the shell configuration file permissions, then run ctx init-shell",
			})
		case !configured:
			checks = append(checks, doctorCheck{
				Name:   "Shell integration",
				Detail: fmt.Sprintf("ctx block not found in %s", config.Path),
				Fix:    "ctx init-shell",
			})
		default:
			checks = append(checks, doctorCheck{
				Name:   "Shell integration",
				OK:     true,
				Detail: fmt.Sprintf("configured in %s", config.Path),
			})
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		checks = append(checks, doctorCheck{
			Name:   "Workspace",
			Detail: err.Error(),
			Fix:    "change to an accessible directory",
		})
		return checks
	}
	workspace, err := FindWorkspace(wd)
	switch {
	case errors.Is(err, ErrWorkspaceNotFound):
		checks = append(checks, doctorCheck{
			Name:   "Workspace",
			Detail: fmt.Sprintf("no workspace found from %s", wd),
			Fix:    "ctx workspace init",
		})
	case err != nil:
		checks = append(checks, doctorCheck{
			Name:   "Workspace",
			Detail: err.Error(),
			Fix:    "inspect or remove the invalid .ctx marker",
		})
	default:
		needsUpdate, updateErr := workspaceNeedsUpdate(workspace)
		switch {
		case updateErr != nil:
			checks = append(checks, doctorCheck{
				Name:   "Workspace data",
				Detail: updateErr.Error(),
				Fix:    "check CTX_HOME permissions",
			})
		case needsUpdate:
			checks = append(checks, doctorCheck{
				Name:   "Workspace data",
				Detail: fmt.Sprintf("data is incomplete for %s", workspace.ID),
				Fix:    "ctx workspace init",
			})
		default:
			checks = append(checks, doctorCheck{
				Name:   "Workspace",
				OK:     true,
				Detail: fmt.Sprintf("%s (%s)", workspace.ID, workspace.RootPath),
			})
		}
	}
	return checks
}

func writeDoctorReport(stdout io.Writer, checks []doctorCheck) (int, error) {
	if _, err := fmt.Fprintf(stdout, "ctx doctor %s\n\n", Version); err != nil {
		return 0, err
	}
	okCount := 0
	ngCount := 0
	color := doctorColorEnabled(stdout)
	for _, check := range checks {
		status := "NG"
		statusColor := doctorRed
		if check.OK {
			status = "OK"
			statusColor = doctorGreen
			okCount++
		} else {
			ngCount++
		}
		if color {
			status = statusColor + status + doctorReset
		}
		if _, err := fmt.Fprintf(stdout, "%s  %s\n", status, check.Name); err != nil {
			return 0, err
		}
		if check.Detail != "" {
			if _, err := fmt.Fprintf(stdout, "    %s\n", check.Detail); err != nil {
				return 0, err
			}
		}
		if !check.OK && check.Fix != "" {
			label := "Fix:"
			if color {
				label = doctorYellow + label + doctorReset
			}
			if _, err := fmt.Fprintf(stdout, "    %s %s\n", label, check.Fix); err != nil {
				return 0, err
			}
		}
	}
	okSummary := fmt.Sprintf("%d OK", okCount)
	ngSummary := fmt.Sprintf("%d NG", ngCount)
	if color {
		okSummary = doctorGreen + okSummary + doctorReset
		ngSummary = doctorRed + ngSummary + doctorReset
	}
	if _, err := fmt.Fprintf(stdout, "\n%s, %s\n", okSummary, ngSummary); err != nil {
		return 0, err
	}
	return ngCount, nil
}

func sameExecutable(left, right string) bool {
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	if leftErr == nil && rightErr == nil {
		return os.SameFile(leftInfo, rightInfo)
	}
	leftAbsolute, leftAbsErr := filepath.Abs(left)
	rightAbsolute, rightAbsErr := filepath.Abs(right)
	return leftAbsErr == nil && rightAbsErr == nil && leftAbsolute == rightAbsolute
}
