ALTER TABLE llm_model_group_members DROP COLUMN IF EXISTS max_output_tokens;
ALTER TABLE llm_model_group_members DROP COLUMN IF EXISTS max_context_tokens;

ALTER TABLE llm_channels DROP COLUMN IF EXISTS quota_window_seconds;
ALTER TABLE llm_channels DROP COLUMN IF EXISTS vendor;
