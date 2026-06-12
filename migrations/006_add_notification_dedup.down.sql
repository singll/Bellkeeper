DROP INDEX IF EXISTS idx_matrix_notifications_dedup_key;
ALTER TABLE matrix_notifications DROP COLUMN IF EXISTS severity;
ALTER TABLE matrix_notifications DROP COLUMN IF EXISTS dedup_key;
