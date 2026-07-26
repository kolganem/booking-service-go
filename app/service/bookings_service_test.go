package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"booking-service/app/models"
	"booking-service/app/service"
)

type fakeConfirmRepository struct {
	models.BookingRepository
	booking   *models.Booking
	getErr    error
	updateErr error
	updated   *models.Booking
}

func (f *fakeConfirmRepository) GetByID(_ context.Context, _ int64) (*models.Booking, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.booking, nil
}

func (f *fakeConfirmRepository) Update(_ context.Context, booking *models.Booking) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = booking
	return nil
}

func newAwaitingBooking(t *testing.T) *models.Booking {
	t.Helper()
	b, err := models.NewBooking(1, 10, time.Now().AddDate(0, 0, 7), time.Now().AddDate(0, 0, 14))
	require.NoError(t, err)
	return b
}

func newCancellationPendingBooking(t *testing.T) *models.Booking {
	t.Helper()
	b := newAwaitingBooking(t)
	require.NoError(t, b.BeginCancellation(time.Now()))
	return b
}

func TestConfirm_FromAwaitsConfirmation_ReturnsPreviousStatus(t *testing.T) {
	repo := &fakeConfirmRepository{booking: newAwaitingBooking(t)}
	svc := service.NewBookingsService(repo, nil, zap.NewNop())

	previousStatus, err := svc.Confirm(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusAwaitsConfirmation, previousStatus)
	assert.Equal(t, models.BookingStatusConfirmed, repo.updated.Status())
}

func TestConfirm_FromCancellationPending_ReturnsPreviousStatus(t *testing.T) {
	repo := &fakeConfirmRepository{booking: newCancellationPendingBooking(t)}
	svc := service.NewBookingsService(repo, nil, zap.NewNop())

	previousStatus, err := svc.Confirm(context.Background(), 1)

	require.NoError(t, err)
	assert.Equal(t, models.BookingStatusCancellationPending, previousStatus)
	assert.Equal(t, models.BookingStatusConfirmed, repo.updated.Status())
}

func TestConfirm_RepositoryGetError_PropagatesError(t *testing.T) {
	wantErr := errors.New("db down")
	repo := &fakeConfirmRepository{getErr: wantErr}
	svc := service.NewBookingsService(repo, nil, zap.NewNop())

	_, err := svc.Confirm(context.Background(), 1)

	assert.ErrorIs(t, err, wantErr)
}

func TestConfirm_InvalidTransition_PropagatesErrorAndCurrentStatus(t *testing.T) {
	booking := newAwaitingBooking(t)
	require.NoError(t, booking.Confirm())
	repo := &fakeConfirmRepository{booking: booking}
	svc := service.NewBookingsService(repo, nil, zap.NewNop())

	previousStatus, err := svc.Confirm(context.Background(), 1)

	assert.ErrorIs(t, err, models.ErrInvalidStatusTransition)
	assert.Equal(t, models.BookingStatusConfirmed, previousStatus)
}
