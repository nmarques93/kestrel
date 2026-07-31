-- +goose Up
CREATE TABLE targets (
    id                     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name                   text NOT NULL,
    url                    text NOT NULL,
    expected_status_range  int4range NOT NULL DEFAULT int4range(200, 300),
    interval_seconds       integer NOT NULL DEFAULT 60,
    timeout_ms             integer NOT NULL DEFAULT 5000,
    consecutive_threshold  integer NOT NULL DEFAULT 3,
    created_at             timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE targets;
