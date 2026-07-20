ALTER TABLE web_discoveries ADD COLUMN discovery_type TEXT NOT NULL DEFAULT 'path';
ALTER TABLE web_discoveries ADD COLUMN template_url TEXT;
ALTER TABLE web_discoveries ADD COLUMN parameter_name TEXT;
ALTER TABLE web_discoveries ADD COLUMN parameter_value TEXT;
ALTER TABLE web_discoveries ADD COLUMN fuzz_part TEXT;
ALTER TABLE web_discoveries ADD COLUMN word_count INTEGER;
ALTER TABLE web_discoveries ADD COLUMN line_count INTEGER;

CREATE INDEX idx_web_discoveries_target_type ON web_discoveries(target_id, discovery_type, created_at DESC);
