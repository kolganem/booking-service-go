package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

type fakeStuckRepository struct {
	models.BookingRepository
	bookings     []models.Booking
	err          error
	gotOlderThan time.Time
	gotLimit     int
}

func (f *fakeStuckRepository) GetStuckCancellations(_ context.Context, olderThan time.Time, limit int) ([]models.Booking, error) {
	f.gotOlderThan = olderThan
	f.gotLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.bookings, nil
}

type fakePublisher struct {
	published []string
	errFor    map[string]error
}

func (f *fakePublisher) PublishCancelBookingJob(_ context.Context, cmd messaging.CancelBookingJobCommand) error {
	if err, ok := f.errFor[cmd.RequestId]; ok {
		return err
	}
	f.published = append(f.published, cmd.RequestId)
	return nil
}

func stuckBooking(id int64, requestedAt time.Time) models.Booking {
	b := models.RestoreBooking(
		id,
		models.BookingStatusCancellationPending,
		1, 10,
		time.Now().AddDate(0, 0, 7),
		time.Now().AddDate(0, 0, 14),
		time.Now().AddDate(0, 0, -1),
		models.BookingStatusAwaitsConfirmation,
		requestedAt,
	)
	return *b
}

func TestProcessBatch_PublishesForEachStuckBooking(t *testing.T) {
	repo := &fakeStuckRepository{bookings: []models.Booking{
		stuckBooking(1, time.Now().Add(-10*time.Minute)),
		stuckBooking(2, time.Now().Add(-10*time.Minute)),
	}}
	pub := &fakePublisher{}
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 10, zap.NewNop())

	w.processBatch(context.Background())

	assert.ElementsMatch(t, []string{
		messaging.BookingIDToRequestID(1),
		messaging.BookingIDToRequestID(2),
	}, pub.published)
}

func TestProcessBatch_ErrorOnOneBooking_DoesNotStopOthers(t *testing.T) {
	repo := &fakeStuckRepository{bookings: []models.Booking{
		stuckBooking(1, time.Now().Add(-10*time.Minute)),
		stuckBooking(2, time.Now().Add(-10*time.Minute)),
		stuckBooking(3, time.Now().Add(-10*time.Minute)),
	}}
	pub := &fakePublisher{errFor: map[string]error{
		messaging.BookingIDToRequestID(2): errors.New("publish failed"),
	}}
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 10, zap.NewNop())

	w.processBatch(context.Background())

	assert.ElementsMatch(t, []string{
		messaging.BookingIDToRequestID(1),
		messaging.BookingIDToRequestID(3),
	}, pub.published)
}

func TestProcessBatch_UsesTimeoutAndBatchSize(t *testing.T) {
	repo := &fakeStuckRepository{}
	pub := &fakePublisher{}
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 7, zap.NewNop())

	before := time.Now()
	w.processBatch(context.Background())

	assert.Equal(t, 7, repo.gotLimit)
	assert.WithinDuration(t, before.Add(-5*time.Minute), repo.gotOlderThan, time.Second)
}

func TestProcessBatch_RepositoryError_DoesNotPublish(t *testing.T) {
	repo := &fakeStuckRepository{err: errors.New("db down")}
	pub := &fakePublisher{}
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 10, zap.NewNop())

	w.processBatch(context.Background())

	assert.Empty(t, pub.published)
}

func TestProcessBatch_EmptyResult_DoesNotPublish(t *testing.T) {
	repo := &fakeStuckRepository{bookings: nil}
	pub := &fakePublisher{}
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 10, zap.NewNop())

	w.processBatch(context.Background())

	assert.Empty(t, pub.published)
}
