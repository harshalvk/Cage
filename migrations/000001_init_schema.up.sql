CREATE TABLE IF NOT EXISTS sandboxes (
    id           UUID PRIMARY KEY,
    container_id TEXT NOT NULL,
    status       TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);