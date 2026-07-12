DROP INDEX IF EXISTS idx_notes_workspace_created_at;
DROP INDEX IF EXISTS idx_scan_runs_target_id;
DROP INDEX IF EXISTS idx_command_logs_workspace_started_at;
DROP INDEX IF EXISTS idx_credentials_target_id;
DROP INDEX IF EXISTS idx_services_target_id;
DROP INDEX IF EXISTS idx_hosts_target_id;
DROP INDEX IF EXISTS idx_targets_workspace_id;
DROP INDEX IF EXISTS idx_targets_one_primary;

DROP TABLE IF EXISTS notes;
DROP TABLE IF EXISTS scan_runs;
DROP TABLE IF EXISTS credentials;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS hosts;
DROP TABLE IF EXISTS command_logs;
DROP TABLE IF EXISTS targets;
DROP TABLE IF EXISTS workspaces;
