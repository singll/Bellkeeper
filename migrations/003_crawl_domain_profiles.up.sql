CREATE TABLE IF NOT EXISTS crawl_domain_profiles (
  id BIGSERIAL PRIMARY KEY,
  domain VARCHAR(255) NOT NULL,
  default_delay_seconds BIGINT DEFAULT 60,
  max_concurrency BIGINT DEFAULT 1,
  success_rate_24h DOUBLE PRECISION DEFAULT 0,
  block_rate_24h DOUBLE PRECISION DEFAULT 0,
  last_status VARCHAR(50),
  next_allowed_at TIMESTAMPTZ,
  robots_checked_at TIMESTAMPTZ,
  notes TEXT,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_crawl_domain_profiles_domain
  ON crawl_domain_profiles(domain)
  WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_crawl_domain_profiles_next_allowed_at
  ON crawl_domain_profiles(next_allowed_at);
CREATE INDEX IF NOT EXISTS idx_crawl_domain_profiles_deleted_at
  ON crawl_domain_profiles(deleted_at);
