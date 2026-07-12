-- +goose Up
ALTER TABLE bookings ADD COLUMN previous_status VARCHAR(30);
ALTER TABLE bookings ADD COLUMN cancellation_requested_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE bookings DROP COLUMN previous_status;
ALTER TABLE bookings DROP COLUMN cancellation_requested_at;