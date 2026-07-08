package ctx

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
)

type Host struct {
	ID         int64
	Hostname   string
	TargetName string
	TargetIP   string
	Source     string
}

func AddHost(workspace *Workspace, hostname, targetName string) (*Host, error) {
	hostname, err := normalizeHostname(hostname)
	if err != nil {
		return nil, err
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	target, err := hostTarget(db, workspace.ID, targetName)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		INSERT INTO host (workspace_id, target_id, hostname, source, created_at, updated_at)
		VALUES (?, ?, ?, 'manual', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(workspace_id, hostname) DO UPDATE SET
			target_id = excluded.target_id,
			source = excluded.source,
			updated_at = CURRENT_TIMESTAMP
	`, workspace.ID, target.ID, hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to add host: %w", err)
	}

	return GetHost(workspace, hostname)
}

func RemoveHost(workspace *Workspace, hostname string) error {
	hostname, err := normalizeHostname(hostname)
	if err != nil {
		return err
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return err
	}
	defer db.Close()

	result, err := db.Exec(`DELETE FROM host WHERE workspace_id = ? AND hostname = ?`, workspace.ID, hostname)
	if err != nil {
		return fmt.Errorf("failed to remove host: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect host removal: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("host not found: %s", hostname)
	}

	return nil
}

func ListHosts(workspace *Workspace) ([]Host, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT h.id, h.hostname, t.name, t.ip, COALESCE(h.source, '')
		FROM host h
		LEFT JOIN target t ON t.id = h.target_id
		WHERE h.workspace_id = ?
		ORDER BY h.hostname
	`, workspace.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}
	defer rows.Close()

	var hosts []Host
	for rows.Next() {
		var host Host
		if err := rows.Scan(&host.ID, &host.Hostname, &host.TargetName, &host.TargetIP, &host.Source); err != nil {
			return nil, fmt.Errorf("failed to read host: %w", err)
		}
		hosts = append(hosts, host)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	return hosts, nil
}

func GetHost(workspace *Workspace, hostname string) (*Host, error) {
	hostname, err := normalizeHostname(hostname)
	if err != nil {
		return nil, err
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var host Host
	err = db.QueryRow(`
		SELECT h.id, h.hostname, t.name, t.ip, COALESCE(h.source, '')
		FROM host h
		LEFT JOIN target t ON t.id = h.target_id
		WHERE h.workspace_id = ? AND h.hostname = ?
	`, workspace.ID, hostname).Scan(&host.ID, &host.Hostname, &host.TargetName, &host.TargetIP, &host.Source)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("host not found: %s", hostname)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load host: %w", err)
	}

	return &host, nil
}

func hostTarget(db *sql.DB, workspaceID, targetName string) (*Target, error) {
	var target Target
	var isPrimary int
	var err error

	if targetName == "" {
		err = db.QueryRow(`
			SELECT id, name, ip, is_primary
			FROM target
			WHERE workspace_id = ? AND is_primary = 1
			ORDER BY id
			LIMIT 1
		`, workspaceID).Scan(&target.ID, &target.Name, &target.IP, &isPrimary)
	} else {
		err = db.QueryRow(`
			SELECT id, name, ip, is_primary
			FROM target
			WHERE workspace_id = ? AND name = ?
		`, workspaceID, targetName).Scan(&target.ID, &target.Name, &target.IP, &isPrimary)
	}
	if err == sql.ErrNoRows && targetName == "" {
		return nil, errors.New("primary target not set")
	}
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("target not found: %s", targetName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load target: %w", err)
	}

	target.IsPrimary = isPrimary == 1
	return &target, nil
}

func normalizeHostname(hostname string) (string, error) {
	hostname = strings.TrimSpace(strings.TrimSuffix(hostname, "."))
	if hostname == "" {
		return "", errors.New("hostname is required")
	}
	if len(hostname) > 253 {
		return "", fmt.Errorf("invalid hostname: %s", hostname)
	}
	if strings.ContainsAny(hostname, `/\\:`) {
		return "", fmt.Errorf("invalid hostname: %s", hostname)
	}
	for _, r := range hostname {
		if unicode.IsSpace(r) {
			return "", fmt.Errorf("invalid hostname: %s", hostname)
		}
	}

	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 {
			return "", fmt.Errorf("invalid hostname: %s", hostname)
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid hostname: %s", hostname)
		}
		for _, r := range label {
			if r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
				continue
			}
			return "", fmt.Errorf("invalid hostname: %s", hostname)
		}
	}

	return strings.ToLower(hostname), nil
}

