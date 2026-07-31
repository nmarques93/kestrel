-- +goose Up
CREATE TABLE checks (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    target_id   bigint NOT NULL REFERENCES targets (id) ON DELETE CASCADE,
    checked_at  timestamptz NOT NULL DEFAULT now(),
    success     boolean NOT NULL,
    status_code integer,
    latency_ms  integer NOT NULL,
    error       text
);

-- Every read this project cares about (recent checks for a target, uptime %
-- over a window) filters by target_id and orders by checked_at.
CREATE INDEX checks_target_id_checked_at_idx ON checks (target_id, checked_at DESC);

-- +goose Down
DROP TABLE checks;
