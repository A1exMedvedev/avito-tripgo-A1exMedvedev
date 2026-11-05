-- +goose Up
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key          UUID PRIMARY KEY,
    request_hash BYTEA NOT NULL,
    trip_id      UUID NOT NULL REFERENCES trips(id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    created_at   TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS idempotency_keys;