func RenderHostsBlock(workspace *Workspace) (string, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return "", err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT t.id, t.ip, h.hostname
		FROM target t
		JOIN host h ON h.target_id = t.id
		WHERE t.workspace_id = ? AND h.workspace_id = ?
		ORDER BY t.id, h.hostname
	`, workspace.ID, workspace.ID)
	if err != nil {
		return "", fmt.Errorf("failed to list host mappings: %w", err)
	}
	defer rows.Close()

	var b strings.Builder
	found := false
	fmt.Fprintf(&b, "# >>> ctx: %s\n", workspace.ID)

	var currentTargetID int64
	var currentIP string
	var hostnames []string
	flush := func() {
		if currentIP == "" || len(hostnames) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s %s\n", currentIP, strings.Join(hostnames, " "))
	}

	for rows.Next() {
		var targetID int64
		var ip string
		var hostname string
		if err := rows.Scan(&targetID, &ip, &hostname); err != nil {
			return "", fmt.Errorf("failed to read host mapping: %w", err)
		}
		if currentIP != "" && targetID != currentTargetID {
			flush()
			hostnames = nil
		}
		currentTargetID = targetID
		currentIP = ip
		found = true
		hostnames = append(hostnames, hostname)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("failed to list host mappings: %w", err)
	}
	flush()
	if !found {
		return "", nil
	}

	fmt.Fprintf(&b, "# <<< ctx: %s\n", workspace.ID)
	return b.String(), nil
}

func SyncHostsFile(workspace *Workspace, hostsPath string) error {
	block, err := RenderHostsBlock(workspace)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read hosts file %s: %w", hostsPath, err)
	}

	updated, err := replaceHostsBlock(string(content), workspace.ID, block)
	if err != nil {
		return err
	}
	if err := os.WriteFile(hostsPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to write hosts file %s: %w", hostsPath, err)
	}

	return nil
}

func replaceHostsBlock(content, workspaceID, block string) (string, error) {
	startMarker := fmt.Sprintf("# >>> ctx: %s", workspaceID)
	endMarker := fmt.Sprintf("# <<< ctx: %s", workspaceID)

	lines := splitLines(content)
	start := -1
	end := -1
	for i, line := range lines {
		switch line {
		case startMarker:
			if start != -1 {
				return "", fmt.Errorf("multiple ctx hosts blocks found for workspace %s", workspaceID)
			}
			start = i
		case endMarker:
			if start != -1 && end == -1 {
				end = i
			}
		}
	}
	if start != -1 && end == -1 {
		return "", fmt.Errorf("unterminated ctx hosts block for workspace %s", workspaceID)
	}
	if start == -1 && end != -1 {
		return "", fmt.Errorf("ctx hosts end marker without start for workspace %s", workspaceID)
	}

	block = strings.TrimRight(block, "\n")
	if block == "" {
		if start == -1 {
			return strings.Join(lines, "\n") + newlineSuffix(lines), nil
		}
		deleteStart := start
		if deleteStart > 0 && lines[deleteStart-1] == "" {
			deleteStart--
		}
		lines = append(lines[:deleteStart], lines[end+1:]...)
		return strings.Join(lines, "\n") + newlineSuffix(lines), nil
	}

	blockLines := strings.Split(block, "\n")
	if start != -1 {
		lines = append(append(lines[:start], blockLines...), lines[end+1:]...)
	} else {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, blockLines...)
	}

	return strings.Join(lines, "\n") + "\n", nil
}

func splitLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func newlineSuffix(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return "\n"
}

func CleanHostsFile(workspace *Workspace, hostsPath string) error {
	content, err := os.ReadFile(hostsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read hosts file %s: %w", hostsPath, err)
	}

	updated, err := replaceHostsBlock(string(content), workspace.ID, "")
	if err != nil {
		return err
	}
	if err := os.WriteFile(hostsPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to write hosts file %s: %w", hostsPath, err)
	}

	return nil
}
