package postgres

const (
	queryInsertBooking = `
		INSERT INTO bookings (status, user_id, resource_id, start_date, end_date, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	queryGetBookingByID = `
		SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancellation_requested_at
		FROM bookings
		WHERE id = $1`

	queryUpdateBooking = `
		UPDATE bookings
		SET status = $1, previous_status = $2, cancellation_requested_at = $3
		WHERE id = $4`

	queryGetBookingsByFilter = `
		SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancellation_requested_at
		FROM bookings
		WHERE ($1::BIGINT IS NULL OR user_id = $1)
		  AND ($2::BIGINT IS NULL OR resource_id = $2)
		  AND ($3::VARCHAR IS NULL OR status = $3)
		ORDER BY id DESC
		LIMIT $4 OFFSET $5`

	queryCountBookingsByFilter = `
		SELECT COUNT(*)
		FROM bookings
		WHERE ($1::BIGINT IS NULL OR user_id = $1)
		  AND ($2::BIGINT IS NULL OR resource_id = $2)
		  AND ($3::VARCHAR IS NULL OR status = $3)`

	queryGetAwaitingConfirmation = `
		SELECT id, status, user_id, resource_id, start_date, end_date, created_at, previous_status, cancellation_requested_at
		FROM bookings
		WHERE status = 'awaits_confirmation'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED`

	queryCountBookingsByPeriod = `
		SELECT COUNT(*)
		FROM bookings
		WHERE created_at::date BETWEEN $1::date AND $2::date`

	queryGetBookingStatusCounts = `
		SELECT status, COUNT(*)
		FROM bookings
		WHERE created_at::date BETWEEN $1::date AND $2::date
		GROUP BY status`

	queryGetTopResourcesByBookingCount = `
		SELECT resource_id, COUNT(*) AS booking_count
		FROM bookings
		WHERE created_at::date BETWEEN $1::date AND $2::date
		GROUP BY resource_id
		ORDER BY booking_count DESC, resource_id ASC
		LIMIT 5`
)
