-- +goose Up
ALTER TABLE radio ADD COLUMN average_rating REAL NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE radio DROP COLUMN average_rating;
