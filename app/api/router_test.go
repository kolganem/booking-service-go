package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"booking-service/app/api"
	"booking-service/app/api/dto"
	"booking-service/app/api/handler"
)

type routerStubQueries struct {
	handler.BookingQueries
	called bool
}

func (s *routerStubQueries) GetStatistics(_ context.Context, _, _ time.Time) (dto.StatisticsResponse, error) {
	s.called = true
	return dto.StatisticsResponse{ByStatus: map[string]int64{}}, nil
}

type routerStubService struct {
	handler.BookingService
}

func TestRouter_GetStatistics_RoutesToHandler(t *testing.T) {
	queries := &routerStubQueries{}
	h := handler.NewBookingsHandler(&routerStubService{}, queries, zap.NewNop())
	router := api.NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/api/bookings/statistics?dateFrom=2026-01-01&dateTo=2026-01-31", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, queries.called)
}
