package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/api/handler"
)

type stubQueries struct {
	handler.BookingQueries
	statisticsResp dto.StatisticsResponse
	statisticsErr  error
	gotDateFrom    time.Time
	gotDateTo      time.Time
}

func (s *stubQueries) GetStatistics(_ context.Context, dateFrom, dateTo time.Time) (dto.StatisticsResponse, error) {
	s.gotDateFrom = dateFrom
	s.gotDateTo = dateTo
	return s.statisticsResp, s.statisticsErr
}

type stubService struct {
	handler.BookingService
}

func newTestHandler(queries *stubQueries) *handler.BookingsHandler {
	return handler.NewBookingsHandler(&stubService{}, queries, zap.NewNop())
}

func TestGetStatistics_MissingDateFrom_Returns400(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateTo=2026-01-31", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_MissingDateTo_Returns400(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2026-01-01", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_InvalidDateFormat_Returns400(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=01-01-2026&dateTo=2026-01-31", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_DateToBeforeDateFrom_Returns400(t *testing.T) {
	h := newTestHandler(&stubQueries{})
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2026-02-01&dateTo=2026-01-01", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetStatistics_Success_ReturnsBody(t *testing.T) {
	queries := &stubQueries{
		statisticsResp: dto.StatisticsResponse{
			TotalCount: 3,
			ByStatus: map[string]int64{
				"awaits_confirmation":  1,
				"confirmed":            2,
				"cancelled":            0,
				"cancellation_pending": 0,
			},
			TopResources: []dto.ResourceStatItem{{ResourceID: 10, Count: 3}},
		},
	}
	h := newTestHandler(queries)
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2026-01-01&dateTo=2026-01-31", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body dto.StatisticsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, int64(3), body.TotalCount)
	assert.Equal(t, int64(2), body.ByStatus["confirmed"])
	assert.Equal(t, []dto.ResourceStatItem{{ResourceID: 10, Count: 3}}, body.TopResources)

	expectedFrom, _ := time.Parse(dto.DateFormat, "2026-01-01")
	expectedTo, _ := time.Parse(dto.DateFormat, "2026-01-31")
	assert.True(t, queries.gotDateFrom.Equal(expectedFrom))
	assert.True(t, queries.gotDateTo.Equal(expectedTo))
}

func TestGetStatistics_RepositoryError_Returns500(t *testing.T) {
	queries := &stubQueries{statisticsErr: errors.New("db down")}
	h := newTestHandler(queries)
	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2026-01-01&dateTo=2026-01-31", nil)
	w := httptest.NewRecorder()

	h.GetStatistics(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
