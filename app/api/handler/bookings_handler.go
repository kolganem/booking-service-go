package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"booking-service/app/api/dto"
	"booking-service/app/models"
)

// BookingService определяет командные операции с бронированиями.
type BookingService interface {
	Create(ctx context.Context, req dto.CreateBookingRequest) (int64, error)
	Cancel(ctx context.Context, id int64) error
	RequestCancellation(ctx context.Context, id int64) error
}

// BookingQueries определяет операции чтения бронирований.
type BookingQueries interface {
	GetByID(ctx context.Context, id int64) (dto.BookingResponse, error)
	GetByFilter(ctx context.Context, req dto.GetBookingsByFilterRequest) (dto.PagedResponse[dto.BookingResponse], error)
	GetStatus(ctx context.Context, id int64) (models.BookingStatus, error)
}

// BookingsHandler содержит обработчики HTTP-запросов для бронирований.
type BookingsHandler struct {
	service BookingService
	queries BookingQueries
	logger  *zap.Logger
}

// NewBookingsHandler создаёт новый экземпляр BookingsHandler.
func NewBookingsHandler(service BookingService, queries BookingQueries, logger *zap.Logger) *BookingsHandler {
	return &BookingsHandler{
		service: service,
		queries: queries,
		logger:  logger,
	}
}

// Create обрабатывает POST /api/bookings/create.
func (h *BookingsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblemDetails(w, http.StatusBadRequest, "Некорректный формат запроса", err.Error())
		return
	}

	id, err := h.service.Create(r.Context(), req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, dto.CreateBookingResponse{ID: id})
}

// GetByID обрабатывает GET /api/bookings/{id}.
func (h *BookingsHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		writeProblemDetails(w, http.StatusBadRequest, "Некорректный ID", err.Error())
		return
	}

	booking, err := h.queries.GetByID(r.Context(), id)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, booking)
}

// Cancel обрабатывает PUT /api/bookings/{id}/cancel.
func (h *BookingsHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		writeProblemDetails(w, http.StatusBadRequest, "Некорректный ID", err.Error())
		return
	}

	if err := h.service.RequestCancellation(r.Context(), id); err != nil {
		h.handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetByFilter обрабатывает POST /api/bookings/by-filter.
func (h *BookingsHandler) GetByFilter(w http.ResponseWriter, r *http.Request) {
	var req dto.GetBookingsByFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblemDetails(w, http.StatusBadRequest, "Некорректный формат запроса", err.Error())
		return
	}

	result, err := h.queries.GetByFilter(r.Context(), req)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetStatus обрабатывает GET /api/bookings/{id}/status.
func (h *BookingsHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r)
	if err != nil {
		writeProblemDetails(w, http.StatusBadRequest, "Некорректный ID", err.Error())
		return
	}

	status, err := h.queries.GetStatus(r.Context(), id)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, dto.BookingStatusResponse{Status: string(status)})
}

// handleServiceError маппит доменные ошибки на HTTP-ответы.
func (h *BookingsHandler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrBookingNotFound):
		writeProblemDetails(w, http.StatusNotFound, "Бронирование не найдено", err.Error())
	case errors.Is(err, models.ErrInvalidStatusTransition):
		writeProblemDetails(w, http.StatusConflict, "Недопустимый переход статуса", err.Error())
	case errors.Is(err, models.ErrCannotCancelPastBooking):
		writeProblemDetails(w, http.StatusConflict, "Нельзя отменить прошедшее бронирование", err.Error())
	case errors.Is(err, models.ErrInvalidUserID),
		errors.Is(err, models.ErrInvalidResourceID),
		errors.Is(err, models.ErrInvalidDateRange),
		errors.Is(err, models.ErrEndDateBeforeStartDate):
		writeProblemDetails(w, http.StatusBadRequest, "Ошибка валидации", err.Error())
	default:
		h.logger.Error("необработанная ошибка", zap.Error(err))
		writeProblemDetails(w, http.StatusInternalServerError, "Внутренняя ошибка сервера", "")
	}
}

// Вспомогательные функции

func parseIDParam(r *http.Request) (int64, error) {
	idStr := chi.URLParam(r, "id")
	return strconv.ParseInt(idStr, 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeProblemDetails(w http.ResponseWriter, status int, title, detail string) {
	pd := dto.ProblemDetails{
		Type:   "about:blank",
		Title:  title,
		Status: status,
		Detail: detail,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(pd)
}
