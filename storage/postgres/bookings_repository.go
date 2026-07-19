package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"booking-service/app/models"
)

// BookingsRepository реализует models.BookingRepository.
type BookingsRepository struct {
	pool *pgxpool.Pool
}

// NewBookingsRepository создаёт новый экземпляр BookingsRepository.
func NewBookingsRepository(pool *pgxpool.Pool) *BookingsRepository {
	return &BookingsRepository{pool: pool}
}

// Create сохраняет новое бронирование.
func (r *BookingsRepository) Create(ctx context.Context, booking *models.Booking) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, queryInsertBooking,
		string(booking.Status()),
		booking.UserID(),
		booking.ResourceID(),
		booking.StartDate(),
		booking.EndDate(),
		booking.CreatedAt(),
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("создание бронирования: %w", err)
	}
	return id, nil
}

// GetByID возвращает бронирование по ID.
func (r *BookingsRepository) GetByID(ctx context.Context, id int64) (*models.Booking, error) {
	booking, err := r.scanBooking(r.pool.QueryRow(ctx, queryGetBookingByID, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrBookingNotFound
		}
		return nil, fmt.Errorf("получение бронирования id=%d: %w", id, err)
	}
	return booking, nil
}

// Update обновляет статус бронирования.
func (r *BookingsRepository) Update(ctx context.Context, booking *models.Booking) error {
	var previousStatus *string
	if booking.PreviousStatus() != "" {
		s := string(booking.PreviousStatus())
		previousStatus = &s
	}

	var cancellationRequestedAt *time.Time
	if !booking.CancellationRequestedAt().IsZero() {
		t := booking.CancellationRequestedAt()
		cancellationRequestedAt = &t
	}

	tag, err := r.pool.Exec(ctx, queryUpdateBooking,
		string(booking.Status()),
		previousStatus,
		cancellationRequestedAt,
		booking.ID(),
	)
	if err != nil {
		return fmt.Errorf("обновление бронирования id=%d: %w", booking.ID(), err)
	}
	if tag.RowsAffected() == 0 {
		return models.ErrBookingNotFound
	}
	return nil
}

// GetByFilter возвращает бронирования с фильтрацией и пагинацией.
func (r *BookingsRepository) GetByFilter(ctx context.Context, filter models.BookingFilter) ([]models.Booking, int64, error) {
	offset := (filter.Page - 1) * filter.Size

	var userID, resourceID *int64
	var status *string
	if filter.UserID != nil {
		userID = filter.UserID
	}
	if filter.ResourceID != nil {
		resourceID = filter.ResourceID
	}
	if filter.Status != nil {
		s := string(*filter.Status)
		status = &s
	}

	// Получение общего количества
	var totalCount int64
	err := r.pool.QueryRow(ctx, queryCountBookingsByFilter, userID, resourceID, status).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("подсчёт бронирований: %w", err)
	}

	// Получение данных
	rows, err := r.pool.Query(ctx, queryGetBookingsByFilter, userID, resourceID, status, filter.Size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("получение бронирований по фильтру: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("сканирование бронирования: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("итерация по строкам: %w", err)
	}

	return bookings, totalCount, nil
}

// GetAwaitingConfirmation возвращает бронирования, ожидающие подтверждения,
// с пессимистичной блокировкой FOR UPDATE SKIP LOCKED.
func (r *BookingsRepository) GetAwaitingConfirmation(ctx context.Context, limit int) ([]models.Booking, error) {
	rows, err := r.pool.Query(ctx, queryGetAwaitingConfirmation, limit)
	if err != nil {
		return nil, fmt.Errorf("получение бронирований для подтверждения: %w", err)
	}
	defer rows.Close()

	var bookings []models.Booking
	for rows.Next() {
		booking, err := r.scanBookingFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("сканирование бронирования: %w", err)
		}
		bookings = append(bookings, *booking)
	}

	return bookings, rows.Err()
}

