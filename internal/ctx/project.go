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

or register named roots:

    ctx project root add ~/projects/thm
    ctx project root use thm

or use:

    ctx workspace init

to initialize the current directory.`

var ErrProjectRootUnset = errors.New(projectRootUnsetMessage)

type Project struct {
	ID   int64
	Name string
	Path string
}

type ProjectRoot struct {
	Name   string
	Path   string
	Active bool
}

func SetProjectRoot(rawPath string) (string, error) {
	root, err := expandPath(rawPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("failed to create project root %s: %w", root, err)
	}
	return SetConfigValue(ConfigKeyProjectRoot, root)
}

func GetProjectRoot() (string, error) {
	return GetConfigValue(ConfigKeyProjectRoot)
}

func AddProjectRoot(rawPath, name string) (ProjectRoot, error) {
	root, err := expandPath(rawPath)
	if err != nil {
		return ProjectRoot{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(filepath.Clean(root))
		if name == "." || name == string(filepath.Separator) {
			return ProjectRoot{}, errors.New("project root name cannot be derived from path; use --name")
		}
	}
	if err := validateProjectRootName(name); err != nil {
		return ProjectRoot{}, err
	}
	config, err := LoadConfig()
	if err != nil {
		return ProjectRoot{}, err
	}
	if _, exists := config.ProjectRoots[name]; exists {
		return ProjectRoot{}, fmt.Errorf("project root already exists: %s", name)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		return ProjectRoot{}, fmt.Errorf("failed to create project root %s: %w", root, err)
	}
	config.ProjectRoots[name] = root
	if config.ActiveProjectRoot == "" {
		config.ActiveProjectRoot = name
		config.ProjectRoot = root
	}
	if err := SaveConfig(config); err != nil {
		return ProjectRoot{}, err
	}
	return ProjectRoot{Name: name, Path: root, Active: config.ActiveProjectRoot == name}, nil
}

func UseProjectRoot(name string) (ProjectRoot, error) {
	name = strings.TrimSpace(name)
	if err := validateProjectRootName(name); err != nil {
		return ProjectRoot{}, err
	}
	config, err := LoadConfig()
	if err != nil {
		return ProjectRoot{}, err
	}
	root, ok := config.ProjectRoots[name]
	if !ok {
		return ProjectRoot{}, fmt.Errorf("project root not found: %s", name)
	}
	config.ActiveProjectRoot = name
	config.ProjectRoot = root
	if err := SaveConfig(config); err != nil {
		return ProjectRoot{}, err
	}
	return ProjectRoot{Name: name, Path: root, Active: true}, nil
}

func ListProjectRoots() ([]ProjectRoot, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(config.ProjectRoots))
	for name := range config.ProjectRoots {
		names = append(names, name)
	}
	sort.Strings(names)
	roots := make([]ProjectRoot, 0, len(names))
	for _, name := range names {
		roots = append(roots, ProjectRoot{Name: name, Path: config.ProjectRoots[name], Active: name == config.ActiveProjectRoot})
	}
	return roots, nil
}

func RemoveProjectRoot(name string) (ProjectRoot, error) {
	name = strings.TrimSpace(name)
	if err := validateProjectRootName(name); err != nil {
		return ProjectRoot{}, err
	}
	config, err := LoadConfig()
	if err != nil {
		return ProjectRoot{}, err
	}
	root, ok := config.ProjectRoots[name]
	if !ok {
		return ProjectRoot{}, fmt.Errorf("project root not found: %s", name)
	}
	if name == config.ActiveProjectRoot {
		return ProjectRoot{}, fmt.Errorf("cannot remove active project root %s; switch roots first", name)
	}
	delete(config.ProjectRoots, name)
	if err := SaveConfig(config); err != nil {
		return ProjectRoot{}, err
	}
	return ProjectRoot{Name: name, Path: root}, nil
}

func validateProjectRootName(name string) error {
	if name == "" {
		return errors.New("project root name must not be empty")
	}
	for _, character := range name {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' {
			continue
		}
		return fmt.Errorf("invalid project root name: %s", name)
	}
	return nil
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
		markerUUID, err := readWorkspaceMarker(filepath.Join(path, MarkerFile))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("failed to inspect project %s: %w", path, err)
		}
		project := Project{Name: entry.Name(), Path: path}
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
	if _, err := readWorkspaceMarker(filepath.Join(projectPath, MarkerFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
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
	markerID, err := readWorkspaceMarker(filepath.Join(projectPath, MarkerFile))
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
