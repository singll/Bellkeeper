-- 001_init.down.sql
-- Rollback Bellkeeper Database Schema

DROP TABLE IF EXISTS dataset_mapping_tags;
DROP TABLE IF EXISTS rss_tags;
DROP TABLE IF EXISTS datasource_tags;
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS article_tags;
DROP TABLE IF EXISTS dataset_mappings;
DROP TABLE IF EXISTS webhook_history;
DROP TABLE IF EXISTS webhook_configs;
DROP TABLE IF EXISTS rss_feeds;
DROP TABLE IF EXISTS data_sources;
DROP TABLE IF EXISTS tags;
