CREATE TABLE workqueue_symptom_re_batches (
    id              UUID PRIMARY KEY,
    requested_count INT NOT NULL,
    enqueued_count  INT NOT NULL DEFAULT 0,
    deduped_count   INT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_symptom_re_batches_status ON workqueue_symptom_re_batches (status);

CREATE TABLE workqueue_symptom_re_batch_items (
    id           BIGSERIAL PRIMARY KEY,
    batch_id     UUID NOT NULL REFERENCES workqueue_symptom_re_batches(id) ON DELETE CASCADE,
    river_job_id BIGINT,
    item_key     TEXT NOT NULL,
    UNIQUE(batch_id, item_key)
);

CREATE INDEX idx_symptom_re_batch_items_batch
    ON workqueue_symptom_re_batch_items (batch_id, river_job_id);
