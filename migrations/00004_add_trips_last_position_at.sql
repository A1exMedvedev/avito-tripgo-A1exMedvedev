-- +goose Up
ALTER TABLE trips ADD COLUMN IF NOT EXISTS last_position_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE trips DROP COLUMN IF EXISTS last_position_at;
