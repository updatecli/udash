BEGIN;

DROP INDEX IF EXISTS idx_api_tokens_subject;
DROP TABLE IF EXISTS api_tokens;

COMMIT;
