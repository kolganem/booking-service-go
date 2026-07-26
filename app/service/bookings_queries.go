package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/models"
)

// BookingsQueries обрабатывает запросы (чтение данных) для бронирований.
type BookingsQueries struct {
	repo   models.BookingRepository
	logger *zap.Logger
}

// NewBookingsQueries создаёт новый BookingsQueries.
func NewBookingsQueries(repo models.BookingRepository, logger *zap.Logger) *BookingsQueries {
	return &BookingsQueries{
		repo:   repo,
		logger: logger,
	}
}

// GetByID возвращает бронирование по ID.
func (q *BookingsQueries) GetByID(ctx context.Context, id int64) (dto.BookingResponse, error) {
	booking, err := q.repo.GetByID(ctx, id)
	if err != nil {
		return dto.BookingResponse{}, err
	}

	return mapBookingToResponse(booking), nil
}

// GetStatus возвращает статус бронирования по ID.
func (q *BookingsQueries) GetStatus(ctx context.Context, id int64) (models.BookingStatus, error) {
	booking, err := q.repo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return booking.Status(), nil
}

// GetStatistics возвращает агрегированную статистику бронирований за период.
func (q *BookingsQueries) GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (dto.StatisticsResponse, error) {
	statistics, err := q.repo.GetStatistics(ctx, dateFrom, dateTo)
	if err != nil {
		return dto.StatisticsResponse{}, fmt.Errorf("получение статистики: %w", err)
	}

	return mapStatisticsToResponse(statistics), nil
}

// mapStatisticsToResponse конвертирует доменную статистику в DTO ответа,
// гарантируя присутствие всех известных статусов (даже с нулевым count).
func mapStatisticsToResponse(s models.BookingStatistics) dto.StatisticsResponse {
	byStatus := map[string]int64{
		string(models.BookingStatusAwaitsConfirmation):  s.ByStatus[models.BookingStatusAwaitsConfirmation],
		string(models.BookingStatusConfirmed):           s.ByStatus[models.BookingStatusConfirmed],
		string(models.BookingStatusCancelled):           s.ByStatus[models.BookingStatusCancelled],
		string(models.BookingStatusCancellationPending): s.ByStatus[models.BookingStatusCancellationPending],
	}

	topResources := make([]dto.ResourceStatItem, 0, len(s.TopResources))
	for _, r := range s.TopResources {
		topResources = append(topResources, dto.ResourceStatItem{
			ResourceID: r.ResourceID,
			Count:      r.Count,
		})
	}

	return dto.StatisticsResponse{
		TotalCount:   s.TotalCount,
		ByStatus:     byStatus,
		TopResources: topResources,
	}
}

// GetByFilter возвращает список бронирований с пагинацией.
func (q *BookingsQueries) GetByFilter(ctx context.Context, req dto.GetBookingsByFilterRequest) (dto.PagedResponse[dto.BookingResponse], error) {
	filter := models.NewDefaultFilter()

	if req.Page > 0 {
		filter.Page = req.Page
	}
	if req.Size > 0 {
		filter.Size = req.Size
	}
	if req.UserID != nil {
		filter.UserID = req.UserID
	}
	if req.ResourceID != nil {
		filter.ResourceID = req.ResourceID
	}
	if req.Status != nil {
		status := models.BookingStatus(*req.Status)
		if !status.IsValid() {
			return dto.PagedResponse[dto.BookingResponse]{}, fmt.Errorf("некорректный статус: %s", *req.Status)
		}
		filter.Status = &status
	}

	bookings, totalCount, err := q.repo.GetByFilter(ctx, filter)
	if err != nil {
		return dto.PagedResponse[dto.BookingResponse]{}, fmt.Errorf("получение бронирований: %w", err)
	}

	items := make([]dto.BookingResponse, 0, len(bookings))
	for i := range bookings {
		items = append(items, mapBookingToResponse(&bookings[i]))
	}

	return dto.PagedResponse[dto.BookingResponse]{
		Items:      items,
		TotalCount: totalCount,
		Page:       filter.Page,
		Size:       filter.Size,
	}, nil
}

// mapBookingToResponse конвертирует доменный объект в DTO ответа.
func mapBookingToResponse(b *models.Booking) dto.BookingResponse {
	return dto.BookingResponse{
		ID:         b.ID(),
		Status:     string(b.Status()),
		UserID:     b.UserID(),
		ResourceID: b.ResourceID(),
		StartDate:  b.StartDate().Format(dto.DateFormat),
		EndDate:    b.EndDate().Format(dto.DateFormat),
		CreatedAt:  b.CreatedAt().Format("2006-01-02T15:04:05Z07:00"),
	}
}
