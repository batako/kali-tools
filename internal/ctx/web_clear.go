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

	_, err = os.Lstat(webWordlistCachePath(workspace, target))
	switch {
	case err == nil:
		summary.CachePresent = true
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return WebDiscoveryDataSummary{}, fmt.Errorf("failed to inspect web wordlist cache: %w", err)
	}
	return summary, nil
}

func ClearWebDiscoveryData(workspace *Workspace, target *Target) (WebDiscoveryDataSummary, error) {
	summary, err := InspectWebDiscoveryData(workspace, target)
	if err != nil {
		return WebDiscoveryDataSummary{}, err
	}

	cachePath := webWordlistCachePath(workspace, target)
	backupPath := cachePath + ".clearing-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	cacheMoved := false
	if summary.CachePresent {
		if err := os.Rename(cachePath, backupPath); err != nil {
			return WebDiscoveryDataSummary{}, fmt.Errorf("failed to isolate web wordlist cache: %w", err)
		}
		cacheMoved = true
	}

	db, err := openWorkspaceDatabase(workspace)
	if err != nil {
		return WebDiscoveryDataSummary{}, restoreWebWordlistCache(cacheMoved, backupPath, cachePath, err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return WebDiscoveryDataSummary{}, restoreWebWordlistCache(cacheMoved, backupPath, cachePath, fmt.Errorf("failed to start web discovery clear transaction: %w", err))
	}

	actual := WebDiscoveryDataSummary{CachePresent: summary.CachePresent}
	actual.Discoveries, err = deleteWebDiscoveryRows(tx, "web_discoveries", target.ID)
	if err == nil {
		actual.WordlistRuns, err = deleteWebDiscoveryRows(tx, "web_wordlist_runs", target.ID)
	}
	if err != nil {
		_ = tx.Rollback()
		return WebDiscoveryDataSummary{}, restoreWebWordlistCache(cacheMoved, backupPath, cachePath, err)
	}
	if err := tx.Commit(); err != nil {
		return WebDiscoveryDataSummary{}, restoreWebWordlistCache(cacheMoved, backupPath, cachePath, fmt.Errorf("failed to commit web discovery clear transaction: %w", err))
	}

	if cacheMoved {
		if err := os.RemoveAll(backupPath); err != nil {
			return actual, fmt.Errorf("web discovery data was cleared but the isolated cache could not be removed: %w", err)
		}
	}
	return actual, nil
}

func webWordlistCachePath(workspace *Workspace, target *Target) string {
	return filepath.Join(workspace.DataPath, "web-wordlists", strconv.FormatInt(target.ID, 10))
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

func restoreWebWordlistCache(moved bool, backupPath, cachePath string, cause error) error {
	if !moved {
		return cause
	}
	if err := os.Rename(backupPath, cachePath); err != nil {
		return fmt.Errorf("%v; failed to restore web wordlist cache: %w", cause, err)
	}
	return cause
}
