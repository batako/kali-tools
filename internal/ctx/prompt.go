package ctx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PromptData struct {
	Active         bool   `json:"active"`
	WorkspaceID    string `json:"-"`
	WorkspaceName  string `json:"workspace_name"`
	WorkspacePath  string `json:"workspace_path"`
	LocalIP        string `json:"local_ip"`
	LocalInterface string `json:"local_interface"`
	TargetName     string `json:"target_name"`
	TargetIP       string `json:"target_ip"`
}

var promptLocalAddressFunc = localAddressForTarget

func LoadPromptData(startPath string) (PromptData, error) {
	workspace, err := FindWorkspace(startPath)
	if errors.Is(err, ErrWorkspaceNotFound) {
		return PromptData{}, nil
	}
	if err != nil {
		return PromptData{}, err
	}

	record, err := GetWorkspaceRecord(workspace)
	if err != nil {
		return PromptData{}, err
	}
	data := PromptData{
		Active:        true,
		WorkspaceID:   fmt.Sprintf("%d", record.ID),
		WorkspaceName: filepath.Base(record.RootPath),
		WorkspacePath: record.RootPath,
	}

	targets, err := ListTargets(workspace)
	if err != nil {
		return PromptData{}, err
	}
	for _, target := range targets {
		if target.IsPrimary {
			data.TargetName = target.Name
			data.TargetIP = target.IP
			break
		}
	}

	data.LocalIP, data.LocalInterface = promptLocalAddressFunc(data.TargetIP)
	return data, nil
}

func WritePromptData(stdout io.Writer, data PromptData, format, field string) error {
	if field != "" {
		value, ok := promptField(data, field)
		if !ok {
			return fmt.Errorf("unknown prompt field: %s", field)
		}
		if value == "" {
			return nil
		}
		_, err := fmt.Fprintln(stdout, value)
		return err
	}

	switch format {
	case "shell":
		values := []struct {
			name  string
			value string
		}{
			{"CTX_ACTIVE", boolDigit(data.Active)},
			{"CTX_WORKSPACE_ID", data.WorkspaceID},
			{"CTX_WORKSPACE_NAME", data.WorkspaceName},
			{"CTX_WORKSPACE_PATH", data.WorkspacePath},
			{"CTX_LOCAL_IP", data.LocalIP},
			{"CTX_LOCAL_INTERFACE", data.LocalInterface},
			{"CTX_TARGET_NAME", data.TargetName},
			{"CTX_TARGET_IP", data.TargetIP},
		}
		for _, item := range values {
			if _, err := fmt.Fprintf(stdout, "%s=%s\n", item.name, shellQuote(item.value)); err != nil {
				return err
			}
		}
		return nil
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(data)
	default:
		return fmt.Errorf("unsupported prompt format: %s", format)
	}
}

func promptField(data PromptData, field string) (string, bool) {
	fields := map[string]string{
		"active":          boolDigit(data.Active),
		"workspace-id":    data.WorkspaceID,
		"workspace-name":  data.WorkspaceName,
		"workspace-path":  data.WorkspacePath,
		"local-ip":        data.LocalIP,
		"local-interface": data.LocalInterface,
		"target-name":     data.TargetName,
		"target-ip":       data.TargetIP,
	}
	value, ok := fields[field]
	return value, ok
}

func boolDigit(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func localAddressForTarget(targetIP string) (string, string) {
	if targetIP != "" {
		target := net.ParseIP(targetIP)
		if target != nil {
			connection, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: target, Port: 53})
			if err == nil {
				localIP := connection.LocalAddr().(*net.UDPAddr).IP.String()
				_ = connection.Close()
				return localIP, interfaceNameForIP(localIP)
			}
		}
	}
	return preferredLocalAddress()
}

func preferredLocalAddress() (string, string) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	sort.SliceStable(interfaces, func(i, j int) bool {
		return interfacePriority(interfaces[i].Name) < interfacePriority(interfaces[j].Name)
	})
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ip := addressIP(address)
			if ip != nil && ip.IsGlobalUnicast() {
				return ip.String(), networkInterface.Name
			}
		}
	}
	return "", ""
}

func interfaceNameForIP(value string) string {
	target := net.ParseIP(value)
	if target == nil {
		return ""
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, networkInterface := range interfaces {
		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			if ip := addressIP(address); ip != nil && ip.Equal(target) {
				return networkInterface.Name
			}
		}
	}
	return ""
}

func addressIP(address net.Addr) net.IP {
	switch value := address.(type) {
	case *net.IPNet:
		return value.IP
	case *net.IPAddr:
		return value.IP
	default:
		ip, _, err := net.ParseCIDR(address.String())
		if err == nil {
			return ip
		}
		return net.ParseIP(address.String())
	}
}

func interfacePriority(name string) int {
	lower := strings.ToLower(name)
	for _, prefix := range []string{"tun", "tap", "wg", "tailscale", "ppp"} {
		if strings.HasPrefix(lower, prefix) {
			return 0
		}
	}
	return 1
}

func currentPromptData() (PromptData, error) {
	wd, err := os.Getwd()
	if err != nil {
		return PromptData{}, fmt.Errorf("failed to get current directory: %w", err)
	}
	return LoadPromptData(wd)
}
