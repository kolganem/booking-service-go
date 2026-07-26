package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"booking-service/app/messaging"
	"booking-service/app/messaging/handlers"
	"booking-service/app/models"
	"booking-service/app/service"
)

type fakeRepository struct {
	models.BookingRepository
	booking   *models.Booking
	getErr    error
	updateErr error
}

func (f *fakeRepository) GetByID(_ context.Context, _ int64) (*models.Booking, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.booking, nil
}

func (f *fakeRepository) Update(_ context.Context, booking *models.Booking) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.booking = booking
	return nil
}

func newHandler(repo *fakeRepository) (*handlers.BookingConfirmedHandler, *observer.ObservedLogs) {
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	svc := service.NewBookingsService(repo, nil, logger)
	return handlers.NewBookingConfirmedHandler(svc, logger), recorded
}

func eventBody(t *testing.T, bookingID int64) []byte {
	t.Helper()
	body, err := json.Marshal(messaging.BookingJobConfirmed{
		EventId:   messaging.NewMessageID(),
		Id:        1,
		RequestId: messaging.BookingIDToRequestID(bookingID),
	})
	require.NoError(t, err)
	return body
}

func newAwaitingBooking(t *testing.T) *models.Booking {
	t.Helper()
	b, err := models.NewBooking(1, 10, time.Now().AddDate(0, 0, 7), time.Now().AddDate(0, 0, 14))
	require.NoError(t, err)
	return b
}

func TestHandle_FromAwaitsConfirmation_NoRaceConditionWarning(t *testing.T) {
	repo := &fakeRepository{booking: newAwaitingBooking(t)}
	h, recorded := newHandler(repo)

	err := h.Handle(context.Background(), eventBody(t, 1))

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusConfirmed, repo.booking.Status())
	assert.Equal(t, 0, recorded.FilterMessageSnippet("race condition").Len())
}

func TestHandle_FromCancellationPending_LogsRaceConditionWarning(t *testing.T) {
	booking := newAwaitingBooking(t)
	require.NoError(t, booking.BeginCancellation(time.Now()))
	repo := &fakeRepository{booking: booking}
	h, recorded := newHandler(repo)

	err := h.Handle(context.Background(), eventBody(t, 1))

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusConfirmed, repo.booking.Status())

	warnings := recorded.FilterMessageSnippet("race condition")
	require.Equal(t, 1, warnings.Len())
	assert.Equal(t, zapcore.WarnLevel, warnings.All()[0].Level)
}

func TestHandle_BookingNotFound_ReturnsNilAndLogsWarning(t *testing.T) {
	repo := &fakeRepository{getErr: models.ErrBookingNotFound}
	h, recorded := newHandler(repo)

	err := h.Handle(context.Background(), eventBody(t, 1))

	require.NoError(t, err)
	assert.Equal(t, 1, recorded.FilterMessageSnippet("бронирование не найдено").Len())
}

func TestHandle_AlreadyConfirmed_ReturnsNilAndLogsDebug(t *testing.T) {
	booking := newAwaitingBooking(t)
	require.NoError(t, booking.Confirm())
	repo := &fakeRepository{booking: booking}
	h, recorded := newHandler(repo)

	err := h.Handle(context.Background(), eventBody(t, 1))

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusConfirmed, repo.booking.Status())

	entries := recorded.FilterMessageSnippet("уже подтверждено")
	require.Equal(t, 1, entries.Len())
	assert.Equal(t, zapcore.DebugLevel, entries.All()[0].Level)
}

func TestHandle_AlreadyCancelled_ReturnsNilAndLogsDesyncError(t *testing.T) {
	booking := newAwaitingBooking(t)
	require.NoError(t, booking.Cancel(time.Now()))
	repo := &fakeRepository{booking: booking}
	h, recorded := newHandler(repo)

	err := h.Handle(context.Background(), eventBody(t, 1))

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancelled, repo.booking.Status())

	entries := recorded.FilterMessageSnippet("рассинхронизация с Catalog")
	require.Equal(t, 1, entries.Len())
	assert.Equal(t, zapcore.ErrorLevel, entries.All()[0].Level)
}

func TestHandle_InvalidRequestId_ReturnsError(t *testing.T) {
	repo := &fakeRepository{}
	h, _ := newHandler(repo)
	body, err := json.Marshal(messaging.BookingJobConfirmed{RequestId: "not-a-valid-uuid"})
	require.NoError(t, err)

	handleErr := h.Handle(context.Background(), body)

	assert.Error(t, handleErr)
}

func TestHandle_RepositoryUpdateError_ReturnsWrappedError(t *testing.T) {
	repo := &fakeRepository{booking: newAwaitingBooking(t), updateErr: errors.New("db down")}
	h, _ := newHandler(repo)

	err := h.Handle(context.Background(), eventBody(t, 1))

	assert.Error(t, err)
}
