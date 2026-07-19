package models_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"booking-service/app/models"
)

func TestNewBooking_Success(t *testing.T) {
	// Arrange
	userID := int64(1)
	resourceID := int64(10)
	startDate := time.Now().AddDate(0, 0, 7)
	endDate := time.Now().AddDate(0, 0, 14)

	// Act
	booking, err := models.NewBooking(userID, resourceID, startDate, endDate)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, booking.Status())
	assert.Equal(t, userID, booking.UserID())
	assert.Equal(t, resourceID, booking.ResourceID())
}

func TestNewBooking_InvalidUserID(t *testing.T) {
	_, err := models.NewBooking(0, 10, time.Now(), time.Now().AddDate(0, 0, 1))
	assert.ErrorIs(t, err, models.ErrInvalidUserID)
}

func TestNewBooking_EndDateBeforeStartDate(t *testing.T) {
	start := time.Now().AddDate(0, 0, 7)
	end := time.Now().AddDate(0, 0, 1)
	_, err := models.NewBooking(1, 10, start, end)
	assert.ErrorIs(t, err, models.ErrEndDateBeforeStartDate)
}

func TestConfirm_FromAwaitsConfirmation(t *testing.T) {
	booking := createTestBooking(t)

	err := booking.Confirm()

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusConfirmed, booking.Status())
}

func TestConfirm_FromConfirmed_Error(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Confirm()

	err := booking.Confirm()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestCancel_FromAwaitsConfirmation(t *testing.T) {
	booking := createTestBooking(t)

	err := booking.Cancel(time.Now())

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancelled, booking.Status())
}

func TestCancel_FromConfirmed_FutureStartDate(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Confirm()
	today := time.Now()

	err := booking.Cancel(today)

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancelled, booking.Status())
}

func TestCancel_FromConfirmed_PastStartDate_Error(t *testing.T) {
	b := models.RestoreBooking(
		1,
		models.BookingStatusConfirmed,
		1, 10,
		time.Now().AddDate(0, 0, -3),
		time.Now().AddDate(0, 0, -1),
		time.Now().AddDate(0, 0, -5),
		"",
		time.Time{},
	)

	err := b.Cancel(time.Now())

	assert.ErrorIs(t, err, models.ErrCannotCancelPastBooking)
}

func TestCancel_FromCancelled_Error(t *testing.T) {
	booking := createTestBooking(t)
	_ = booking.Cancel(time.Now())

	err := booking.Cancel(time.Now())

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestBeginCancellation_FromAwaitsConfirmation(t *testing.T) {
	booking := createTestBooking(t)
	now := time.Now()

	err := booking.BeginCancellation(now)

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancellationPending, booking.Status())
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, booking.PreviousStatus())
	assert.WithinDuration(t, now, booking.CancellationRequestedAt(), 0)
}

func TestBeginCancellation_FromConfirmed_FutureStartDate(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.Confirm())
	now := time.Now()

	err := booking.BeginCancellation(now)

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancellationPending, booking.Status())
	assert.Equal(t, models.BookingStatusConfirmed, booking.PreviousStatus())
	assert.WithinDuration(t, now, booking.CancellationRequestedAt(), 0)
}

func TestBeginCancellation_FromConfirmed_PastStartDate_Error(t *testing.T) {
	b := models.RestoreBooking(
		1,
		models.BookingStatusConfirmed,
		1, 10,
		time.Now().AddDate(0, 0, -3),
		time.Now().AddDate(0, 0, -1),
		time.Now().AddDate(0, 0, -5),
		"",
		time.Time{},
	)

	err := b.BeginCancellation(time.Now())

	assert.ErrorIs(t, err, models.ErrCannotCancelPastBooking)
	assert.Equal(t, models.BookingStatusConfirmed, b.Status())
}

func TestBeginCancellation_FromCancelled_Error(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.Cancel(time.Now()))

	err := booking.BeginCancellation(time.Now())

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestBeginCancellation_FromCancellationPending_Error(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.BeginCancellation(time.Now()))

	err := booking.BeginCancellation(time.Now())

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestCompleteCancellation_FromCancellationPending(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.BeginCancellation(time.Now()))

	err := booking.CompleteCancellation()

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancelled, booking.Status())
	assert.Empty(t, booking.PreviousStatus())
	assert.True(t, booking.CancellationRequestedAt().IsZero())
}

func TestCompleteCancellation_FromAwaitsConfirmation_Error(t *testing.T) {
	booking := createTestBooking(t)

	err := booking.CompleteCancellation()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestCompleteCancellation_FromConfirmed_Error(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.Confirm())

	err := booking.CompleteCancellation()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestCompleteCancellation_FromCancelled_Error(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.Cancel(time.Now()))

	err := booking.CompleteCancellation()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestRollbackCancellation_ToAwaitsConfirmation(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.BeginCancellation(time.Now()))

	err := booking.RollbackCancellation()

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, booking.Status())
	assert.True(t, booking.CancellationRequestedAt().IsZero())
	assert.Equal(t, models.BookingStatus(""), booking.PreviousStatus())
}

func TestRollbackCancellation_ToConfirmed(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.Confirm())
	require.NoError(t, booking.BeginCancellation(time.Now()))

	err := booking.RollbackCancellation()

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusConfirmed, booking.Status())
	assert.True(t, booking.CancellationRequestedAt().IsZero())
	assert.Equal(t, models.BookingStatus(""), booking.PreviousStatus())
}

func TestRollbackCancellation_EmptyPreviousStatus_Error(t *testing.T) {
	b := models.RestoreBooking(
		1,
		models.BookingStatusCancellationPending,
		1, 10,
		time.Now().AddDate(0, 0, 7),
		time.Now().AddDate(0, 0, 14),
		time.Now(),
		"",
		time.Now(),
	)

	err := b.RollbackCancellation()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
	assert.Equal(t, models.BookingStatusCancellationPending, b.Status())
}

func TestRollbackCancellation_FromAwaitsConfirmation_Error(t *testing.T) {
	booking := createTestBooking(t)

	err := booking.RollbackCancellation()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestRollbackCancellation_FromConfirmed_Error(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.Confirm())

	err := booking.RollbackCancellation()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func TestRollbackCancellation_FromCancelled_Error(t *testing.T) {
	booking := createTestBooking(t)
	require.NoError(t, booking.Cancel(time.Now()))

	err := booking.RollbackCancellation()

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
}

func createTestBooking(t *testing.T) *models.Booking {
	t.Helper()
	b, err := models.NewBooking(1, 10, time.Now().AddDate(0, 0, 7), time.Now().AddDate(0, 0, 14))
	require.NoError(t, err)
	return b
}
