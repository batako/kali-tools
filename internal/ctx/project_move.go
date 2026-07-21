package ctx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type ProjectRootMove struct {
	WorkspaceID   int64
	WorkspaceUUID string
	Name          string
	SourcePath    string
	TargetPath    string
}

type ProjectRootMovePlan struct {
	Source       ProjectRoot
	Target       ProjectRoot
	Projects     []ProjectRootMove
	SwitchActive bool
}

var renameProjectRootPath = os.Rename

func PlanProjectRootMove(sourceName, targetName string) (*ProjectRootMovePlan, error) {
	if err := validateProjectRootName(sourceName); err != nil {
		return nil, err
	}
	if err := validateProjectRootName(targetName); err != nil {
		return nil, err
	}
	if sourceName == targetName {
		return nil, errors.New("source and destination project roots must differ")
	}

	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	sourcePath, ok := config.ProjectRoots[sourceName]
	if !ok {
		return nil, fmt.Errorf("project root not found: %s", sourceName)
	}
	targetPath, ok := config.ProjectRoots[targetName]
	if !ok {
		return nil, fmt.Errorf("project root not found: %s", targetName)
	}
	sourcePath = filepath.Clean(sourcePath)
	targetPath = filepath.Clean(targetPath)
	if sourcePath == targetPath {
		return nil, errors.New("source and destination project roots resolve to the same path")
	}
	if err := requireDirectory(sourcePath, "source project root"); err != nil {
		return nil, err
	}
	if err := requireDirectory(targetPath, "destination project root"); err != nil {
		return nil, err
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return nil, err
	}
	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}
	if os.SameFile(sourceInfo, targetInfo) {
		return nil, errors.New("source and destination project roots resolve to the same directory")
	}
	nested, err := pathWithin(sourcePath, targetPath)
	if err != nil {
		return nil, err
	}
	if nested {
		return nil, errors.New("destination project root must not be inside source project root")
	}
	nested, err = pathWithin(targetPath, sourcePath)
	if err != nil {
		return nil, err
	}
	if nested {
		return nil, errors.New("source project root must not be inside destination project root")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve current directory: %w", err)
	}
	inside, err := pathWithin(sourcePath, cwd)
	if err != nil {
		return nil, err
	}
	if inside {
		return nil, fmt.Errorf("cannot move project root while current directory is inside it: %s", sourcePath)
	}

	records, err := ListWorkspaceRecords()
	if err != nil {
		return nil, err
	}
	recordsByPath := make(map[string]WorkspaceRecord, len(records))
	sourceRecords := make(map[string]WorkspaceRecord)
	for _, record := range records {
		cleanPath := filepath.Clean(record.RootPath)
		recordsByPath[cleanPath] = record
		if filepath.Dir(cleanPath) == sourcePath {
			sourceRecords[cleanPath] = record
		}
	}

	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to list source project root %s: %w", sourcePath, err)
	}
	projects := make([]ProjectRootMove, 0, len(entries))
	validatedSourceRecords := make(map[string]struct{}, len(sourceRecords))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectPath := filepath.Join(sourcePath, entry.Name())
		markerUUID, err := readWorkspaceMarker(filepath.Join(projectPath, MarkerFile))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("failed to inspect project %s: %w", projectPath, err)
		}
		record, ok := recordsByPath[filepath.Clean(projectPath)]
		if !ok {
			return nil, fmt.Errorf("workspace database does not contain project: %s", projectPath)
		}
		if markerUUID != record.UUID {
			return nil, fmt.Errorf("workspace marker does not match database for project: %s", projectPath)
		}
		validatedSourceRecords[filepath.Clean(projectPath)] = struct{}{}

		destination := filepath.Join(targetPath, entry.Name())
		if _, err := os.Lstat(destination); err == nil {
			return nil, fmt.Errorf("destination already exists: %s", destination)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to inspect destination %s: %w", destination, err)
		}
		if _, exists := recordsByPath[filepath.Clean(destination)]; exists {
			return nil, fmt.Errorf("workspace database already contains destination: %s", destination)
		}
		projects = append(projects, ProjectRootMove{
			WorkspaceID:   record.ID,
			WorkspaceUUID: record.UUID,
			Name:          entry.Name(),
			SourcePath:    projectPath,
			TargetPath:    destination,
		})
	}
	for path := range sourceRecords {
		if _, ok := validatedSourceRecords[path]; !ok {
			return nil, fmt.Errorf("ctx project is missing or has no workspace marker: %s", path)
		}
	}
	if len(projects) == 0 {
		return nil, fmt.Errorf("no ctx projects under project root: %s", sourceName)
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })

	return &ProjectRootMovePlan{
		Source:       ProjectRoot{Name: sourceName, Path: sourcePath, Active: config.ActiveProjectRoot == sourceName},
		Target:       ProjectRoot{Name: targetName, Path: targetPath, Active: config.ActiveProjectRoot == targetName},
		Projects:     projects,
		SwitchActive: config.ActiveProjectRoot == sourceName,
	}, nil
}

