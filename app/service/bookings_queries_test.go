package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/models"
	"booking-service/app/service"
)

type stubStatisticsRepository struct {
	models.BookingRepository
	statistics models.BookingStatistics
	err        error
}

func (s *stubStatisticsRepository) GetStatistics(_ context.Context, _, _ time.Time) (models.BookingStatistics, error) {
	return s.statistics, s.err
}

func TestGetStatistics_FillsZeroCountForMissingStatuses(t *testing.T) {
	repo := &stubStatisticsRepository{
		statistics: models.BookingStatistics{
			TotalCount: 5,
			ByStatus: map[models.BookingStatus]int64{
				models.BookingStatusConfirmed: 5,
			},
		},
	}
	q := service.NewBookingsQueries(repo, zap.NewNop())

	result, err := q.GetStatistics(context.Background(), time.Now(), time.Now())

	require.NoError(t, err)
	assert.Equal(t, int64(5), result.TotalCount)
	assert.Equal(t, int64(5), result.ByStatus["confirmed"])
	assert.Equal(t, int64(0), result.ByStatus["awaits_confirmation"])
	assert.Equal(t, int64(0), result.ByStatus["cancelled"])
	assert.Equal(t, int64(0), result.ByStatus["cancellation_pending"])
}

func TestGetStatistics_MapsTopResources(t *testing.T) {
	repo := &stubStatisticsRepository{
		statistics: models.BookingStatistics{
			TopResources: []models.ResourceStatistic{
				{ResourceID: 10, Count: 7},
				{ResourceID: 22, Count: 4},
			},
		},
	}
	q := service.NewBookingsQueries(repo, zap.NewNop())

	result, err := q.GetStatistics(context.Background(), time.Now(), time.Now())

	require.NoError(t, err)
	assert.Equal(t, []dto.ResourceStatItem{{ResourceID: 10, Count: 7}, {ResourceID: 22, Count: 4}}, result.TopResources)
}

func TestGetStatistics_PropagatesRepositoryError(t *testing.T) {
	repo := &stubStatisticsRepository{err: errors.New("db down")}
	q := service.NewBookingsQueries(repo, zap.NewNop())

	_, err := q.GetStatistics(context.Background(), time.Now(), time.Now())

	assert.Error(t, err)
}
