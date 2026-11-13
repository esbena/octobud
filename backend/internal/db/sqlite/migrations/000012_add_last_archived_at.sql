-- +goose Up
ALTER TABLE notifications ADD COLUMN last_archived_at TEXT;

-- +goose Down
ALTER TABLE notifications DROP COLUMN last_archived_at;
