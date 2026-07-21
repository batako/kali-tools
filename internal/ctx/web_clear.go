package ctx

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type WebDiscoveryDataSummary struct {
	Discoveries  int64
	WordlistRuns int64
	CachePresent bool
}

func InspectWebDiscoveryData(workspace *Workspace, target *Target) (WebDiscoveryDataSummary, error) {
	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return WebDiscoveryDataSummary{}, err
	}
	defer db.Close()

	var summary WebDiscoveryDataSummary
	err = db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM web_discoveries WHERE target_id = ?),
			(SELECT COUNT(*) FROM web_wordlist_runs WHERE target_id = ?)
	`, target.ID, target.ID).Scan(&summary.Discoveries, &summary.WordlistRuns)
	if err != nil {
		return WebDiscoveryDataSummary{}, fmt.Errorf("failed to inspect web discovery data: %w", err)
	}

	for _, path := range webDiscoveryCachePaths(workspace, target) {
		_, err = os.Lstat(path)
		switch {
		case err == nil:
			summary.CachePresent = true
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			return WebDiscoveryDataSummary{}, fmt.Errorf("failed to inspect web discovery cache %s: %w", path, err)
		}
	}
	return summary, nil
}

func ClearWebDiscoveryData(workspace *Workspace, target *Target) (WebDiscoveryDataSummary, error) {
	_, err := InspectWebDiscoveryData(workspace, target)
	if err != nil {
		return WebDiscoveryDataSummary{}, err
	}

	movedCaches, err := isolateWebDiscoveryCaches(webDiscoveryCachePaths(workspace, target))
	if err != nil {
		return WebDiscoveryDataSummary{}, err
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return WebDiscoveryDataSummary{}, restoreWebDiscoveryCaches(movedCaches, err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return WebDiscoveryDataSummary{}, restoreWebDiscoveryCaches(movedCaches, fmt.Errorf("failed to start web discovery clear transaction: %w", err))
	}

	actual := WebDiscoveryDataSummary{CachePresent: len(movedCaches) > 0}
	actual.Discoveries, err = deleteWebDiscoveryRows(tx, "web_discoveries", target.ID)
	if err == nil {
		actual.WordlistRuns, err = deleteWebDiscoveryRows(tx, "web_wordlist_runs", target.ID)
	}
	if err != nil {
		_ = tx.Rollback()
		return WebDiscoveryDataSummary{}, restoreWebDiscoveryCaches(movedCaches, err)
	}
	if err := tx.Commit(); err != nil {
		return WebDiscoveryDataSummary{}, restoreWebDiscoveryCaches(movedCaches, fmt.Errorf("failed to commit web discovery clear transaction: %w", err))
	}

	var removeErrors []error
	for _, cache := range movedCaches {
		if err := os.RemoveAll(cache.backupPath); err != nil {
			removeErrors = append(removeErrors, fmt.Errorf("%s: %w", cache.backupPath, err))
		}
	}
	if len(removeErrors) > 0 {
		return actual, fmt.Errorf("web discovery data was cleared but one or more isolated caches could not be removed: %w", errors.Join(removeErrors...))
	}
	return actual, nil
}

func webWordlistCachePath(workspace *Workspace, target *Target) string {
	return filepath.Join(workspace.DataPath, "web-wordlists", strconv.FormatInt(target.ID, 10))
}

func webDiscoveryCachePaths(workspace *Workspace, target *Target) []string {
	targetID := strconv.FormatInt(target.ID, 10)
	return []string{
		webWordlistCachePath(workspace, target),
		filepath.Join(workspace.DataPath, "xffuf-vhost", targetID),
		filepath.Join(workspace.DataPath, "xffuf-param", targetID),
	}
}

type isolatedWebDiscoveryCache struct {
	path       string
	backupPath string
}

func isolateWebDiscoveryCaches(paths []string) ([]isolatedWebDiscoveryCache, error) {
	suffix := ".clearing-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	moved := make([]isolatedWebDiscoveryCache, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, restoreWebDiscoveryCaches(moved, fmt.Errorf("failed to inspect web discovery cache %s: %w", path, err))
		}
		cache := isolatedWebDiscoveryCache{path: path, backupPath: path + suffix}
		if err := os.Rename(cache.path, cache.backupPath); err != nil {
			return nil, restoreWebDiscoveryCaches(moved, fmt.Errorf("failed to isolate web discovery cache %s: %w", path, err))
		}
		moved = append(moved, cache)
	}
	return moved, nil
}

func deleteWebDiscoveryRows(tx *sql.Tx, table string, targetID int64) (int64, error) {
	query := "DELETE FROM " + table + " WHERE target_id = ?"
	result, err := tx.Exec(query, targetID)
	if err != nil {
		return 0, fmt.Errorf("failed to clear %s: %w", table, err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to inspect cleared %s: %w", table, err)
	}
	return count, nil
}

func restoreWebDiscoveryCaches(moved []isolatedWebDiscoveryCache, cause error) error {
	var restoreErrors []error
	for i := len(moved) - 1; i >= 0; i-- {
		cache := moved[i]
		if err := os.Rename(cache.backupPath, cache.path); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("%s: %w", cache.path, err))
		}
	}
	if len(restoreErrors) > 0 {
		return fmt.Errorf("%v; one or more web discovery caches could not be restored: %w", cause, errors.Join(restoreErrors...))
	}
	return cause
}