func MoveProjectRootProjects(plan *ProjectRootMovePlan) error {
	if plan == nil || len(plan.Projects) == 0 {
		return errors.New("project root move plan is empty")
	}
	config, err := LoadConfig()
	if err != nil {
		return err
	}
	if filepath.Clean(config.ProjectRoots[plan.Source.Name]) != plan.Source.Path || filepath.Clean(config.ProjectRoots[plan.Target.Name]) != plan.Target.Path {
		return errors.New("project root configuration changed after move planning")
	}
	if (config.ActiveProjectRoot == plan.Source.Name) != plan.SwitchActive {
		return errors.New("active project root changed after move planning")
	}

	db, err := openDatabase(filepath.Join(dataRoot(), "db.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()
	if err := createSchema(db); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin project root move: %w", err)
	}
	defer tx.Rollback()

	moved := make([]ProjectRootMove, 0, len(plan.Projects))
	rollback := func(primary error) error {
		if rollbackErr := rollbackProjectRootMoves(moved); rollbackErr != nil {
			return fmt.Errorf("%w; filesystem rollback failed: %v", primary, rollbackErr)
		}
		return primary
	}

	for _, project := range plan.Projects {
		if _, err := os.Lstat(project.TargetPath); err == nil {
			return rollback(fmt.Errorf("destination appeared after move planning: %s", project.TargetPath))
		} else if !errors.Is(err, os.ErrNotExist) {
			return rollback(fmt.Errorf("failed to inspect destination %s: %w", project.TargetPath, err))
		}
		if err := renameProjectRootPath(project.SourcePath, project.TargetPath); err != nil {
			return rollback(fmt.Errorf("failed to move project %s from %s to %s: %w", project.Name, project.SourcePath, project.TargetPath, err))
		}
		moved = append(moved, project)
		result, err := tx.Exec(`
			UPDATE workspaces
			SET path = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND name = ? AND path = ?
		`, project.TargetPath, project.WorkspaceID, project.WorkspaceUUID, project.SourcePath)
		if err != nil {
			return rollback(fmt.Errorf("failed to update workspace path for %s: %w", project.Name, err))
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return rollback(fmt.Errorf("failed to verify workspace path for %s: %w", project.Name, err))
		}
		if affected != 1 {
			return rollback(fmt.Errorf("workspace changed after move planning: %s", project.Name))
		}
	}

	if plan.SwitchActive {
		if _, err := UseProjectRoot(plan.Target.Name); err != nil {
			return rollback(fmt.Errorf("failed to switch active project root: %w", err))
		}
	}
	if err := tx.Commit(); err != nil {
		if plan.SwitchActive {
			_, _ = UseProjectRoot(plan.Source.Name)
		}
		return rollback(fmt.Errorf("failed to commit project root move: %w", err))
	}
	return nil
}

func rollbackProjectRootMoves(projects []ProjectRootMove) error {
	var rollbackErr error
	for index := len(projects) - 1; index >= 0; index-- {
		project := projects[index]
		if err := renameProjectRootPath(project.TargetPath, project.SourcePath); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("%s: %w", project.Name, err))
		}
	}
	return rollbackErr
}

func requireDirectory(path, label string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to inspect %s %s: %w", label, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %s", label, path)
	}
	return nil
}

func pathWithin(root, path string) (bool, error) {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false, fmt.Errorf("failed to compare paths %s and %s: %w", root, path, err)
	}
	return relative == "." || relative != ".." && !filepath.IsAbs(relative) && !startsWithParent(relative), nil
}

func startsWithParent(path string) bool {
	return len(path) > 3 && path[:3] == ".."+string(filepath.Separator)
}
