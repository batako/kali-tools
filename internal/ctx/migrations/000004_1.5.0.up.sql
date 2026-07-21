ALTER TABLE web_discoveries ADD COLUMN discovery_type TEXT NOT NULL DEFAULT 'path';
ALTER TABLE web_discoveries ADD COLUMN template_url TEXT;
ALTER TABLE web_discoveries ADD COLUMN parameter_name TEXT;
ALTER TABLE web_discoveries ADD COLUMN parameter_value TEXT;
ALTER TABLE web_discoveries ADD COLUMN fuzz_part TEXT;
ALTER TABLE web_discoveries ADD COLUMN word_count INTEGER;
ALTER TABLE web_discoveries ADD COLUMN line_count INTEGER;

ALTER TABLE command_logs ADD COLUMN parent_id INTEGER;
ALTER TABLE command_logs ADD COLUMN phase TEXT;
ALTER TABLE command_logs ADD COLUMN target TEXT;
ALTER TABLE command_logs ADD COLUMN sequence INTEGER;

CREATE INDEX idx_command_logs_parent_id ON command_logs(parent_id);

CREATE INDEX idx_web_discoveries_target_type ON web_discoveries(target_id, discovery_type, created_at DESC);
