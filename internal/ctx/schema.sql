CREATE TABLE workspaces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	path TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (path)
);

CREATE TABLE targets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL,
	name TEXT NOT NULL,
	ip TEXT NOT NULL,
	os_name TEXT,
	os_accuracy INTEGER,
	os_source TEXT,
	is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
	UNIQUE (workspace_id, name)
);

CREATE TABLE command_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL,
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
	parent_id INTEGER,
	phase TEXT,
	target TEXT,
	sequence INTEGER,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE TABLE hosts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	hostname TEXT NOT NULL,
	source TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
	UNIQUE (target_id, hostname)
);

CREATE TABLE services (
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
	FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
	UNIQUE (target_id, port, protocol)
);

CREATE TABLE credentials (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	scope TEXT NOT NULL,
	username TEXT NOT NULL,
	password TEXT,
	verified INTEGER NOT NULL DEFAULT 0 CHECK (verified IN (0, 1)),
	evidence_log_id INTEGER,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
	FOREIGN KEY (evidence_log_id) REFERENCES command_logs(id) ON DELETE SET NULL,
	UNIQUE (target_id, scope, username)
);

CREATE TABLE scan_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	target_ip TEXT NOT NULL,
	ports TEXT NOT NULL,
	command_log_id INTEGER NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
	FOREIGN KEY (command_log_id) REFERENCES command_logs(id) ON DELETE CASCADE,
	UNIQUE (target_id, target_ip, ports)
);

CREATE TABLE notes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_id INTEGER NOT NULL,
	body TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
);

CREATE TABLE web_discoveries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	url TEXT NOT NULL,
	path TEXT NOT NULL,
	status_code INTEGER NOT NULL,
	content_length INTEGER,
	redirect_url TEXT,
	source_tool TEXT NOT NULL,
	wordlist TEXT NOT NULL,
	command_log_id INTEGER,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	discovery_type TEXT NOT NULL DEFAULT 'path',
	template_url TEXT,
	parameter_name TEXT,
	parameter_value TEXT,
	fuzz_part TEXT,
	word_count INTEGER,
	line_count INTEGER,
	FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
	FOREIGN KEY (command_log_id) REFERENCES command_logs(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_targets_one_primary ON targets(workspace_id) WHERE is_primary = 1;
CREATE INDEX idx_targets_workspace_id ON targets(workspace_id);
CREATE INDEX idx_hosts_target_id ON hosts(target_id);
CREATE INDEX idx_services_target_id ON services(target_id);
CREATE INDEX idx_credentials_target_id ON credentials(target_id);
CREATE INDEX idx_command_logs_workspace_started_at ON command_logs(workspace_id, started_at DESC);
CREATE INDEX idx_scan_runs_target_id ON scan_runs(target_id);
CREATE INDEX idx_notes_workspace_created_at ON notes(workspace_id, created_at DESC);
CREATE INDEX idx_web_discoveries_target_id ON web_discoveries(target_id, created_at DESC);
CREATE INDEX idx_web_discoveries_command_log_id ON web_discoveries(command_log_id);
CREATE INDEX idx_web_discoveries_target_type ON web_discoveries(target_id, discovery_type, created_at DESC);

CREATE TABLE web_wordlist_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER NOT NULL,
	url TEXT NOT NULL,
	provider TEXT NOT NULL,
	profile TEXT NOT NULL,
	search_signature TEXT NOT NULL,
	wordlist TEXT NOT NULL,
	status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'interrupted')),
	command_log_id INTEGER,
	started_at TEXT NOT NULL,
	ended_at TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
	FOREIGN KEY (command_log_id) REFERENCES command_logs(id) ON DELETE SET NULL,
	UNIQUE (target_id, url, profile, search_signature, wordlist)
);

CREATE INDEX idx_web_wordlist_runs_target_url ON web_wordlist_runs(target_id, url, id);
CREATE INDEX idx_command_logs_parent_id ON command_logs(parent_id);
