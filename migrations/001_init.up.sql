-- 001_init.up.sql
-- Bellkeeper Database Schema

-- Tags table
CREATE TABLE IF NOT EXISTS tags (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    color VARCHAR(20) DEFAULT '#409EFF',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Data sources table
CREATE TABLE IF NOT EXISTS data_sources (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    url VARCHAR(1000) NOT NULL,
    type VARCHAR(50) DEFAULT 'website',
    category VARCHAR(100),
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- RSS feeds table
CREATE TABLE IF NOT EXISTS rss_feeds (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    url VARCHAR(1000) UNIQUE NOT NULL,
    category VARCHAR(100),
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    last_fetched_at TIMESTAMP,
    fetch_interval_minutes INT DEFAULT 60,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Webhook configurations table
CREATE TABLE IF NOT EXISTS webhook_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(200) NOT NULL,
    url VARCHAR(1000) NOT NULL,
    method VARCHAR(10) DEFAULT 'POST',
    content_type VARCHAR(100) DEFAULT 'application/json',
    headers JSONB,
    body_template TEXT,
    timeout_seconds INT DEFAULT 30,
    description TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Webhook history table
CREATE TABLE IF NOT EXISTS webhook_history (
    id SERIAL PRIMARY KEY,
    webhook_id INT REFERENCES webhook_configs(id),
    request_url VARCHAR(1000),
    request_method VARCHAR(10),
    request_headers JSONB,
    request_body TEXT,
    status VARCHAR(20) DEFAULT 'pending',
    response_code INT,
    response_headers JSONB,
    response_body TEXT,
    duration_ms INT,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Dataset mappings table
CREATE TABLE IF NOT EXISTS dataset_mappings (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    display_name VARCHAR(200),
    dataset_id VARCHAR(100) NOT NULL,
    description TEXT,
    is_default BOOLEAN DEFAULT false,
    is_active BOOLEAN DEFAULT true,
    parser_id VARCHAR(50) DEFAULT 'naive',
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Article tags table
CREATE TABLE IF NOT EXISTS article_tags (
    id SERIAL PRIMARY KEY,
    document_id VARCHAR(100) NOT NULL,
    dataset_id VARCHAR(100) NOT NULL,
    tag_id INT REFERENCES tags(id),
    article_title VARCHAR(1000),
    article_url VARCHAR(2000),
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(document_id, tag_id)
);

-- Settings table
CREATE TABLE IF NOT EXISTS settings (
    id SERIAL PRIMARY KEY,
    key VARCHAR(255) UNIQUE NOT NULL,
    value TEXT,
    value_type VARCHAR(50) DEFAULT 'string',
    category VARCHAR(100),
    description TEXT,
    is_secret BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    deleted_at TIMESTAMP
);

-- Junction table: datasource_tags
CREATE TABLE IF NOT EXISTS datasource_tags (
    data_source_id INT REFERENCES data_sources(id) ON DELETE CASCADE,
    tag_id INT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (data_source_id, tag_id)
);

-- Junction table: rss_tags
CREATE TABLE IF NOT EXISTS rss_tags (
    rss_feed_id INT REFERENCES rss_feeds(id) ON DELETE CASCADE,
    tag_id INT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (rss_feed_id, tag_id)
);

-- Junction table: dataset_mapping_tags
CREATE TABLE IF NOT EXISTS dataset_mapping_tags (
    dataset_mapping_id INT REFERENCES dataset_mappings(id) ON DELETE CASCADE,
    tag_id INT REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (dataset_mapping_id, tag_id)
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_tags_name ON tags(name);
CREATE INDEX IF NOT EXISTS idx_tags_deleted_at ON tags(deleted_at);
CREATE INDEX IF NOT EXISTS idx_data_sources_category ON data_sources(category);
CREATE INDEX IF NOT EXISTS idx_data_sources_deleted_at ON data_sources(deleted_at);
CREATE INDEX IF NOT EXISTS idx_rss_feeds_category ON rss_feeds(category);
CREATE INDEX IF NOT EXISTS idx_rss_feeds_deleted_at ON rss_feeds(deleted_at);
CREATE INDEX IF NOT EXISTS idx_webhook_history_webhook_id ON webhook_history(webhook_id);
CREATE INDEX IF NOT EXISTS idx_webhook_history_created_at ON webhook_history(created_at);
CREATE INDEX IF NOT EXISTS idx_article_tags_document_id ON article_tags(document_id);
CREATE INDEX IF NOT EXISTS idx_article_tags_dataset_id ON article_tags(dataset_id);
CREATE INDEX IF NOT EXISTS idx_settings_key ON settings(key);
CREATE INDEX IF NOT EXISTS idx_settings_category ON settings(category);

-- Insert default settings
INSERT INTO settings (key, value, value_type, category, description, is_secret) VALUES
('ragflow.base_url', 'http://ragflow:9380', 'string', 'api', 'RagFlow Base URL', false),
('ragflow.api_key', '', 'string', 'api', 'RagFlow API Key', true),
('n8n.webhook_base_url', 'http://n8n:5678', 'string', 'api', 'n8n Webhook Base URL', false),
('feature.auto_parse', 'true', 'bool', 'feature', 'Auto parse uploaded documents', false),
('feature.url_dedup', 'true', 'bool', 'feature', 'Enable URL deduplication check', false),
('ui.theme', 'dark', 'string', 'ui', 'UI Theme', false)
ON CONFLICT (key) DO NOTHING;
