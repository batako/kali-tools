package ctx

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const MarkerFile = ".ctx"

var ErrWorkspaceNotFound = errors.New("ctx workspace not found (run ctx workspace init)")

type Workspace struct {
	ID           string
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
	if existingID, err := readWorkspaceID(markerPath); err == nil {
		workspace := workspaceFromID(existingID, rootPath)
		needsUpdate, err := workspaceNeedsUpdate(workspace)
		if err != nil {
			return nil, WorkspaceUnchanged, err
		}
		if err := ensureWorkspaceDirs(workspace.DataPath); err != nil {
			return nil, WorkspaceUnchanged, err
		}
		if err := EnsureDatabase(workspace); err != nil {
			return nil, WorkspaceUnchanged, err
		}
		if needsUpdate {
			return workspace, WorkspaceUpdated, nil
		}
		return workspace, WorkspaceUnchanged, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, WorkspaceUnchanged, err
	}

	id, err := newWorkspaceID()
	if err != nil {
		return nil, WorkspaceUnchanged, err
	}

	if err := os.WriteFile(markerPath, []byte(id+"\n"), 0644); err != nil {
		return nil, WorkspaceUnchanged, fmt.Errorf("failed to write %s: %w", markerPath, err)
	}

	workspace := workspaceFromID(id, rootPath)
	if err := ensureWorkspaceDirs(workspace.DataPath); err != nil {
		return nil, WorkspaceUnchanged, err
	}

	if err := EnsureDatabase(workspace); err != nil {
		return nil, WorkspaceUnchanged, err
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
		id, err := readWorkspaceID(markerPath)
		if err == nil {
			return workspaceFromID(id, current), nil
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

func readWorkspaceID(markerPath string) (string, error) {
	content, err := os.ReadFile(markerPath)
	if err != nil {
		return "", err
	}

	id := strings.TrimSpace(string(content))
	if id == "" {
		return "", fmt.Errorf("invalid ctx marker %s: empty workspace id", markerPath)
	}
	if strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("invalid ctx marker %s: workspace id must not contain path separators", markerPath)
	}

	return id, nil
}

func workspaceFromID(id, rootPath string) *Workspace {
	return &Workspace{
		ID:           id,
		RootPath:     rootPath,
		DataPath:     filepath.Join(dataRoot(), "workspaces", id),
		DatabasePath: filepath.Join(dataRoot(), "db.sqlite"),
	}
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
	exists, err := WorkspaceRecordExists(workspace)
	if err != nil {
		return false, err
	}
	return !exists, nil
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
