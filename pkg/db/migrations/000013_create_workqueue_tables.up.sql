CREATE TABLE workqueue_batches (
    id              UUID PRIMARY KEY,
    kind            TEXT NOT NULL,
    requested_count INT NOT NULL,
    enqueued_count  INT NOT NULL DEFAULT 0,
    deduped_count   INT NOT NULL DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ
);

CREATE INDEX idx_workqueue_batches_kind_status ON workqueue_batches (kind, status);

CREATE TABLE workqueue_batch_items (
    id           BIGSERIAL PRIMARY KEY,
    batch_id     UUID NOT NULL REFERENCES workqueue_batches(id) ON DELETE CASCADE,
    river_job_id BIGINT NOT NULL,
    item_key     TEXT NOT NULL,
    UNIQUE(batch_id, item_key)
);

CREATE INDEX idx_workqueue_batch_items_batch ON workqueue_batch_items (batch_id, river_job_id);
