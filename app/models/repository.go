package models

import (
	"context"
	"time"
)

// BookingRepository -- интерфейс репозитория бронирований.
type BookingRepository interface {
	// Create сохраняет новое бронирование и возвращает присвоенный ID.
	Create(ctx context.Context, booking *Booking) (int64, error)

	// GetByID возвращает бронирование по ID.
	GetByID(ctx context.Context, id int64) (*Booking, error)

	// Update обновляет бронирование в хранилище.
	Update(ctx context.Context, booking *Booking) error

	// GetByFilter возвращает список бронирований с пагинацией.
	GetByFilter(ctx context.Context, filter BookingFilter) ([]Booking, int64, error)

	// GetAwaitingConfirmation возвращает бронирования в статусе AwaitsConfirmation.
	// Без блокировки строк: при нескольких работающих инстансах воркера одна и
	// та же запись может быть возвращена параллельно более чем одному вызову.
	GetAwaitingConfirmation(ctx context.Context, limit int) ([]Booking, error)

	// GetStuckCancellations возвращает бронирования в статусе CancellationPending,
	// у которых cancellation_requested_at старше olderThan.
	// Без блокировки строк: при нескольких работающих инстансах воркера одна и
	// та же запись может быть возвращена параллельно более чем одному вызову.
	GetStuckCancellations(ctx context.Context, olderThan time.Time, limit int) ([]Booking, error)

	// GetStatistics возвращает статистику бронирований за указанный период.
	GetStatistics(ctx context.Context, dateFrom, dateTo time.Time) (BookingStatistics, error)
}

// BookingFilter содержит параметры фильтрации и пагинации.
type BookingFilter struct {
	UserID     *int64
	ResourceID *int64
	Status     *BookingStatus
	Page       int
	Size       int
}

// NewDefaultFilter создаёт фильтр с пагинацией по умолчанию.
func NewDefaultFilter() BookingFilter {
	return BookingFilter{
		Page: 1,
		Size: 25,
	}
}
