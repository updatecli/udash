BEGIN;
CREATE TABLE IF NOT EXISTS pipelines(
   id SERIAL PRIMARY KEY,
   data JSON NOT NULL,
   created_at TIMESTAMP,
   updated_at TIMESTAMP
);
ALTER TABLE pipelines ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE pipelines ALTER COLUMN updated_at SET DEFAULT now();
COMMIT;