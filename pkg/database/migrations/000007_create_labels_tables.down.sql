DROP INDEX IF EXISTS idx_labels_key;
DROP INDEX IF EXISTS idx_labels_value;
ALTER TABLE IF EXISTS labels DROP CONSTRAINT IF EXISTS labels_key_value_unique;
DROP TABLE IF EXISTS labels;
