package ctx

import (
	"database/sql"
	"fmt"
)

type Service struct {
	ID               int64
	WorkspaceID      int64
	TargetID         int64
	Port             int
	Protocol         string
	State            string
	StateValid       bool
	Reason           string
	ReasonValid      bool
	ServiceName      string
	ServiceNameValid bool
	Product          string
	ProductValid     bool
	Version          string
	VersionValid     bool
	ExtraInfo        string
	ExtraInfoValid   bool
	Tunnel           string
	TunnelValid      bool
	Hostname         string
	CPE              string
	CPEValid         bool
	ScriptsJSON      string
	LastSeen         string
	LastSeenValid    bool
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
		SELECT id, target_id, port, protocol, state, reason,
		       service_name, product, version, extrainfo, tunnel, cpe,
		       last_seen
		FROM services
		WHERE target_id = ?
		ORDER BY protocol ASC, port ASC, id ASC
	`, target.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		var service Service
		var state, reason, serviceName, product, version sql.NullString
		var extraInfo, tunnel, cpe, lastSeen sql.NullString
		if err := rows.Scan(
			&service.ID,
			&service.TargetID,
			&service.Port,
			&service.Protocol,
			&state,
			&reason,
			&serviceName,
			&product,
			&version,
			&extraInfo,
			&tunnel,
			&cpe,
			&lastSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to read service: %w", err)
		}
		service.WorkspaceID = workspace.ID
		service.State, service.StateValid = nullableStringValue(state)
		service.Reason, service.ReasonValid = nullableStringValue(reason)
		service.ServiceName, service.ServiceNameValid = nullableStringValue(serviceName)
		service.Product, service.ProductValid = nullableStringValue(product)
		service.Version, service.VersionValid = nullableStringValue(version)
		service.ExtraInfo, service.ExtraInfoValid = nullableStringValue(extraInfo)
		service.Tunnel, service.TunnelValid = nullableStringValue(tunnel)
		service.CPE, service.CPEValid = nullableStringValue(cpe)
		service.LastSeen, service.LastSeenValid = nullableStringValue(lastSeen)
		services = append(services, service)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	return services, nil
}

func nullableStringValue(value sql.NullString) (string, bool) {
	if !value.Valid {
		return "", false
	}
	return value.String, true
}
