package api

import (
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"booking-service/app/api/handler"
	mw "booking-service/app/api/middleware"
)

// NewRouter создаёт и настраивает chi-роутер.
func NewRouter(bookingsHandler *handler.BookingsHandler) chi.Router {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(mw.RequestLogger)

	// Health check
	r.Get("/health", handler.HealthCheck)

	// Маршруты бронирований
	r.Route("/api/bookings", func(r chi.Router) {
		r.Post("/create", bookingsHandler.Create)           // POST api/bookings/create
		r.Post("/by-filter", bookingsHandler.GetByFilter)   // POST api/bookings/by-filter
		r.Get("/statistics", bookingsHandler.GetStatistics) // GET  api/bookings/statistics
		r.Get("/{id}", bookingsHandler.GetByID)             // GET  api/bookings/{id}
		r.Put("/{id}/cancel", bookingsHandler.Cancel)       // PUT  api/bookings/{id}/cancel
		r.Get("/{id}/status", bookingsHandler.GetStatus)    // GET  api/bookings/{id}/status
	})

	return r
}
