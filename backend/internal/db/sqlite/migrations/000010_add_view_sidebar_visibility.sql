-- +goose Up
ALTER TABLE views ADD COLUMN show_in_sidebar INTEGER NOT NULL DEFAULT 1 CHECK (show_in_sidebar IN (0, 1));

-- +goose Down
ALTER TABLE views DROP COLUMN show_in_sidebar;
