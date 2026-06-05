-- team-svc schema + tables.
CREATE SCHEMA IF NOT EXISTS team;
SET search_path TO team;

CREATE TABLE IF NOT EXISTS teams (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    created_by  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS team_members (
    team_id  UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id  UUID NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE IF NOT EXISTS team_managers (
    team_id  UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id  UUID NOT NULL,
    is_main  BOOLEAN NOT NULL DEFAULT false,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);

CREATE INDEX IF NOT EXISTS team_members_user_idx ON team_members(user_id);
CREATE INDEX IF NOT EXISTS team_managers_user_idx ON team_managers(user_id);
-- Guarantee at most one main manager per team.
CREATE UNIQUE INDEX IF NOT EXISTS team_managers_one_main_idx ON team_managers(team_id) WHERE is_main;
