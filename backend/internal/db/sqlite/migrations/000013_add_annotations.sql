-- +goose Up
-- Local preview databases may already have this table.
CREATE TABLE IF NOT EXISTS annotations (
    id TEXT NOT NULL PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    code TEXT NOT NULL,
    view_ids TEXT NOT NULL DEFAULT '[]',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS annotations_user_id ON annotations (user_id);

-- +goose Down
DROP TABLE annotations;
