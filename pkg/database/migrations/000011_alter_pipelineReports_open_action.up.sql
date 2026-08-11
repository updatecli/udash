-- Updatecli reports a pipeline which had nothing to change as a success, even when the
-- change it would have made is already sitting in a pull request nobody merged. That state
-- is the one which needs a human, yet it is indistinguishable from a genuinely up to date
-- pipeline when looking at the result alone.
--
-- It is however recorded in the report payload: reports.Action.Link is serialized as
-- "actionUrl", is omitted when empty, and Updatecli only ever fills it from an open pull
-- request. So "$.Actions.*.actionUrl" existing is an exact, self clearing marker for
-- "a pull request is open right now", and it is already true of every report stored so far.
--
-- The expression must stay byte for byte the one in openActionSQLExpr, otherwise the queries
-- keep returning the right reports while silently falling back to a sequential scan.
--
-- An expression index is used rather than a denormalized column: migration 000010 exists
-- precisely because a denormalized column silently drifted from the payload, and a generated
-- column would rewrite the whole table. An index needs no backfill, cannot drift, and covers
-- every existing row as soon as it is built. The result and the range predicates are part of
-- it so that the reports search and the reports summary, which always filter on a time range
-- and group per result, keep their index only scan.
BEGIN;

CREATE INDEX IF NOT EXISTS idx_pipelinereports_updated_at_result_open_action
ON pipelineReports (
    updated_at,
    pipeline_result,
    (jsonb_path_exists(data, '$.Actions.*.actionUrl'))
);

COMMIT;
