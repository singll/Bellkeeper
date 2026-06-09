-- Runtime tables introduced after the initial content-management schema.
-- Keep this migration explicit so production can disable GORM AutoMigrate and
-- still apply LLM, Matrix, LogCenter, CrawlQueue, and ActivityLog schema changes.

CREATE TABLE IF NOT EXISTS activity_logs (
  id BIGSERIAL PRIMARY KEY,
  module VARCHAR(50),
  action VARCHAR(100),
  status VARCHAR(20),
  summary VARCHAR(500),
  detail TEXT,
  ref_id VARCHAR(100),
  duration_ms BIGINT DEFAULT 0,
  created_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_activity_logs_module ON activity_logs(module);
CREATE INDEX IF NOT EXISTS idx_activity_logs_action ON activity_logs(action);
CREATE INDEX IF NOT EXISTS idx_activity_logs_status ON activity_logs(status);
CREATE INDEX IF NOT EXISTS idx_activity_logs_ref_id ON activity_logs(ref_id);
CREATE INDEX IF NOT EXISTS idx_activity_logs_created_at ON activity_logs(created_at);

CREATE TABLE IF NOT EXISTS llm_proxy_logs (
  id BIGSERIAL PRIMARY KEY,
  channel_name VARCHAR(100),
  model VARCHAR(100),
  request_path VARCHAR(500),
  status_code BIGINT DEFAULT 0,
  is_rate_limit BOOLEAN DEFAULT FALSE,
  retry_count BIGINT DEFAULT 0,
  duration_ms BIGINT DEFAULT 0,
  prompt_tokens BIGINT DEFAULT 0,
  comp_tokens BIGINT DEFAULT 0,
  cached_tokens BIGINT DEFAULT 0,
  cost_cents BIGINT DEFAULT 0,
  cost_micro_cents BIGINT DEFAULT 0,
  error_message TEXT,
  caller_id VARCHAR(100),
  created_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_llm_proxy_logs_channel_name ON llm_proxy_logs(channel_name);
CREATE INDEX IF NOT EXISTS idx_llm_proxy_logs_model ON llm_proxy_logs(model);
CREATE INDEX IF NOT EXISTS idx_llm_proxy_logs_is_rate_limit ON llm_proxy_logs(is_rate_limit);
CREATE INDEX IF NOT EXISTS idx_llm_proxy_logs_caller_id ON llm_proxy_logs(caller_id);
CREATE INDEX IF NOT EXISTS idx_llm_proxy_logs_created_at ON llm_proxy_logs(created_at);

CREATE TABLE IF NOT EXISTS llm_channels (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  base_url VARCHAR(500) NOT NULL,
  api_key_env VARCHAR(200),
  provider_type VARCHAR(50) DEFAULT 'openai',
  rpm BIGINT DEFAULT 500,
  rpd BIGINT DEFAULT 50000,
  priority BIGINT DEFAULT 1,
  is_free BOOLEAN DEFAULT FALSE,
  is_enabled BOOLEAN DEFAULT TRUE,
  models TEXT,
  balance_provider_type VARCHAR(50),
  balance_config_json TEXT,
  model_rpm_overrides TEXT,
  task_types TEXT,
  tier VARCHAR(20),
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_channels_name ON llm_channels(name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_llm_channels_deleted_at ON llm_channels(deleted_at);

CREATE TABLE IF NOT EXISTS llm_model_groups (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  description VARCHAR(500),
  strategy VARCHAR(50) DEFAULT 'priority-health',
  strategy_params JSONB,
  sticky_ttl_seconds BIGINT DEFAULT 600,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_model_groups_name ON llm_model_groups(name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_llm_model_groups_deleted_at ON llm_model_groups(deleted_at);

CREATE TABLE IF NOT EXISTS llm_model_group_members (
  id BIGSERIAL PRIMARY KEY,
  group_id BIGINT NOT NULL,
  channel_name VARCHAR(100) NOT NULL,
  model VARCHAR(200) NOT NULL,
  weight BIGINT DEFAULT 1,
  created_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_llm_model_group_members_group_id ON llm_model_group_members(group_id);
CREATE INDEX IF NOT EXISTS idx_llm_model_group_members_deleted_at ON llm_model_group_members(deleted_at);

CREATE TABLE IF NOT EXISTS llm_tokens (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(100) NOT NULL,
  key_hash VARCHAR(64) NOT NULL,
  key_prefix VARCHAR(16),
  caller_id VARCHAR(100) NOT NULL,
  allowed_models TEXT,
  allowed_groups TEXT,
  quota_requests_daily BIGINT DEFAULT 0,
  quota_tokens_daily BIGINT DEFAULT 0,
  quota_cost_monthly_cents BIGINT DEFAULT 0,
  expires_at TIMESTAMPTZ,
  enabled BOOLEAN DEFAULT TRUE,
  last_used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_tokens_key_hash ON llm_tokens(key_hash) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_tokens_caller_id ON llm_tokens(caller_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_llm_tokens_deleted_at ON llm_tokens(deleted_at);

CREATE TABLE IF NOT EXISTS llm_token_usage_daily (
  id BIGSERIAL PRIMARY KEY,
  token_id BIGINT NOT NULL,
  date TIMESTAMPTZ NOT NULL,
  requests BIGINT DEFAULT 0,
  prompt_tokens BIGINT DEFAULT 0,
  completion_tokens BIGINT DEFAULT 0,
  cached_tokens BIGINT DEFAULT 0,
  cost_cents BIGINT DEFAULT 0,
  cost_micro_cents BIGINT DEFAULT 0,
  error_count BIGINT DEFAULT 0,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_usage_token_date ON llm_token_usage_daily(token_id, date);

CREATE TABLE IF NOT EXISTS llm_model_pricing (
  id BIGSERIAL PRIMARY KEY,
  channel_name VARCHAR(100) NOT NULL,
  model VARCHAR(200) NOT NULL,
  input_price_per_1m_cents BIGINT DEFAULT 0,
  output_price_per_1m_cents BIGINT DEFAULT 0,
  cached_input_price_per_1m_cents BIGINT DEFAULT 0,
  currency VARCHAR(10) DEFAULT 'USD',
  effective_from TIMESTAMPTZ,
  notes TEXT,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_model_pricing_channel_model ON llm_model_pricing(channel_name, model);

CREATE TABLE IF NOT EXISTS llm_model_rate_limits (
  id BIGSERIAL PRIMARY KEY,
  channel_id BIGINT NOT NULL,
  model VARCHAR(200) NOT NULL,
  configured_rpm BIGINT DEFAULT 0,
  configured_rpd BIGINT DEFAULT 0,
  learned_rpm_safe BIGINT DEFAULT 0,
  learned_rpd_safe BIGINT DEFAULT 0,
  learned_concurrent_max BIGINT DEFAULT 0,
  reset_pattern VARCHAR(50),
  confidence_score DOUBLE PRECISION DEFAULT 0,
  last429_at TIMESTAMPTZ,
  last429_observed_rpm BIGINT DEFAULT 0,
  last_adjust_at TIMESTAMPTZ,
  locked BOOLEAN DEFAULT FALSE,
  adjustment_log JSONB,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_llm_model_rate_limits_channel_id ON llm_model_rate_limits(channel_id);
CREATE INDEX IF NOT EXISTS idx_llm_model_rate_limits_model ON llm_model_rate_limits(model);

CREATE TABLE IF NOT EXISTS llm_conversation_bindings (
  id BIGSERIAL PRIMARY KEY,
  conversation_id VARCHAR(255) NOT NULL,
  channel_id BIGINT NOT NULL,
  channel_name VARCHAR(100),
  model VARCHAR(200),
  task_type VARCHAR(64),
  first_seen_at TIMESTAMPTZ,
  last_seen_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  request_count BIGINT DEFAULT 0,
  total_tokens BIGINT DEFAULT 0,
  total_cost_cents BIGINT DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_conversation_bindings_conversation_id ON llm_conversation_bindings(conversation_id);
CREATE INDEX IF NOT EXISTS idx_llm_conversation_bindings_expires_at ON llm_conversation_bindings(expires_at);

CREATE TABLE IF NOT EXISTS llm_alert_events (
  id BIGSERIAL PRIMARY KEY,
  alert_type VARCHAR(50),
  severity VARCHAR(20),
  channel_id BIGINT DEFAULT 0,
  channel_name VARCHAR(100),
  message TEXT,
  dedup_key VARCHAR(255),
  flushed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_llm_alert_events_alert_type ON llm_alert_events(alert_type);
CREATE INDEX IF NOT EXISTS idx_llm_alert_events_severity ON llm_alert_events(severity);
CREATE INDEX IF NOT EXISTS idx_llm_alert_events_dedup_key ON llm_alert_events(dedup_key);
CREATE INDEX IF NOT EXISTS idx_llm_alert_events_created_at ON llm_alert_events(created_at);

CREATE TABLE IF NOT EXISTS llm_channel_credentials (
  id BIGSERIAL PRIMARY KEY,
  channel_id BIGINT NOT NULL,
  purpose VARCHAR(50) NOT NULL,
  source VARCHAR(50),
  env_var_name VARCHAR(200),
  is_preset BOOLEAN DEFAULT FALSE,
  label VARCHAR(100),
  priority BIGINT DEFAULT 0,
  provider_type VARCHAR(50),
  credential_json TEXT,
  status VARCHAR(50),
  error_message TEXT,
  last_refreshed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_llm_channel_credentials_channel_id ON llm_channel_credentials(channel_id);
CREATE INDEX IF NOT EXISTS idx_llm_channel_credentials_deleted_at ON llm_channel_credentials(deleted_at);

CREATE TABLE IF NOT EXISTS llm_channel_balance_snapshots (
  id BIGSERIAL PRIMARY KEY,
  channel_id BIGINT NOT NULL,
  channel_name VARCHAR(100),
  balance_usd DOUBLE PRECISION DEFAULT 0,
  currency VARCHAR(16),
  total_granted DOUBLE PRECISION DEFAULT 0,
  total_used DOUBLE PRECISION DEFAULT 0,
  balance_raw TEXT,
  latency_ms BIGINT DEFAULT 0,
  fetched_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_llm_channel_balance_snapshots_channel_id ON llm_channel_balance_snapshots(channel_id);
CREATE INDEX IF NOT EXISTS idx_llm_channel_balance_snapshots_fetched_at ON llm_channel_balance_snapshots(fetched_at);

CREATE TABLE IF NOT EXISTS llm_jobs (
  id BIGSERIAL PRIMARY KEY,
  task_type VARCHAR(64),
  caller_id VARCHAR(128),
  model VARCHAR(128),
  status VARCHAR(32) DEFAULT 'pending',
  priority BIGINT DEFAULT 0,
  idempotency_key VARCHAR(255),
  request_json JSONB,
  response_text TEXT,
  error_class VARCHAR(64),
  error_message TEXT,
  retry_count BIGINT DEFAULT 0,
  max_retries BIGINT DEFAULT 12,
  next_retry_at TIMESTAMPTZ,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_llm_jobs_task_type ON llm_jobs(task_type);
CREATE INDEX IF NOT EXISTS idx_llm_jobs_caller_id ON llm_jobs(caller_id);
CREATE INDEX IF NOT EXISTS idx_llm_jobs_model ON llm_jobs(model);
CREATE INDEX IF NOT EXISTS idx_llm_jobs_status ON llm_jobs(status);
CREATE INDEX IF NOT EXISTS idx_llm_jobs_priority ON llm_jobs(priority);
CREATE INDEX IF NOT EXISTS idx_llm_jobs_idempotency_key ON llm_jobs(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_llm_jobs_next_retry_at ON llm_jobs(next_retry_at);
CREATE INDEX IF NOT EXISTS idx_llm_jobs_deleted_at ON llm_jobs(deleted_at);

CREATE TABLE IF NOT EXISTS log_sources (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  source_type VARCHAR(32) NOT NULL,
  description VARCHAR(256),
  api_key VARCHAR(128),
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_log_sources_name ON log_sources(name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_log_sources_deleted_at ON log_sources(deleted_at);

CREATE TABLE IF NOT EXISTS log_entries (
  id BIGSERIAL PRIMARY KEY,
  source_id BIGINT,
  module VARCHAR(64),
  action VARCHAR(64),
  level VARCHAR(16),
  status VARCHAR(32),
  summary VARCHAR(500),
  detail JSONB,
  ref_id VARCHAR(128),
  duration_ms BIGINT DEFAULT 0,
  trace_id VARCHAR(64),
  created_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_log_entries_source_id ON log_entries(source_id);
CREATE INDEX IF NOT EXISTS idx_log_entries_module ON log_entries(module);
CREATE INDEX IF NOT EXISTS idx_log_entries_level ON log_entries(level);
CREATE INDEX IF NOT EXISTS idx_log_entries_status ON log_entries(status);
CREATE INDEX IF NOT EXISTS idx_log_entries_ref_id ON log_entries(ref_id);
CREATE INDEX IF NOT EXISTS idx_log_entries_trace_id ON log_entries(trace_id);
CREATE INDEX IF NOT EXISTS idx_log_entries_created_at ON log_entries(created_at);

CREATE TABLE IF NOT EXISTS log_alert_rules (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  condition JSONB NOT NULL,
  notify_channel VARCHAR(64),
  is_active BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_log_alert_rules_deleted_at ON log_alert_rules(deleted_at);

CREATE TABLE IF NOT EXISTS matrix_rooms (
  id BIGSERIAL PRIMARY KEY,
  room_id VARCHAR(255) NOT NULL,
  room_name VARCHAR(255),
  room_type VARCHAR(50) NOT NULL,
  is_active BOOLEAN DEFAULT TRUE,
  config JSONB,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_matrix_rooms_room_id ON matrix_rooms(room_id);
CREATE INDEX IF NOT EXISTS idx_matrix_rooms_room_type ON matrix_rooms(room_type);
CREATE INDEX IF NOT EXISTS idx_matrix_rooms_is_active ON matrix_rooms(is_active);

CREATE TABLE IF NOT EXISTS matrix_channels (
  id BIGSERIAL PRIMARY KEY,
  channel_name VARCHAR(100) NOT NULL,
  room_id VARCHAR(255) NOT NULL,
  is_active BOOLEAN DEFAULT TRUE,
  priority BIGINT DEFAULT 0,
  config JSONB,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_matrix_channels_channel_name ON matrix_channels(channel_name);
CREATE INDEX IF NOT EXISTS idx_matrix_channels_room_id ON matrix_channels(room_id);
CREATE INDEX IF NOT EXISTS idx_matrix_channels_is_active ON matrix_channels(is_active);

CREATE TABLE IF NOT EXISTS matrix_commands (
  id BIGSERIAL PRIMARY KEY,
  command_name VARCHAR(100) NOT NULL,
  handler_type VARCHAR(100) NOT NULL,
  handler_config JSONB,
  permission_level VARCHAR(50) DEFAULT 'user',
  room_scope VARCHAR(50) DEFAULT 'any',
  is_active BOOLEAN DEFAULT TRUE,
  description TEXT,
  usage_example TEXT,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_matrix_commands_command_name ON matrix_commands(command_name);
CREATE INDEX IF NOT EXISTS idx_matrix_commands_handler_type ON matrix_commands(handler_type);
CREATE INDEX IF NOT EXISTS idx_matrix_commands_is_active ON matrix_commands(is_active);

CREATE TABLE IF NOT EXISTS matrix_events (
  id BIGSERIAL PRIMARY KEY,
  event_id VARCHAR(255) NOT NULL,
  room_id VARCHAR(255) NOT NULL,
  sender VARCHAR(255) NOT NULL,
  event_type VARCHAR(100) NOT NULL,
  content JSONB,
  processing_status VARCHAR(50) DEFAULT 'pending',
  error_message TEXT,
  processed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_matrix_events_event_id ON matrix_events(event_id);
CREATE INDEX IF NOT EXISTS idx_matrix_events_room_id ON matrix_events(room_id);
CREATE INDEX IF NOT EXISTS idx_matrix_events_processing_status ON matrix_events(processing_status);
CREATE INDEX IF NOT EXISTS idx_matrix_events_created_at ON matrix_events(created_at);

CREATE TABLE IF NOT EXISTS matrix_notifications (
  id BIGSERIAL PRIMARY KEY,
  notification_id VARCHAR(100) NOT NULL,
  channel_name VARCHAR(100) NOT NULL,
  room_id VARCHAR(255),
  message_type VARCHAR(50) DEFAULT 'text',
  message_content TEXT NOT NULL,
  metadata JSONB,
  status VARCHAR(50) DEFAULT 'pending',
  retry_count BIGINT DEFAULT 0,
  last_error TEXT,
  sent_event_id VARCHAR(255),
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_matrix_notifications_notification_id ON matrix_notifications(notification_id);
CREATE INDEX IF NOT EXISTS idx_matrix_notifications_channel_name ON matrix_notifications(channel_name);
CREATE INDEX IF NOT EXISTS idx_matrix_notifications_status ON matrix_notifications(status);
CREATE INDEX IF NOT EXISTS idx_matrix_notifications_created_at ON matrix_notifications(created_at);

CREATE TABLE IF NOT EXISTS matrix_command_logs (
  id BIGSERIAL PRIMARY KEY,
  event_id VARCHAR(255) NOT NULL,
  room_id VARCHAR(255) NOT NULL,
  sender VARCHAR(255) NOT NULL,
  command_name VARCHAR(100) NOT NULL,
  command_args TEXT,
  handler_type VARCHAR(100),
  execution_status VARCHAR(50) DEFAULT 'pending',
  execution_time_ms BIGINT DEFAULT 0,
  error_message TEXT,
  response_event_id VARCHAR(255),
  created_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_matrix_command_logs_event_id ON matrix_command_logs(event_id);
CREATE INDEX IF NOT EXISTS idx_matrix_command_logs_room_id ON matrix_command_logs(room_id);
CREATE INDEX IF NOT EXISTS idx_matrix_command_logs_command_name ON matrix_command_logs(command_name);
CREATE INDEX IF NOT EXISTS idx_matrix_command_logs_execution_status ON matrix_command_logs(execution_status);
CREATE INDEX IF NOT EXISTS idx_matrix_command_logs_created_at ON matrix_command_logs(created_at);

CREATE TABLE IF NOT EXISTS matrix_sync_state (
  id BIGSERIAL PRIMARY KEY,
  bot_user_id VARCHAR(255) NOT NULL,
  next_batch VARCHAR(255),
  filter_id VARCHAR(100),
  last_sync_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_matrix_sync_state_bot_user_id ON matrix_sync_state(bot_user_id);

CREATE TABLE IF NOT EXISTS matrix_user_roles (
  id BIGSERIAL PRIMARY KEY,
  user_id VARCHAR(255) NOT NULL,
  room_id VARCHAR(255) NOT NULL,
  role VARCHAR(50) NOT NULL,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_matrix_user_roles_user_room ON matrix_user_roles(user_id, room_id);

CREATE TABLE IF NOT EXISTS crawl_sources (
  id BIGSERIAL PRIMARY KEY,
  name VARCHAR(200) NOT NULL,
  source_type VARCHAR(50) NOT NULL,
  url VARCHAR(1000) NOT NULL,
  category VARCHAR(100),
  tags JSONB,
  is_active BOOLEAN DEFAULT TRUE,
  health_score BIGINT DEFAULT 100,
  consecutive_failures BIGINT DEFAULT 0,
  last_failure_reason VARCHAR(500),
  is_paused BOOLEAN DEFAULT FALSE,
  paused_at TIMESTAMPTZ,
  max_concurrency BIGINT DEFAULT 3,
  total_fetched BIGINT DEFAULT 0,
  total_failed BIGINT DEFAULT 0,
  last_fetched_at TIMESTAMPTZ,
  fetch_interval BIGINT DEFAULT 60,
  metadata JSONB,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_crawl_sources_source_type ON crawl_sources(source_type);
CREATE INDEX IF NOT EXISTS idx_crawl_sources_category ON crawl_sources(category);
CREATE INDEX IF NOT EXISTS idx_crawl_sources_deleted_at ON crawl_sources(deleted_at);

CREATE TABLE IF NOT EXISTS crawl_jobs (
  id BIGSERIAL PRIMARY KEY,
  source_id BIGINT NOT NULL,
  url VARCHAR(1000) NOT NULL,
  priority BIGINT DEFAULT 0,
  retry_count BIGINT DEFAULT 0,
  max_retries BIGINT DEFAULT 4,
  status VARCHAR(20) NOT NULL,
  error_message VARCHAR(500),
  quality_score DOUBLE PRECISION DEFAULT 0,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  next_retry_at TIMESTAMPTZ,
  title VARCHAR(1000),
  error_type VARCHAR(50),
  channel_type VARCHAR(30),
  content_length BIGINT DEFAULT 0,
  extractor_used VARCHAR(50),
  source_domain VARCHAR(255),
  block_reason VARCHAR(500),
  metadata JSONB,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_crawl_jobs_source_id ON crawl_jobs(source_id);
CREATE INDEX IF NOT EXISTS idx_crawl_jobs_url ON crawl_jobs(url);
CREATE INDEX IF NOT EXISTS idx_crawl_jobs_status ON crawl_jobs(status);
CREATE INDEX IF NOT EXISTS idx_crawl_jobs_error_type ON crawl_jobs(error_type);
CREATE INDEX IF NOT EXISTS idx_crawl_jobs_channel_type ON crawl_jobs(channel_type);
CREATE INDEX IF NOT EXISTS idx_crawl_jobs_source_domain ON crawl_jobs(source_domain);
CREATE INDEX IF NOT EXISTS idx_crawl_jobs_deleted_at ON crawl_jobs(deleted_at);
