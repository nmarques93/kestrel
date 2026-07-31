-- +goose Up
CREATE TABLE incidents (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id   bigint NOT NULL REFERENCES targets (id) ON DELETE CASCADE,
    started_at  timestamptz NOT NULL,
    resolved_at timestamptz,
    cause       text
);

CREATE INDEX incidents_target_id_started_at_idx ON incidents (target_id, started_at DESC);

-- Enforce at most one open incident per target: partial unique index on
-- target_id where resolved_at is still null.
CREATE UNIQUE INDEX incidents_one_open_per_target_idx ON incidents (target_id) WHERE resolved_at IS NULL;

-- +goose Down
DROP TABLE incidents;
