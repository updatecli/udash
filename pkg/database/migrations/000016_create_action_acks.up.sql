BEGIN;

-- An acknowledged action is hidden from the open actions list, for every viewer, until
-- `until` passes or the worst result of its pipelines no longer equals worst_result.
--
-- Actions are identified by url, as in the open actions search.
CREATE TABLE IF NOT EXISTS action_acks (
  url             text PRIMARY KEY,
  -- until and created_at hold the UTC wall clock, like pipelineReports.updated_at.
  until           timestamp NOT NULL,
  worst_result    text NOT NULL,
  acknowledged_by text NOT NULL DEFAULT '',
  created_at      timestamp NOT NULL
);

COMMIT;
