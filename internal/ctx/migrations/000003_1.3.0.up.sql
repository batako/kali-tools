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
	discovered_at TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
	FOREIGN KEY (command_log_id) REFERENCES command_logs(id) ON DELETE SET NULL
);

CREATE INDEX idx_web_discoveries_target_id ON web_discoveries(target_id, discovered_at DESC);
CREATE INDEX idx_web_discoveries_command_log_id ON web_discoveries(command_log_id);
