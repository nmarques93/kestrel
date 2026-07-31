-- +goose Up
CREATE TABLE mcp_tokens (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash    text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz
);

CREATE UNIQUE INDEX mcp_tokens_token_hash_idx ON mcp_tokens (token_hash);

-- +goose Down
DROP TABLE mcp_tokens;
