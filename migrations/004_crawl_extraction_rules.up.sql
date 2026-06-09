CREATE TABLE IF NOT EXISTS crawl_extraction_rules (
  id BIGSERIAL PRIMARY KEY,
  domain VARCHAR(255) NOT NULL,
  match_pattern VARCHAR(500),
  strategy VARCHAR(30) NOT NULL,
  rsshub_route VARCHAR(500),
  css_title_selector VARCHAR(500),
  css_content_selector VARCHAR(500),
  css_remove_selectors TEXT,
  firecrawl_options JSONB,
  trafilatura_options JSONB,
  quality_min_chars BIGINT DEFAULT 200,
  version BIGINT DEFAULT 1,
  status VARCHAR(20) NOT NULL DEFAULT 'candidate',
  created_by VARCHAR(20) NOT NULL DEFAULT 'llm',
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_crawl_ext_rules_domain
  ON crawl_extraction_rules(domain)
  WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_crawl_ext_rules_status
  ON crawl_extraction_rules(status);
CREATE INDEX IF NOT EXISTS idx_crawl_ext_rules_deleted_at
  ON crawl_extraction_rules(deleted_at);

CREATE TABLE IF NOT EXISTS crawl_rule_trials (
  id BIGSERIAL PRIMARY KEY,
  rule_id BIGINT NOT NULL,
  sample_urls JSONB,
  attempt BIGINT DEFAULT 1,
  before_error VARCHAR(500),
  after_status VARCHAR(30),
  content_len BIGINT DEFAULT 0,
  quality_score DOUBLE PRECISION DEFAULT 0,
  diff_summary TEXT,
  created_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_crawl_rule_trials_rule_id
  ON crawl_rule_trials(rule_id);
