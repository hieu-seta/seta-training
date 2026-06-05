-- asset-svc schema + tables.
CREATE SCHEMA IF NOT EXISTS asset;
SET search_path TO asset;

CREATE TABLE IF NOT EXISTS folders (
    id          UUID PRIMARY KEY,
    owner_id    UUID NOT NULL,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS notes (
    id          UUID PRIMARY KEY,
    folder_id   UUID NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    owner_id    UUID NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS folder_shares (
    folder_id   UUID NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL,
    access      TEXT NOT NULL CHECK (access IN ('read','write')),
    granted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (folder_id, user_id)
);

CREATE TABLE IF NOT EXISTS note_shares (
    note_id     UUID NOT NULL REFERENCES notes(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL,
    access      TEXT NOT NULL CHECK (access IN ('read','write')),
    granted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (note_id, user_id)
);

CREATE INDEX IF NOT EXISTS folders_owner_idx       ON folders(owner_id);
CREATE INDEX IF NOT EXISTS notes_owner_idx         ON notes(owner_id);
CREATE INDEX IF NOT EXISTS notes_folder_idx        ON notes(folder_id);
CREATE INDEX IF NOT EXISTS folder_shares_user_idx  ON folder_shares(user_id);
CREATE INDEX IF NOT EXISTS note_shares_user_idx    ON note_shares(user_id);
