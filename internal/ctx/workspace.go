package ctx

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const MarkerFile = ".ctx"

var ErrWorkspaceNotFound = errors.New("ctx workspace not found (run ctx workspace init)")

type Workspace struct {
	ID           int64
	UUID         string
	Name         string
	RootPath     string
	DataPath     string
	DatabasePath string
}

type WorkspaceInitStatus int

const (
	WorkspaceCreated WorkspaceInitStatus = iota
	WorkspaceUpdated
	WorkspaceUnchanged
)

func InitWorkspace(rootPath string) (*Workspace, error) {
	workspace, _, err := InitWorkspaceWithStatus(rootPath)
	return workspace, err
}

func InitWorkspaceWithStatus(rootPath string) (*Workspace, WorkspaceInitStatus, error) {
	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, WorkspaceUnchanged, fmt.Errorf("failed to resolve workspace root: %w", err)
	}

	markerPath := filepath.Join(rootPath, MarkerFile)
	if existingUUID, err := readWorkspaceMarker(markerPath); err == nil {
		workspace := workspaceFromUUID(existingUUID, rootPath)
		recordExists, err := WorkspaceRecordExists(workspace)
		if err != nil {
			return nil, WorkspaceUnchanged, err
		}
		needsUpdate, err := workspaceNeedsUpdate(workspace)
		if err != nil {
			return nil, WorkspaceUnchanged, err
		}
		if err := EnsureDatabase(workspace); err != nil {
			return nil, WorkspaceUnchanged, err
		}
		record, err := GetWorkspaceRecordByUUID(workspace.UUID)
		if err != nil {
			return nil, WorkspaceUnchanged, err
		}
		workspace = workspaceFromRecord(*record)
		if err := ensureWorkspaceDirs(workspace.DataPath); err != nil {
			return nil, WorkspaceUnchanged, err
		}
		if needsUpdate || !recordExists {
			return workspace, WorkspaceUpdated, nil
		}
		return workspace, WorkspaceUnchanged, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, WorkspaceUnchanged, err
	}

	record, err := GetWorkspaceRecordByRoot(rootPath)
	if err != nil {
		return nil, WorkspaceUnchanged, err
	}
	if record != nil {
		workspace := workspaceFromRecord(*record)
		if err := os.WriteFile(markerPath, []byte(formatWorkspaceMarker(workspace.UUID)+"\n"), 0644); err != nil {
			return nil, WorkspaceUnchanged, fmt.Errorf("failed to restore %s: %w", markerPath, err)
		}
		if err := ensureWorkspaceDirs(workspace.DataPath); err != nil {
			return nil, WorkspaceUnchanged, err
		}
		if err := EnsureDatabase(workspace); err != nil {
			return nil, WorkspaceUnchanged, err
		}
		return workspace, WorkspaceUpdated, nil
	}

	uuid, err := newWorkspaceUUID()
	if err != nil {
		return nil, WorkspaceUnchanged, err
	}

	workspace := workspaceFromUUID(uuid, rootPath)
	if err := ensureWorkspaceDirs(workspace.DataPath); err != nil {
		return nil, WorkspaceUnchanged, err
	}

	if err := EnsureDatabase(workspace); err != nil {
		return nil, WorkspaceUnchanged, err
	}

	record, err = GetWorkspaceRecordByUUID(workspace.UUID)
	if err != nil {
		return nil, WorkspaceUnchanged, err
	}
	workspace = workspaceFromRecord(*record)
	if err := os.WriteFile(markerPath, []byte(formatWorkspaceMarker(workspace.UUID)+"\n"), 0644); err != nil {
		return nil, WorkspaceUnchanged, fmt.Errorf("failed to write %s: %w", markerPath, err)
	}
	return workspace, WorkspaceCreated, nil
}

func FindWorkspace(startPath string) (*Workspace, error) {
	current, err := filepath.Abs(startPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve start path: %w", err)
	}

	info, err := os.Stat(current)
	if err != nil {
		return nil, fmt.Errorf("failed to stat %s: %w", current, err)
	}
	if !info.IsDir() {
		current = filepath.Dir(current)
	}

	for {
		markerPath := filepath.Join(current, MarkerFile)
		uuid, err := readWorkspaceMarker(markerPath)
		if err == nil {
			workspace := workspaceFromUUID(uuid, current)
			if err := EnsureDatabase(workspace); err != nil {
				return nil, err
			}
			record, err := GetWorkspaceRecordByUUID(workspace.UUID)
			if err != nil {
				return nil, err
			}
			return workspaceFromRecord(*record), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return nil, ErrWorkspaceNotFound
		}
		current = parent
	}
}

func readWorkspaceMarker(markerPath string) (string, error) {
	content, err := os.ReadFile(markerPath)
	if err != nil {
		return "", err
	}

	value := strings.TrimSpace(string(content))
	if value == "" {
		return "", fmt.Errorf("invalid ctx marker %s: empty workspace UUID", markerPath)
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("invalid ctx marker %s: expected one UUID", markerPath)
	}
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != value {
		return "", fmt.Errorf("invalid ctx marker %s: expected canonical UUID", markerPath)
	}
	return value, nil
}

func workspaceFromUUID(uuid, rootPath string) *Workspace {
	return &Workspace{
		UUID:         uuid,
		Name:         filepath.Base(rootPath),
		RootPath:     rootPath,
		DataPath:     filepath.Join(dataRoot(), "workspaces", uuid),
		DatabasePath: filepath.Join(dataRoot(), "db.sqlite"),
	}
}

func workspaceFromRecord(record WorkspaceRecord) *Workspace {
	workspace := workspaceFromUUID(record.UUID, record.RootPath)
	workspace.ID = record.ID
	workspace.Name = workspaceName(record.RootPath)
	return workspace
}

func workspaceName(rootPath string) string {
	if name := filepath.Base(rootPath); name != "." && name != string(filepath.Separator) {
		return name
	}
	return rootPath
}

func formatWorkspaceMarker(uuid string) string {
	return uuid
}

func ensureWorkspaceDirs(dataPath string) error {
	for _, dir := range []string{
		dataRoot(),
		dataPath,
		filepath.Join(dataPath, "logs"),
		filepath.Join(dataPath, "files"),
		filepath.Join(dataPath, "scans"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	return nil
}

func workspaceNeedsUpdate(workspace *Workspace) (bool, error) {
	for _, dir := range []string{
		workspace.DataPath,
		filepath.Join(workspace.DataPath, "logs"),
		filepath.Join(workspace.DataPath, "files"),
		filepath.Join(workspace.DataPath, "scans"),
	} {
		info, err := os.Stat(dir)
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to inspect %s: %w", dir, err)
		}
		if !info.IsDir() {
			return true, nil
		}
	}
	ready, err := WorkspaceDatabaseReady(workspace)
	if err != nil {
		return false, err
	}
	return !ready, nil
}

func dataRoot() string {
	if value := os.Getenv("CTX_HOME"); value != "" {
		return value
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".ctx"
	}
	return filepath.Join(home, ".ctx")
}

func newWorkspaceID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("failed to generate workspace id: %w", err)
	}

	// RFC 9562 UUID version 4.
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		raw[0:4],
		raw[4:6],
		raw[6:8],
		raw[8:10],
		raw[10:16],
	), nil
}

func newWorkspaceUUID() (string, error) {
	return newWorkspaceID()
}
