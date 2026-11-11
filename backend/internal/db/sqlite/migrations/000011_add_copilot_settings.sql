-- +goose Up
ALTER TABLE users ADD COLUMN copilot_settings TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN copilot_settings;
