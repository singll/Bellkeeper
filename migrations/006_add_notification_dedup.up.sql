ALTER TABLE matrix_notifications ADD COLUMN IF NOT EXISTS dedup_key VARCHAR(255);
ALTER TABLE matrix_notifications ADD COLUMN IF NOT EXISTS severity VARCHAR(50) DEFAULT 'info';
CREATE INDEX IF NOT EXISTS idx_matrix_notifications_dedup_key ON matrix_notifications(dedup_key);
