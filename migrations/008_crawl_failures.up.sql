CREATE TABLE IF NOT EXISTS crawl_failures (
    id SERIAL PRIMARY KEY,
    url VARCHAR(1000) NOT NULL,
    source_domain VARCHAR(255) NOT NULL,
    title VARCHAR(1000),
    source_id INTEGER,
    failure_count INTEGER NOT NULL DEFAULT 1,
    last_error_type VARCHAR(50),
    last_error_message VARCHAR(500),
    last_failed_at TIMESTAMP NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'recoverable',
    recovery_attempts INTEGER NOT NULL DEFAULT 0,
    request_overrides JSONB,
    analysis_result TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP
);

CREATE UNIQUE INDEX idx_crawl_failures_url ON crawl_failures (url) WHERE deleted_at IS NULL;
CREATE INDEX idx_crawl_failures_source_domain ON crawl_failures (source_domain);
CREATE INDEX idx_crawl_failures_status ON crawl_failures (status);
CREATE INDEX idx_crawl_failures_deleted_at ON crawl_failures (deleted_at);
