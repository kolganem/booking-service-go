package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"booking-service/app/messaging"
	"booking-service/app/models"
)

type fakeStuckRepository struct {
	models.BookingRepository
	bookings     []models.Booking
	err          error
	gotOlderThan time.Time
	gotLimit     int
	updated      []models.Booking
	updateErr    error
}

func (f *fakeStuckRepository) GetStuckCancellations(_ context.Context, olderThan time.Time, limit int) ([]models.Booking, error) {
	f.gotOlderThan = olderThan
	f.gotLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.bookings, nil
}

func (f *fakeStuckRepository) Update(_ context.Context, booking *models.Booking) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updated = append(f.updated, *booking)
	return nil
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
		time.Time{},
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

func TestProcessBatch_MarksProgress_OnSuccessfulPublish(t *testing.T) {
	repo := &fakeStuckRepository{bookings: []models.Booking{
		stuckBooking(1, time.Now().Add(-10*time.Minute)),
		stuckBooking(2, time.Now().Add(-10*time.Minute)),
	}}
	pub := &fakePublisher{}
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 10, zap.NewNop())

	before := time.Now()
	w.processBatch(context.Background())

	require.Len(t, repo.updated, 2)
	for _, b := range repo.updated {
		assert.False(t, b.LastRetryAt().IsZero())
		assert.WithinDuration(t, before, b.LastRetryAt(), time.Second)
	}
}

func TestProcessBatch_DoesNotMarkProgress_OnPublishFailure(t *testing.T) {
	repo := &fakeStuckRepository{bookings: []models.Booking{
		stuckBooking(1, time.Now().Add(-10*time.Minute)),
		stuckBooking(2, time.Now().Add(-10*time.Minute)),
	}}
	pub := &fakePublisher{errFor: map[string]error{
		messaging.BookingIDToRequestID(2): errors.New("publish failed"),
	}}
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 10, zap.NewNop())

	w.processBatch(context.Background())

	require.Len(t, repo.updated, 1)
	assert.Equal(t, int64(1), repo.updated[0].ID())
}

func TestProcessBatch_LogsSummaryWithRetryAndErrorCounts(t *testing.T) {
	repo := &fakeStuckRepository{bookings: []models.Booking{
		stuckBooking(1, time.Now().Add(-10*time.Minute)),
		stuckBooking(2, time.Now().Add(-10*time.Minute)),
		stuckBooking(3, time.Now().Add(-10*time.Minute)),
	}}
	pub := &fakePublisher{errFor: map[string]error{
		messaging.BookingIDToRequestID(2): errors.New("publish failed"),
	}}
	core, recorded := observer.New(zapcore.InfoLevel)
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 10, zap.New(core))

	w.processBatch(context.Background())

	summary := recorded.FilterMessage("итог обработки пакета зависших отмен")
	require.Equal(t, 1, summary.Len())

	entry := summary.All()[0]
	assert.Equal(t, zapcore.InfoLevel, entry.Level)
	fields := entry.ContextMap()
	assert.EqualValues(t, 2, fields["retry"])
	assert.EqualValues(t, 1, fields["errors"])
}

func TestProcessBatch_UpdateError_DoesNotStopOthers(t *testing.T) {
	repo := &fakeStuckRepository{
		bookings: []models.Booking{
			stuckBooking(1, time.Now().Add(-10*time.Minute)),
			stuckBooking(2, time.Now().Add(-10*time.Minute)),
		},
		updateErr: errors.New("db down"),
	}
	pub := &fakePublisher{}
	w := NewCancellationRetryWorker(repo, pub, 5*time.Minute, time.Second, 10, zap.NewNop())

	w.processBatch(context.Background())

	assert.ElementsMatch(t, []string{
		messaging.BookingIDToRequestID(1),
		messaging.BookingIDToRequestID(2),
	}, pub.published)
}
