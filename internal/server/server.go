// Package server provides the HTTP server for the edge gateway.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// Server wraps the HTTP server with graceful shutdown.
type Server struct {
	server *http.Server
	router chi.Router
}

// New creates a new server with the given options.
func New(addr string) *Server {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(LoggingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(CORSMiddleware)

	s := &Server{
		server: &http.Server{
			Addr:         addr,
			Handler:      h2c.NewHandler(r, &http2.Server{}),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		router: r,
	}

	return s
}

// Router returns the chi router for adding routes.
func (s *Server) Router() chi.Router {
	return s.router
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	slog.Info("starting server", "addr", s.server.Addr)
	err := s.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down server")
	return s.server.Shutdown(ctx)
}
