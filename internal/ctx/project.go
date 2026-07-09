package ctx

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const projectRootUnsetMessage = `No projects root configured.

Run:

    ctx project root ~/projects

or use:

    ctx workspace init

to initialize the current directory.`

var ErrProjectRootUnset = errors.New(projectRootUnsetMessage)

type Project struct {
	Name string
	Path string
}

func SetProjectRoot(rawPath string) (string, error) {
	root, err := expandPath(rawPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("failed to create project root %s: %w", root, err)
	}
	config, err := LoadConfig()
	if err != nil {
		return "", err
	}
	config.ProjectRoot = root
	if err := SaveConfig(config); err != nil {
		return "", err
	}
	return root, nil
}

func GetProjectRoot() (string, error) {
	config, err := LoadConfig()
	if err != nil {
		return "", err
	}
	return config.ProjectRoot, nil
}

func CreateProject(name string) (string, error) {
	root, err := requiredProjectRoot()
	if err != nil {
		return "", err
	}
	projectPath, err := projectPath(root, name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create project %s: %w", projectPath, err)
	}
	if _, err := InitWorkspace(projectPath); err != nil {
		return "", err
	}
	return projectPath, nil
}

func ListProjects() ([]Project, error) {
	root, err := requiredProjectRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("failed to list project root %s: %w", root, err)
	}
	projects := make([]Project, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(path, MarkerFile)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("failed to inspect project %s: %w", path, err)
		}
		projects = append(projects, Project{Name: entry.Name(), Path: path})
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})
	return projects, nil
}

func RemoveProject(name string) (string, error) {
	root, err := requiredProjectRoot()
	if err != nil {
		return "", err
	}
	projectPath, err := projectPath(root, name)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(projectPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("project not found: %s", name)
	}
	if err != nil {
		return "", fmt.Errorf("failed to inspect project %s: %w", projectPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project is not a directory: %s", projectPath)
	}

	if err := removeProjectWorkspaceData(projectPath); err != nil {
		return "", err
	}
	if err := os.RemoveAll(projectPath); err != nil {
		return "", fmt.Errorf("failed to remove project %s: %w", projectPath, err)
	}
	return projectPath, nil
}

func requiredProjectRoot() (string, error) {
	root, err := GetProjectRoot()
	if err != nil {
		return "", err
	}
	if root == "" {
		return "", ErrProjectRootUnset
	}
	return root, nil
}

func projectPath(root, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("project name must not be empty")
	}
	if filepath.Base(name) != name || name == "." || name == ".." {
		return "", fmt.Errorf("invalid project name: %s", name)
	}
	path := filepath.Clean(filepath.Join(root, name))
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve project path: %w", err)
	}
	if relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." || filepath.IsAbs(relative) {
		return "", fmt.Errorf("refusing to use project path outside root: %s", name)
	}
	return path, nil
}

func removeProjectWorkspaceData(projectPath string) error {
	markerID, err := readWorkspaceID(filepath.Join(projectPath, MarkerFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	records, err := ListWorkspaceRecords()
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.ID == markerID && filepath.Clean(record.RootPath) == filepath.Clean(projectPath) {
			return RemoveWorkspace(record)
		}
	}
	return nil
}

func confirmProjectRemoval(stdout io.Writer, scanner *bufio.Scanner, name, path string) (bool, error) {
	if _, err := fmt.Fprintf(stdout, "Remove project %s (%s)? [y/N] ", name, path); err != nil {
		return false, err
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("failed to read confirmation: %w", err)
		}
		if _, err := fmt.Fprintln(stdout, "\ncancelled"); err != nil {
			return false, err
		}
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	if answer != "y" && answer != "yes" {
		if _, err := fmt.Fprintln(stdout, "cancelled"); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}
