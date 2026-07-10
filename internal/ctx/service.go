package ctx

import (
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

	var id int64
	err = db.QueryRow(`
		INSERT INTO services (
			target_id, port, protocol, state, reason, service_name,
			product, version, extrainfo, tunnel, cpe, last_seen,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(target_id, port, protocol) DO UPDATE SET
			state = excluded.state,
			reason = excluded.reason,
			service_name = excluded.service_name,
			product = excluded.product,
			version = excluded.version,
			extrainfo = excluded.extrainfo,
			tunnel = excluded.tunnel,
			cpe = excluded.cpe,
			last_seen = excluded.last_seen,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`, target.ID, service.Port, service.Protocol, service.State, service.Reason,
		service.ServiceName, service.Product, service.Version, service.ExtraInfo,
		service.Tunnel, service.CPE, service.LastSeen).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to save service: %w", err)
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
		SELECT id, target_id, port, protocol, COALESCE(state, ''), COALESCE(reason, ''),
		       COALESCE(service_name, ''), COALESCE(product, ''), COALESCE(version, ''),
		       COALESCE(extrainfo, ''), COALESCE(tunnel, ''), COALESCE(cpe, ''),
		       COALESCE(last_seen, '')
		FROM services
		WHERE target_id = ?
		ORDER BY port ASC, protocol ASC, id ASC
	`, target.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var service Service
		if err := rows.Scan(
			&service.ID,
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
			&service.CPE,
			&service.LastSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to read service: %w", err)
		}
		service.WorkspaceID = workspace.ID
		services = append(services, service)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	return services, nil
}
