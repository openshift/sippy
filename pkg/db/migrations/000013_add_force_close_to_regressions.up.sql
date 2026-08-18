-- TRT-2895: Force close regressions.
--
-- Generic tests (e.g. "install should succeed") stay open for weeks because the
-- 5-day regression reuse window (regressionHysteresisDays) reopens recently
-- closed regressions for unrelated failures, causing false "pants on fire" /
-- "failed fix" status. Force closing a resolved triage's regressions marks them
-- so they are excluded from the reuse window and never reopened.
--
-- All force close metadata lives on test_regressions so a regression is
-- self-contained: it records that it was force closed, by whom, why, and which
-- triage drove the action. Existing rows default to not force closed.

ALTER TABLE test_regressions
    ADD COLUMN IF NOT EXISTS force_closed BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS force_closed_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS force_closed_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS force_closed_by_triage_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_test_regressions_force_closed_by_triage_id
    ON test_regressions (force_closed_by_triage_id);
