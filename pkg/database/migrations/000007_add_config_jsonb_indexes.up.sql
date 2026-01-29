BEGIN;

-- Add GIN indexes on config JSONB columns for faster queries
-- Using jsonb_path_ops for optimal performance on containment queries (config @> ?)
-- These indexes support:
-- - config @> ? (containment queries) - optimized with jsonb_path_ops
-- - config->'spec' (field extraction)
-- - Any JSONB operations on the config column

CREATE INDEX IF NOT EXISTS idx_config_sources_config_gin
ON config_sources USING gin (config jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_config_conditions_config_gin
ON config_conditions USING gin (config jsonb_path_ops);

CREATE INDEX IF NOT EXISTS idx_config_targets_config_gin
ON config_targets USING gin (config jsonb_path_ops);

COMMIT;
