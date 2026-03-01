BEGIN;

-- Drop GIN indexes on config JSONB columns

DROP INDEX IF EXISTS idx_config_sources_config_gin;
DROP INDEX IF EXISTS idx_config_conditions_config_gin;
DROP INDEX IF EXISTS idx_config_targets_config_gin;

COMMIT;
