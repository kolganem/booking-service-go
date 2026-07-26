package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"booking-service/app/messaging"
	"booking-service/app/models"
	"booking-service/app/service"
)

// BookingConfirmedHandler обрабатывает события BookingJobConfirmed.
type BookingConfirmedHandler struct {
	service *service.BookingsService
	logger  *zap.Logger
}

// NewBookingConfirmedHandler создаёт новый обработчик.
func NewBookingConfirmedHandler(svc *service.BookingsService, logger *zap.Logger) *BookingConfirmedHandler {
	return &BookingConfirmedHandler{
		service: svc,
		logger:  logger,
	}
}

// Handle обрабатывает событие подтверждения бронирования.
func (h *BookingConfirmedHandler) Handle(ctx context.Context, body []byte) error {
	var event messaging.BookingJobConfirmed
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("десериализация BookingJobConfirmed: %w", err)
	}

	bookingID, err := messaging.RequestIDToBookingID(event.RequestId)
	if err != nil {
		return fmt.Errorf("извлечение bookingId из RequestId: %w", err)
	}

	h.logger.Info("получено событие BookingJobConfirmed",
		zap.Int64("bookingId", bookingID),
		zap.Int64("catalogJobId", event.Id),
	)

	previousStatus, err := h.service.Confirm(ctx, bookingID)
	if err != nil {
		switch {
		case errors.Is(err, models.ErrBookingNotFound):
			h.logger.Warn("подтверждение невозможно: бронирование не найдено, пропускаем событие",
				zap.Int64("bookingId", bookingID), zap.Error(err))
			return nil
		case errors.Is(err, models.ErrInvalidStatusTransition) && previousStatus == models.BookingStatusCancelled:
			h.logger.Error("рассинхронизация с Catalog: получено подтверждение для уже отменённого бронирования",
				zap.Int64("bookingId", bookingID), zap.Int64("catalogJobId", event.Id))
			return nil
		case errors.Is(err, models.ErrInvalidStatusTransition):
			h.logger.Debug("бронирование уже подтверждено, дубликат события пропущен",
				zap.Int64("bookingId", bookingID))
			return nil
		}
		return fmt.Errorf("подтверждение бронирования %d: %w", bookingID, err)
	}

	if previousStatus == models.BookingStatusCancellationPending {
		h.logger.Warn("race condition: Catalog подтвердил бронирование, отмена которого уже была начата -- синхронизируемся с решением Catalog",
			zap.Int64("bookingId", bookingID),
		)
	}

	h.logger.Info("бронирование подтверждено через событие", zap.Int64("bookingId", bookingID))
	return nil
}
