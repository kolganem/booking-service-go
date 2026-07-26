package dto

// CreateBookingRequest -- запрос на создание бронирования.
type CreateBookingRequest struct {
	UserID     int64  `json:"userId"`
	ResourceID int64  `json:"resourceId"`
	StartDate  string `json:"startDate"` // формат: "2006-01-02"
	EndDate    string `json:"endDate"`   // формат: "2006-01-02"
}

// CreateBookingResponse -- ответ при создании бронирования.
type CreateBookingResponse struct {
	ID int64 `json:"id"`
}

// BookingResponse -- полные данные бронирования.
type BookingResponse struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	UserID     int64  `json:"userId"`
	ResourceID int64  `json:"resourceId"`
	StartDate  string `json:"startDate"`
	EndDate    string `json:"endDate"`
	CreatedAt  string `json:"createdAt"` // формат: RFC3339
}

// BookingStatusResponse -- статус бронирования.
type BookingStatusResponse struct {
	Status string `json:"status"`
}

// GetBookingsByFilterRequest -- запрос с фильтром и пагинацией.
type GetBookingsByFilterRequest struct {
	UserID     *int64  `json:"userId,omitempty"`
	ResourceID *int64  `json:"resourceId,omitempty"`
	Status     *string `json:"status,omitempty"`
	Page       int     `json:"page"`
	Size       int     `json:"size"`
}

// PagedResponse -- ответ с пагинацией.
type PagedResponse[T any] struct {
	Items      []T   `json:"items"`
	TotalCount int64 `json:"totalCount"`
	Page       int   `json:"page"`
	Size       int   `json:"size"`
}

// ProblemDetails -- стандартный формат ошибки RFC 7807.
type ProblemDetails struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// DateFormat -- формат даты для JSON-сериализации.
const DateFormat = "2006-01-02"

// StatisticsResponse -- агрегированная статистика бронирований за период.
type StatisticsResponse struct {
	TotalCount   int64              `json:"totalCount"`
	ByStatus     map[string]int64   `json:"byStatus"`
	TopResources []ResourceStatItem `json:"topResources"`
}

// ResourceStatItem -- количество бронирований по одному ресурсу.
type ResourceStatItem struct {
	ResourceID int64 `json:"resourceId"`
	Count      int64 `json:"count"`
}
