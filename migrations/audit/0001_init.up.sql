-- audit-worker schema + append-only events table.
CREATE SCHEMA IF NOT EXISTS audit;
SET search_path TO audit;

CREATE TABLE IF NOT EXISTS events (
    id          UUID PRIMARY KEY,
    subject     TEXT NOT NULL,
    idem_key    TEXT NOT NULL UNIQUE,
    occurred_at TIMESTAMPTZ NOT NULL,
    actor_uid   UUID,
    payload     JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS events_occurred_idx ON events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS events_subj_occurred_idx ON events(subject, occurred_at DESC);
