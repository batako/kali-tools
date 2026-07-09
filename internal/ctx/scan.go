package ctx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type scanOptions struct {
	IP     string
	Ports  string
	DryRun bool
	Force  bool
}

type nmapRun struct {
	XMLName xml.Name   `xml:"nmaprun"`
	Hosts   []nmapHost `xml:"host"`
}

type nmapHost struct {
	Ports nmapPorts `xml:"ports"`
}

type nmapPorts struct {
	Ports []nmapPort `xml:"port"`
}

type nmapPort struct {
	Protocol string          `xml:"protocol,attr"`
	PortID   int             `xml:"portid,attr"`
	State    nmapPortState   `xml:"state"`
	Service  nmapPortService `xml:"service"`
	Scripts  []nmapScript    `xml:"script"`
}

type nmapPortState struct {
	State  string `xml:"state,attr"`
	Reason string `xml:"reason,attr"`
}

type nmapPortService struct {
	Name      string   `xml:"name,attr"`
	Product   string   `xml:"product,attr"`
	Version   string   `xml:"version,attr"`
	ExtraInfo string   `xml:"extrainfo,attr"`
	Tunnel    string   `xml:"tunnel,attr"`
	Hostname  string   `xml:"hostname,attr"`
	CPE       []string `xml:"cpe"`
}

type nmapScript struct {
	ID     string `xml:"id,attr"`
	Output string `xml:"output,attr"`
}

func RunScan(args []string, stdout, stderr io.Writer) int {
	options, err := parseScanArgs(args[1:])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	workspace, err := currentWorkspace()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	target, resolvedIP, err := resolveScanTarget(workspace, options.IP)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	startedAt := time.Now().UTC()
	stamp := startedAt.Format("20060102-150405.000000000")
	base := filepath.Join(workspace.DataPath, "scans", "nmap-"+stamp)
	normalPath := base + ".txt"
	xmlPath := base + ".xml"

	commandArgs := []string{"nmap", "-Pn", "-n", "-sV", "-oN", normalPath, "-oX", xmlPath}
	if options.Ports != "" {
		commandArgs = append(commandArgs, "-p", options.Ports)
	}
	commandArgs = append(commandArgs, resolvedIP)

	if options.DryRun {
		_, _ = fmt.Fprintln(stdout, commandString(commandArgs))
		return 0
	}

	if !options.Force {
		logID, found, err := findScanRun(workspace, target, resolvedIP, options.Ports)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if found {
			_, _ = fmt.Fprintf(stdout, "Skipped duplicate scan for %s (previous log: %d)\n", resolvedIP, logID)
			_, _ = fmt.Fprintln(stdout, "Use --force to run it again.")
			return 0
		}
	}

	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	cmd := exec.Command(commandArgs[0], commandArgs[1:]...)
	cmd.Stdout = io.MultiWriter(stdout, &stdoutBuffer)
	cmd.Stderr = io.MultiWriter(stderr, &stderrBuffer)

	exitCode := 0
	err = cmd.Run()
	if err != nil {
		exitCode = commandExitCode(err)
		if exitCode == 127 {
			message := fmt.Sprintf("scan: %v\n", err)
			_, _ = io.WriteString(stderr, message)
			_, _ = stderrBuffer.WriteString(message)
		}
	}
	endedAt := time.Now().UTC()

	status := "success"
	if exitCode != 0 {
		status = "failed"
	}
	logID, saveErr := SaveCommandLog(workspace, CommandLog{
		Command:         scanInvocationCommand(args),
		ExpandedCommand: commandString(commandArgs),
		Status:          status,
		ExitCode:        exitCode,
		Stdout:          stdoutBuffer.String(),
		Stderr:          stderrBuffer.String(),
		StartedAt:       startedAt.Format(time.RFC3339Nano),
		EndedAt:         endedAt.Format(time.RFC3339Nano),
	})
	if saveErr != nil {
		fmt.Fprintln(stderr, saveErr)
		return 1
	}
	if exitCode != 0 {
		return exitCode
	}

	services, err := parseNmapXMLFile(xmlPath, endedAt)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	for _, service := range services {
		if _, err := UpsertService(workspace, target, service); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := saveScanRun(workspace, target, resolvedIP, options.Ports, logID); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "\nSaved %d service record(s) for %s (%s)\n", len(services), target.Name, target.IP)
	_, _ = fmt.Fprintf(stdout, "Artifacts: %s  %s\n", normalPath, xmlPath)
	_, _ = fmt.Fprintf(stdout, "Log: ctx log %d\n", logID)
	return 0
}

