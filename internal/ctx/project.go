package ctx

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	ID   int64
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
	records, err := ListWorkspaceRecords()
	if err != nil {
		return nil, err
	}
	recordsByPath := make(map[string]WorkspaceRecord, len(records))
	recordsByUUID := make(map[string]WorkspaceRecord, len(records))
	for _, record := range records {
		recordsByPath[filepath.Clean(record.RootPath)] = record
		recordsByUUID[record.UUID] = record
	}

	projects := make([]Project, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		markerID, markerUUID, err := readWorkspaceMarker(filepath.Join(path, MarkerFile))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("failed to inspect project %s: %w", path, err)
		}
		project := Project{ID: markerID, Name: entry.Name(), Path: path}
		if record, ok := recordsByPath[filepath.Clean(path)]; ok {
			project.ID = record.ID
		} else if record, ok := recordsByUUID[markerUUID]; ok {
			project.ID = record.ID
		}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].Name < projects[j].Name
	})
	return projects, nil
}

func ResolveProject(identifier string) (Project, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return Project{}, errors.New("project name or ID must not be empty")
	}
	if id, err := strconv.ParseInt(identifier, 10, 64); err == nil && id > 0 {
		projects, err := ListProjects()
		if err != nil {
			return Project{}, err
		}
		for _, project := range projects {
			if project.ID == id {
				return project, nil
			}
		}
	}

	root, err := requiredProjectRoot()
	if err != nil {
		return Project{}, err
	}
	projectPath, err := projectPath(root, identifier)
	if err != nil {
		return Project{}, err
	}
	info, err := os.Stat(projectPath)
	if errors.Is(err, os.ErrNotExist) {
		return Project{}, fmt.Errorf("project not found: %s", identifier)
	}
	if err != nil {
		return Project{}, fmt.Errorf("failed to inspect project %s: %w", projectPath, err)
	}
	if !info.IsDir() {
		return Project{}, fmt.Errorf("project is not a directory: %s", projectPath)
	}

	project := Project{Name: filepath.Base(projectPath), Path: projectPath}
	if markerID, _, err := readWorkspaceMarker(filepath.Join(projectPath, MarkerFile)); err == nil {
		project.ID = markerID
	} else if !errors.Is(err, os.ErrNotExist) {
		return Project{}, err
	}
	if record, err := GetWorkspaceRecordByRoot(projectPath); err == nil && record != nil {
		project.ID = record.ID
	} else if err != nil {
		return Project{}, err
	}
	return project, nil
}

func RemoveProject(identifier string) (string, error) {
	project, err := ResolveProject(identifier)
	if err != nil {
		return "", err
	}
	return removeResolvedProject(project)
}

func removeResolvedProject(project Project) (string, error) {
	if err := removeProjectWorkspaceData(project.Path); err != nil {
		return "", err
	}
	if err := os.RemoveAll(project.Path); err != nil {
		return "", fmt.Errorf("failed to remove project %s: %w", project.Path, err)
	}
	return project.Path, nil
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
		if record.UUID == markerID && filepath.Clean(record.RootPath) == filepath.Clean(projectPath) {
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
