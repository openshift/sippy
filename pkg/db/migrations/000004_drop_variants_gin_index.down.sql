CREATE INDEX IF NOT EXISTS idx_prow_jobs_variants ON prow_jobs USING gin (variants);
