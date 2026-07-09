package ctx

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type Service struct {
	ID          int64
	WorkspaceID string
	TargetID    int64
	Port        int
	Protocol    string
	State       string
	Reason      string
	ServiceName string
	Product     string
	Version     string
	ExtraInfo   string
	Tunnel      string
	Hostname    string
	CPE         string
	ScriptsJSON string
	LastSeen    string
}

func UpsertService(workspace *Workspace, target *Target, service Service) (int64, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var existingID int64
	err = db.QueryRow(`
		SELECT id
		FROM service
		WHERE workspace_id = ? AND target_id = ? AND port = ? AND protocol = ?
		ORDER BY id ASC
		LIMIT 1
	`, workspace.ID, target.ID, service.Port, service.Protocol).Scan(&existingID)
	switch err {
	case nil:
		_, err = db.Exec(`
			UPDATE service
			SET state = ?, reason = ?, service_name = ?, product = ?, version = ?,
			    extrainfo = ?, tunnel = ?, hostname = ?, cpe = ?, scripts_json = ?,
			    last_seen = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, service.State, service.Reason, service.ServiceName, service.Product, service.Version,
			service.ExtraInfo, service.Tunnel, service.Hostname, service.CPE, nullableJSONString(service.ScriptsJSON),
			service.LastSeen, existingID)
		if err != nil {
			return 0, fmt.Errorf("failed to update service: %w", err)
		}
		return existingID, nil
	case sql.ErrNoRows:
	default:
		return 0, fmt.Errorf("failed to inspect service: %w", err)
	}

	result, err := db.Exec(`
		INSERT INTO service (
			workspace_id, target_id, port, protocol, state, reason, service_name,
			product, version, extrainfo, tunnel, hostname, cpe, scripts_json, last_seen,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, workspace.ID, target.ID, service.Port, service.Protocol, service.State, service.Reason,
		service.ServiceName, service.Product, service.Version, service.ExtraInfo,
		service.Tunnel, service.Hostname, service.CPE, nullableJSONString(service.ScriptsJSON), service.LastSeen)
	if err != nil {
		return 0, fmt.Errorf("failed to save service: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to inspect service id: %w", err)
	}
	return id, nil
}

func ListServices(workspace *Workspace, target *Target) ([]Service, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT id, workspace_id, target_id, port, protocol, COALESCE(state, ''), COALESCE(reason, ''),
		       COALESCE(service_name, ''), COALESCE(product, ''), COALESCE(version, ''),
		       COALESCE(extrainfo, ''), COALESCE(tunnel, ''), COALESCE(hostname, ''),
		       COALESCE(cpe, ''), COALESCE(scripts_json, ''), COALESCE(last_seen, '')
		FROM service
		WHERE workspace_id = ? AND target_id = ?
		ORDER BY port ASC, protocol ASC, id ASC
	`, workspace.ID, target.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var service Service
		if err := rows.Scan(
			&service.ID,
			&service.WorkspaceID,
			&service.TargetID,
			&service.Port,
			&service.Protocol,
			&service.State,
			&service.Reason,
			&service.ServiceName,
			&service.Product,
			&service.Version,
			&service.ExtraInfo,
			&service.Tunnel,
			&service.Hostname,
			&service.CPE,
			&service.ScriptsJSON,
			&service.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to read service: %w", err)
		}
		services = append(services, service)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	return services, nil
}

func marshalServiceScripts(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal service scripts: %w", err)
	}
	return string(data), nil
}

func nullableJSONString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
