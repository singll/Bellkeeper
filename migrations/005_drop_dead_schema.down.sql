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

ALTER TABLE dataset_mappings ADD COLUMN IF NOT EXISTS parser_id VARCHAR(50) DEFAULT 'naive';
