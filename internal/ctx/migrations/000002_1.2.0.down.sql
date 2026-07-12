CREATE TABLE workspaces_old (
	id TEXT PRIMARY KEY,
	root_path TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (root_path)
);

INSERT INTO workspaces_old (id, root_path, created_at, updated_at)
SELECT name, path, created_at, updated_at
FROM workspaces
ORDER BY id;

CREATE TABLE targets_old (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id TEXT NOT NULL,
	name TEXT NOT NULL,
	ip TEXT NOT NULL,
	os_name TEXT,
	os_accuracy INTEGER,
	os_source TEXT,
	is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces_old(id) ON DELETE CASCADE,
	UNIQUE (workspace_id, name)
);

INSERT INTO targets_old (
	id, workspace_id, name, ip, os_name, os_accuracy, os_source,
	is_primary, created_at, updated_at
)
SELECT t.id, w.name, t.name, t.ip, t.os_name, t.os_accuracy, t.os_source,
       t.is_primary, t.created_at, t.updated_at
FROM targets AS t
JOIN workspaces AS w ON w.id = t.workspace_id;

CREATE TABLE command_logs_old (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id TEXT NOT NULL,
	command TEXT NOT NULL,
	expanded_command TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'interrupted')),
	exit_code INTEGER,
	stdout TEXT,
	stderr TEXT,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces_old(id) ON DELETE CASCADE
);

INSERT INTO command_logs_old (
	id, workspace_id, command, expanded_command, status, exit_code,
	stdout, stderr, started_at, ended_at, created_at, updated_at
)
SELECT c.id, w.name, c.command, c.expanded_command, c.status, c.exit_code,
       c.stdout, c.stderr, c.started_at, c.ended_at, c.created_at, c.updated_at
FROM command_logs AS c
JOIN workspaces AS w ON w.id = c.workspace_id;

CREATE TABLE hosts_old (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	hostname TEXT NOT NULL,
	source TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets_old(id) ON DELETE CASCADE,
	UNIQUE (target_id, hostname)
);

INSERT INTO hosts_old (id, target_id, hostname, source, created_at, updated_at)
SELECT id, target_id, hostname, source, created_at, updated_at
FROM hosts;

CREATE TABLE services_old (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
	protocol TEXT NOT NULL,
	state TEXT,
	reason TEXT,
	service_name TEXT,
	product TEXT,
	version TEXT,
	extrainfo TEXT,
	tunnel TEXT,
	cpe TEXT,
	last_seen TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets_old(id) ON DELETE CASCADE,
	UNIQUE (target_id, port, protocol)
);

INSERT INTO services_old (
	id, target_id, port, protocol, state, reason, service_name,
	product, version, extrainfo, tunnel, cpe, last_seen, created_at, updated_at
)
SELECT id, target_id, port, protocol, state, reason, service_name,
       product, version, extrainfo, tunnel, cpe, last_seen, created_at, updated_at
FROM services;

CREATE TABLE credentials_old (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	scope TEXT NOT NULL,
	username TEXT NOT NULL,
	password TEXT,
	verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0, 1)),
	evidence_log_id INTEGER,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets_old(id) ON DELETE CASCADE,
	FOREIGN KEY (evidence_log_id) REFERENCES command_logs_old(id) ON DELETE SET NULL,
	UNIQUE (target_id, scope, username)
);

INSERT INTO credentials_old (
	id, target_id, scope, username, password, verified,
	evidence_log_id, created_at, updated_at
)
SELECT id, target_id, scope, username, password, verified,
       evidence_log_id, created_at, updated_at
FROM credentials;

CREATE TABLE scan_runs_old (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	target_ip TEXT NOT NULL,
	ports TEXT NOT NULL,
	command_log_id INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets_old(id) ON DELETE CASCADE,
	FOREIGN KEY (command_log_id) REFERENCES command_logs_old(id) ON DELETE CASCADE,
	UNIQUE (target_id, target_ip, ports)
);

INSERT INTO scan_runs_old (
	id, target_id, target_ip, ports, command_log_id, created_at, updated_at
)
SELECT id, target_id, target_ip, ports, command_log_id, created_at, updated_at
FROM scan_runs;

CREATE TABLE notes_old (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id TEXT NOT NULL,
	body TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces_old(id) ON DELETE CASCADE
);

INSERT INTO notes_old (id, workspace_id, body, created_at, updated_at)
SELECT n.id, w.name, n.body, n.created_at, n.updated_at
FROM notes AS n
JOIN workspaces AS w ON w.id = n.workspace_id;

DROP TABLE notes;
DROP TABLE scan_runs;
DROP TABLE credentials;
DROP TABLE services;
DROP TABLE hosts;
DROP TABLE command_logs;
DROP TABLE targets;
DROP TABLE workspaces;

ALTER TABLE workspaces_old RENAME TO workspaces;
ALTER TABLE targets_old RENAME TO targets;
ALTER TABLE command_logs_old RENAME TO command_logs;
ALTER TABLE hosts_old RENAME TO hosts;
ALTER TABLE services_old RENAME TO services;
ALTER TABLE credentials_old RENAME TO credentials;
ALTER TABLE scan_runs_old RENAME TO scan_runs;
ALTER TABLE notes_old RENAME TO notes;

CREATE UNIQUE INDEX idx_targets_one_primary ON targets(workspace_id) WHERE is_primary = 1;
CREATE INDEX idx_targets_workspace_id ON targets(workspace_id);
CREATE INDEX idx_hosts_target_id ON hosts(target_id);
CREATE INDEX idx_services_target_id ON services(target_id);
CREATE INDEX idx_credentials_target_id ON credentials(target_id);
CREATE INDEX idx_command_logs_workspace_started_at ON command_logs(workspace_id, started_at DESC);
CREATE INDEX idx_scan_runs_target_id ON scan_runs(target_id);
CREATE INDEX idx_notes_workspace_created_at ON notes(workspace_id, created_at DESC);
