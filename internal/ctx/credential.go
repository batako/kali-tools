package ctx

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Credential struct {
	ID       int64
	TargetID int64
	Scope    string
	Username string
	Password string
}

func SetCredential(workspace *Workspace, scope, username, password string) (*Credential, error) {
	target, err := GetPrimaryTarget(workspace)
	if err != nil {
		return nil, err
	}
	return upsertCredential(workspace, target, scope, username, password)
}

func AddCredential(workspace *Workspace, scope, username, password string) (*Credential, error) {
	if err := validateCredentialInput(scope, username); err != nil {
		return nil, err
	}
	target, err := GetPrimaryTarget(workspace)
	if err != nil {
		return nil, err
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	exists, err := credentialExists(db, target.ID, scope, username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("credential already exists: %s %s\nUse ctx credential set %s %s [password] to update it.", scope, username, scope, username)
	}

	var id int64
	err = db.QueryRow(`
		INSERT INTO credentials (
			target_id, scope, username, password, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, target.ID, scope, username, nullPassword(password)).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to add credential: %w", err)
	}
	return GetCredentialByID(workspace, id)
}

func credentialExists(db *sql.DB, targetID int64, scope, username string) (bool, error) {
	var exists int
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM credentials
			WHERE target_id = ? AND scope = ? AND username = ?
		)
	`, targetID, scope, username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to inspect credential: %w", err)
	}
	return exists == 1, nil
}

func UpdateCredential(workspace *Workspace, scope, username, password string) (*Credential, error) {
	if err := validateCredentialInput(scope, username); err != nil {
		return nil, err
	}
	target, err := GetPrimaryTarget(workspace)
	if err != nil {
		return nil, err
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	result, err := db.Exec(`
		UPDATE credentials
		SET password = ?, updated_at = CURRENT_TIMESTAMP
		WHERE target_id = ? AND scope = ? AND username = ?
	`, nullPassword(password), target.ID, scope, username)
	if err != nil {
		return nil, fmt.Errorf("failed to update credential: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect credential update: %w", err)
	}
	if rows == 0 {
		return nil, fmt.Errorf("credential not found: %s %s", scope, username)
	}
	return GetCredentialByTargetScopeUsername(workspace, target.ID, scope, username)
}

func ListCredentials(workspace *Workspace, scope string) ([]Credential, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `
		SELECT c.id, c.target_id, c.scope, c.username, c.password
		FROM credentials c
		JOIN targets t ON t.id = c.target_id
		WHERE t.workspace_id = ?
	`
	args := []any{workspace.ID}
	if scope != "" {
		query += ` AND c.scope = ?`
		args = append(args, scope)
	}
	query += ` ORDER BY c.id ASC`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", err)
	}
	defer rows.Close()
	return scanCredentials(rows)
}

func FindCredentialsByUsername(workspace *Workspace, username string) ([]Credential, error) {
	if strings.TrimSpace(username) == "" {
		return nil, errors.New("username is required")
	}
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT c.id, c.target_id, c.scope, c.username, c.password
		FROM credentials c
		JOIN targets t ON t.id = c.target_id
		WHERE t.workspace_id = ? AND c.username = ?
		ORDER BY c.id ASC
	`, workspace.ID, username)
	if err != nil {
		return nil, fmt.Errorf("failed to find credentials: %w", err)
	}
	defer rows.Close()
	return scanCredentials(rows)
}

func FindCredentialsByScopeUsername(workspace *Workspace, scope, username string) ([]Credential, error) {
	if err := validateCredentialInput(scope, username); err != nil {
		return nil, err
	}
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT c.id, c.target_id, c.scope, c.username, c.password
		FROM credentials c
		JOIN targets t ON t.id = c.target_id
		WHERE t.workspace_id = ? AND c.scope = ? AND c.username = ?
		ORDER BY c.id ASC
	`, workspace.ID, scope, username)
	if err != nil {
		return nil, fmt.Errorf("failed to find credentials: %w", err)
	}
	defer rows.Close()
	return scanCredentials(rows)
}

func GetCredentialByID(workspace *Workspace, id int64) (*Credential, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT c.id, c.target_id, c.scope, c.username, c.password
		FROM credentials c
		JOIN targets t ON t.id = c.target_id
		WHERE t.workspace_id = ? AND c.id = ?
	`, workspace.ID, id)
	credential, err := scanCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("credential not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load credential: %w", err)
	}
	return credential, nil
}

func RemoveCredential(workspace *Workspace, id int64) error {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return err
	}
	defer db.Close()

	result, err := db.Exec(`
		DELETE FROM credentials
		WHERE id = ? AND target_id IN (
			SELECT id FROM targets WHERE workspace_id = ?
		)
	`, id, workspace.ID)
	if err != nil {
		return fmt.Errorf("failed to remove credential: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect credential removal: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("credential not found: %d", id)
	}
	return nil
}

func upsertCredential(workspace *Workspace, target *Target, scope, username, password string) (*Credential, error) {
	if err := validateCredentialInput(scope, username); err != nil {
		return nil, err
	}
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var id int64
	err = db.QueryRow(`
		INSERT INTO credentials (
			target_id, scope, username, password, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(target_id, scope, username) DO UPDATE SET
			password = excluded.password,
			updated_at = CURRENT_TIMESTAMP
		RETURNING id
	`, target.ID, scope, username, nullPassword(password)).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to save credential: %w", err)
	}
	return GetCredentialByID(workspace, id)
}

func GetCredentialByTargetScopeUsername(workspace *Workspace, targetID int64, scope, username string) (*Credential, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT c.id, c.target_id, c.scope, c.username, c.password
		FROM credentials c
		JOIN targets t ON t.id = c.target_id
		WHERE t.workspace_id = ? AND c.target_id = ? AND c.scope = ? AND c.username = ?
	`, workspace.ID, targetID, scope, username)
	credential, err := scanCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("credential not found: %s %s", scope, username)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load credential: %w", err)
	}
	return credential, nil
}

func scanCredentials(rows *sql.Rows) ([]Credential, error) {
	var credentials []Credential
	for rows.Next() {
		credential, err := scanCredential(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to read credential: %w", err)
		}
		credentials = append(credentials, *credential)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read credentials: %w", err)
	}
	return credentials, nil
}

type credentialScanner interface {
	Scan(dest ...any) error
}

func scanCredential(scanner credentialScanner) (*Credential, error) {
	var credential Credential
	var password sql.NullString
	if err := scanner.Scan(
		&credential.ID,
		&credential.TargetID,
		&credential.Scope,
		&credential.Username,
		&password,
	); err != nil {
		return nil, err
	}
	if password.Valid {
		credential.Password = password.String
	}
	return &credential, nil
}

func validateCredentialInput(scope, username string) error {
	if strings.TrimSpace(scope) == "" {
		return errors.New("scope is required")
	}
	if strings.TrimSpace(username) == "" {
		return errors.New("username is required")
	}
	return nil
}

func nullPassword(password string) any {
	if password == "" {
		return nil
	}
	return password
}