// GetStatistics возвращает агрегированную статистику бронирований за период.
// Период включительный с обеих сторон, фильтрация по полю created_at.
// Вся агрегация выполняется на стороне БД.
func (r *BookingsRepository) GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (models.BookingStatistics, error) {
	var totalCount int64
	err := r.pool.QueryRow(ctx, queryCountBookingsByPeriod, dateFrom, dateTo).Scan(&totalCount)
	if err != nil {
		return models.BookingStatistics{}, fmt.Errorf("подсчёт бронирований за период: %w", err)
	}

	statusRows, err := r.pool.Query(ctx, queryGetBookingStatusCounts, dateFrom, dateTo)
	if err != nil {
		return models.BookingStatistics{}, fmt.Errorf("получение статистики по статусам: %w", err)
	}
	defer statusRows.Close()

	byStatus := make(map[models.BookingStatus]int64)
	for statusRows.Next() {
		var status string
		var count int64
		if err := statusRows.Scan(&status, &count); err != nil {
			return models.BookingStatistics{}, fmt.Errorf("сканирование статистики по статусам: %w", err)
		}
		byStatus[models.BookingStatus(status)] = count
	}
	if err := statusRows.Err(); err != nil {
		return models.BookingStatistics{}, fmt.Errorf("итерация по статистике статусов: %w", err)
	}

	resourceRows, err := r.pool.Query(ctx, queryGetTopResourcesByBookingCount, dateFrom, dateTo)
	if err != nil {
		return models.BookingStatistics{}, fmt.Errorf("получение топ ресурсов: %w", err)
	}
	defer resourceRows.Close()

	var topResources []models.ResourceStatistic
	for resourceRows.Next() {
		var stat models.ResourceStatistic
		if err := resourceRows.Scan(&stat.ResourceID, &stat.Count); err != nil {
			return models.BookingStatistics{}, fmt.Errorf("сканирование топ ресурсов: %w", err)
		}
		topResources = append(topResources, stat)
	}
	if err := resourceRows.Err(); err != nil {
		return models.BookingStatistics{}, fmt.Errorf("итерация по топ ресурсам: %w", err)
	}

	return models.BookingStatistics{
		TotalCount:   totalCount,
		ByStatus:     byStatus,
		TopResources: topResources,
	}, nil
}

// scanBooking сканирует одну строку в доменный объект Booking.
func (r *BookingsRepository) scanBooking(row pgx.Row) (*models.Booking, error) {
	var (
		id                      int64
		status                  string
		userID                  int64
		resourceID              int64
		startDate               time.Time
		endDate                 time.Time
		createdAt               time.Time
		previousStatus          *string
		cancellationRequestedAt *time.Time
	)

	err := row.Scan(&id, &status, &userID, &resourceID, &startDate, &endDate, &createdAt, &previousStatus, &cancellationRequestedAt)
	if err != nil {
		return nil, err
	}

	return restoreBookingFromRow(id, status, userID, resourceID, startDate, endDate, createdAt, previousStatus, cancellationRequestedAt), nil
}

// scanBookingFromRows сканирует строку из pgx.Rows.
func (r *BookingsRepository) scanBookingFromRows(rows pgx.Rows) (*models.Booking, error) {
	var (
		id                      int64
		status                  string
		userID                  int64
		resourceID              int64
		startDate               time.Time
		endDate                 time.Time
		createdAt               time.Time
		previousStatus          *string
		cancellationRequestedAt *time.Time
	)

	err := rows.Scan(&id, &status, &userID, &resourceID, &startDate, &endDate, &createdAt, &previousStatus, &cancellationRequestedAt)
	if err != nil {
		return nil, err
	}

	return restoreBookingFromRow(id, status, userID, resourceID, startDate, endDate, createdAt, previousStatus, cancellationRequestedAt), nil
}

// restoreBookingFromRow конвертирует NULL-able колонки БД в доменный объект.
func restoreBookingFromRow(
	id int64,
	status string,
	userID, resourceID int64,
	startDate, endDate, createdAt time.Time,
	previousStatus *string,
	cancellationRequestedAt *time.Time,
) *models.Booking {
	var prevStatus models.BookingStatus
	if previousStatus != nil {
		prevStatus = models.BookingStatus(*previousStatus)
	}

	var sentAt time.Time
	if cancellationRequestedAt != nil {
		sentAt = *cancellationRequestedAt
	}

	return models.RestoreBooking(id, models.BookingStatus(status), userID, resourceID, startDate, endDate, createdAt, prevStatus, sentAt)
}
