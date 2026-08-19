DROP INDEX IF EXISTS idx_test_regressions_force_closed;
DROP INDEX IF EXISTS idx_test_regressions_force_closed_by_triage_id;

ALTER TABLE test_regressions
    DROP COLUMN IF EXISTS force_closed_by_triage_id,
    DROP COLUMN IF EXISTS force_closed_reason,
    DROP COLUMN IF EXISTS force_closed_by,
    DROP COLUMN IF EXISTS force_closed;
