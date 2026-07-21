DROP INDEX idx_web_discoveries_target_type;

ALTER TABLE web_discoveries DROP COLUMN line_count;
ALTER TABLE web_discoveries DROP COLUMN word_count;
ALTER TABLE web_discoveries DROP COLUMN fuzz_part;
ALTER TABLE web_discoveries DROP COLUMN parameter_value;
ALTER TABLE web_discoveries DROP COLUMN parameter_name;
ALTER TABLE web_discoveries DROP COLUMN template_url;
ALTER TABLE web_discoveries DROP COLUMN discovery_type;

DROP INDEX idx_command_logs_parent_id;
ALTER TABLE command_logs DROP COLUMN sequence;
ALTER TABLE command_logs DROP COLUMN target;
ALTER TABLE command_logs DROP COLUMN phase;
ALTER TABLE command_logs DROP COLUMN parent_id;
