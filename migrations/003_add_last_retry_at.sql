-- +goose Up
ALTER TABLE bookings ADD COLUMN last_retry_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE bookings DROP COLUMN last_retry_at;