func scanInvocationCommand(args []string) string {
	command := []string{"ctx", "scan"}
	if os.Getenv("CTX_INVOKED_AS") == "xscan" {
		command = []string{"xscan"}
	}
	return commandString(append(command, args[1:]...))
}

func parseScanArgs(args []string) (scanOptions, error) {
	var options scanOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n", "--dry-run":
			options.DryRun = true
		case "-f", "--force":
			options.Force = true
		case "-p", "--ports":
			if i+1 >= len(args) {
				return scanOptions{}, fmt.Errorf("usage: ctx scan [ip] [-p <ports>] [-n]")
			}
			i++
			options.Ports = strings.TrimSpace(args[i])
			if options.Ports == "" {
				return scanOptions{}, fmt.Errorf("scan ports must not be empty")
			}
		default:
			if strings.HasPrefix(args[i], "-p") && len(args[i]) > 2 {
				options.Ports = args[i][2:]
				if options.Ports == "" {
					return scanOptions{}, fmt.Errorf("scan ports must not be empty")
				}
				continue
			}
			if strings.HasPrefix(args[i], "-") {
				return scanOptions{}, fmt.Errorf("unknown ctx scan option: %s", args[i])
			}
			if options.IP != "" {
				return scanOptions{}, fmt.Errorf("usage: ctx scan [ip] [-p <ports>] [-n]")
			}
			options.IP = args[i]
		}
	}
	return options, nil
}

func resolveScanTarget(workspace *Workspace, explicitIP string) (*Target, string, error) {
	if explicitIP == "" {
		target, err := GetPrimaryTarget(workspace)
		if err != nil {
			return nil, "", err
		}
		return target, target.IP, nil
	}
	if err := validateIP(explicitIP); err != nil {
		return nil, "", err
	}

	target, err := GetTargetByIP(workspace, explicitIP)
	if err == nil {
		return target, explicitIP, nil
	}
	created, addErr := AddTarget(workspace, explicitIP, "")
	if addErr != nil {
		return nil, "", addErr
	}
	return created, explicitIP, nil
}

func parseNmapXMLFile(path string, finishedAt time.Time) ([]Service, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read nmap XML %s: %w", path, err)
	}

	var report nmapRun
	if err := xml.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("failed to parse nmap XML %s: %w", path, err)
	}

	lastSeen := finishedAt.Format(time.RFC3339Nano)
	services := make([]Service, 0)
	for _, host := range report.Hosts {
		for _, port := range host.Ports.Ports {
			scriptsJSON, err := marshalServiceScripts(port.Scripts)
			if err != nil {
				return nil, err
			}
			services = append(services, Service{
				Port:        port.PortID,
				Protocol:    port.Protocol,
				State:       port.State.State,
				Reason:      port.State.Reason,
				ServiceName: port.Service.Name,
				Product:     port.Service.Product,
				Version:     port.Service.Version,
				ExtraInfo:   port.Service.ExtraInfo,
				Tunnel:      port.Service.Tunnel,
				Hostname:    port.Service.Hostname,
				CPE:         strings.Join(port.Service.CPE, "\n"),
				ScriptsJSON: scriptsJSON,
				LastSeen:    lastSeen,
			})
		}
	}
	return services, nil
}
