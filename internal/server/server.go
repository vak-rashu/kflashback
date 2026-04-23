// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kflashback/kflashback/internal/ai"
	"github.com/kflashback/kflashback/internal/storage"
)

var serverLog = ctrl.Log.WithName("api-server")

// Server serves the REST API and embedded UI for kflashback.
type Server struct {
	store         storage.Store
	router        *mux.Router
	server        *http.Server
	ai            ai.Provider
	aiContextMode string // "compact" or "full"
}

// New creates a new API server.
func New(store storage.Store, uiDir string, addr string) *Server {
	s := &Server{
		store:  store,
		router: mux.NewRouter(),
	}

	s.registerRoutes(uiDir)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// SetAIProvider configures the AI provider for AI-powered features.
func (s *Server) SetAIProvider(provider ai.Provider, contextMode string) {
	s.ai = provider
	s.aiContextMode = contextMode
	if s.aiContextMode == "" {
		s.aiContextMode = "compact"
	}
	serverLog.Info("AI features enabled", "contextMode", s.aiContextMode)
}

func (s *Server) registerRoutes(uiDir string) {
	// CORS middleware
	s.router.Use(corsMiddleware)

	// API routes
	api := s.router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/stats", s.handleGetStats).Methods("GET", "OPTIONS")
	api.HandleFunc("/stats/kinds", s.handleGetKindStats).Methods("GET", "OPTIONS")
	api.HandleFunc("/resources", s.handleListResources).Methods("GET", "OPTIONS")
	api.HandleFunc("/resources/{uid}", s.handleGetResource).Methods("GET", "OPTIONS")
	api.HandleFunc("/resources/{uid}/history", s.handleGetHistory).Methods("GET", "OPTIONS")
	api.HandleFunc("/resources/{uid}/revisions/{revision}", s.handleGetRevision).Methods("GET", "OPTIONS")
	api.HandleFunc("/resources/{uid}/reconstruct/{revision}", s.handleReconstructAtRevision).Methods("GET", "OPTIONS")
	api.HandleFunc("/resources/{uid}/diff", s.handleDiffRevisions).Methods("GET", "OPTIONS")

	// AI-powered routes (guarded — return 503 if AI not configured)
	s.registerAIRoutes(api)

	// Health endpoints
	s.router.HandleFunc("/healthz", s.handleHealthz).Methods("GET")
	s.router.HandleFunc("/readyz", s.handleReadyz).Methods("GET")

	// Serve embedded UI
	if uiDir != "" {
		spa := spaHandler{staticPath: uiDir, indexPath: "index.html"}
		s.router.PathPrefix("/").Handler(spa)
	}
}

// Start starts the HTTP server in a goroutine.
func (s *Server) Start(ctx context.Context) error {
	serverLog.Info("starting API server", "addr", s.server.Addr)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			serverLog.Error(err, "server shutdown error")
		}
	}()

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server listen error: %w", err)
	}
	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// spaHandler serves a single-page application with proper fallback to index.html.
type spaHandler struct {
	staticPath string
	indexPath  string
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to serve the requested file from staticPath
	path := filepath.Join(h.staticPath, filepath.Clean(r.URL.Path))

	// Check if the file exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) || (info != nil && info.IsDir()) {
		// File doesn't exist or is a directory — serve index.html for SPA routing
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	}
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Serve the actual file (JS, CSS, images, etc.)
	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	_, err := s.store.GetStats(context.Background())
	if err != nil {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
