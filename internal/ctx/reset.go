package ctx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CtxHostsWorkspaceIDs(hostsPath string) ([]string, error) {
	content, err := os.ReadFile(hostsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read hosts file %s: %w", hostsPath, err)
	}

	const prefix = "# >>> ctx: "
	seen := make(map[string]struct{})
	var ids []string
	for _, line := range splitLines(string(content)) {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		id := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func ResetHostsBlocks(hostsPath string, workspaceIDs []string) error {
	if len(workspaceIDs) == 0 {
		return nil
	}
	content, err := os.ReadFile(hostsPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read hosts file %s: %w", hostsPath, err)
	}

	original := string(content)
	updated := original
	for _, workspaceID := range workspaceIDs {
		updated, err = replaceHostsBlock(updated, workspaceID, "")
		if err != nil {
			return err
		}
	}
	if updated == original {
		return nil
	}
	if err := os.WriteFile(hostsPath, []byte(updated), 0644); err != nil {
		return fmt.Errorf("failed to write hosts file %s: %w", hostsPath, err)
	}
	return nil
}

func ResetCtxData(records []WorkspaceRecord) error {
	roots := make(map[string]struct{}, len(records))
	for _, record := range records {
		roots[record.RootPath] = struct{}{}
	}

	for root := range roots {
		markerPath := filepath.Join(root, MarkerFile)
		if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove workspace marker %s: %w", markerPath, err)
		}
	}
	if err := RemoveAllShellConfigs(); err != nil {
		return err
	}
	return removeCtxHomeData()
}

func RemoveAllShellConfigs() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to locate home directory: %w", err)
	}
	for _, name := range []string{".zshrc", ".bashrc"} {
		path := filepath.Join(home, name)
		content, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		updated := string(content)
		changed := false
		for {
			var removed bool
			updated, removed = removeMarkedBlock(updated)
			if !removed {
				break
			}
			changed = true
		}
		if !changed {
			continue
		}
		if updated == "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("failed to remove empty %s: %w", path, err)
			}
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("failed to inspect %s: %w", path, err)
		}
		if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}
	return nil
}

func removeCtxHomeData() error {
	root, err := filepath.Abs(dataRoot())
	if err != nil {
		return fmt.Errorf("failed to resolve ctx home: %w", err)
	}
	home, _ := os.UserHomeDir()
	if root == filepath.Clean(string(filepath.Separator)) || (home != "" && root == filepath.Clean(home)) {
		return fmt.Errorf("refusing to remove unsafe ctx home: %s", root)
	}
	if err := os.RemoveAll(root); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove ctx home %s: %w", root, err)
	}
	return nil
}
