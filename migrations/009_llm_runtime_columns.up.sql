-- Runtime columns added after 002: rolling quota windows, vendor identity, and
-- per-member context/output limits. IF NOT EXISTS keeps this safe for databases
-- that already got the columns via GORM AutoMigrate.

ALTER TABLE llm_channels ADD COLUMN IF NOT EXISTS vendor VARCHAR(50);
ALTER TABLE llm_channels ADD COLUMN IF NOT EXISTS quota_window_seconds BIGINT DEFAULT 0;

ALTER TABLE llm_model_group_members ADD COLUMN IF NOT EXISTS max_context_tokens BIGINT DEFAULT 0;
ALTER TABLE llm_model_group_members ADD COLUMN IF NOT EXISTS max_output_tokens BIGINT DEFAULT 0;
