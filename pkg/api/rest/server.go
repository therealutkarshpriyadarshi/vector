package rest

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	pb "github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
	"github.com/therealutkarshpriyadarshi/vector/pkg/api/rest/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds the REST server configuration
type Config struct {
	Host         string
	Port         int
	GRPCAddress  string
	CORSEnabled  bool
	CORSOrigins  []string
	Auth         middleware.AuthConfig
	RateLimit    middleware.RateLimitConfig
}

// Server represents the REST API server
type Server struct {
	config     Config
	handler    *Handler
	httpServer *http.Server
	grpcConn   *grpc.ClientConn
	mux        *http.ServeMux
}

// NewServer creates a new REST API server
func NewServer(config Config) (*Server, error) {
	// Connect to gRPC server
	conn, err := grpc.NewClient(
		config.GRPCAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	// Create gRPC client
	client := pb.NewVectorDBClient(conn)

	// Create handler
	handler := NewHandler(client)

	// Create server
	server := &Server{
		config:   config,
		handler:  handler,
		grpcConn: conn,
		mux:      http.NewServeMux(),
	}

	// Setup routes
	server.setupRoutes()

	// Create HTTP server with middleware
	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler:      server.withMiddleware(server.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server, nil
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Health and stats endpoints
	s.mux.HandleFunc("/v1/health", s.handler.HealthCheck)
	s.mux.HandleFunc("/v1/stats", s.handler.GetStats)
	s.mux.HandleFunc("/v1/stats/", s.handler.GetStats)

	// Vector operations
	s.mux.HandleFunc("/v1/vectors", s.routeVectors)
	s.mux.HandleFunc("/v1/vectors/", s.routeVectorsWithPath)
	s.mux.HandleFunc("/v1/vectors/search", s.handler.Search)
	s.mux.HandleFunc("/v1/vectors/hybrid-search", s.handler.HybridSearch)
	s.mux.HandleFunc("/v1/vectors/delete", s.handler.Delete)
	s.mux.HandleFunc("/v1/vectors/batch", s.handler.BatchInsert)

	// Documentation endpoints
	s.mux.HandleFunc("/docs", ServeSwaggerUI)
	s.mux.HandleFunc("/docs/openapi.yaml", ServeDocs)
}

// routeVectors handles /v1/vectors endpoint
func (s *Server) routeVectors(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handler.Insert(w, r)
	} else {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// routeVectorsWithPath handles /v1/vectors/{namespace}/{id}
func (s *Server) routeVectorsWithPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/vectors/")

	// Check for specific sub-paths
	if strings.HasPrefix(path, "search") || strings.HasPrefix(path, "hybrid-search") ||
		strings.HasPrefix(path, "delete") || strings.HasPrefix(path, "batch") {
		http.NotFound(w, r)
		return
	}

	// Must be namespace/id pattern
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		writeError(w, "Invalid URL format", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodDelete {
		s.handler.Delete(w, r)
	} else if r.Method == http.MethodPut || r.Method == http.MethodPatch {
		s.handler.Update(w, r)
	} else {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// withMiddleware wraps the handler with all middleware
func (s *Server) withMiddleware(handler http.Handler) http.Handler {
	// Apply middleware in reverse order (last one wraps first)

	// 1. Logging middleware (outermost)
	handler = loggingMiddleware(handler)

	// 2. CORS middleware
	if s.config.CORSEnabled {
		handler = corsMiddleware(s.config.CORSOrigins)(handler)
	}

	// 3. Rate limiting
	rateLimiter := middleware.NewRateLimiter(s.config.RateLimit)
	handler = middleware.RateLimitMiddleware(rateLimiter)(handler)

	// 4. Authentication (innermost, runs last)
	handler = middleware.AuthMiddleware(s.config.Auth)(handler)

	return handler
}

// Start starts the REST API server
func (s *Server) Start() error {
	log.Printf("Starting REST API server on %s:%d", s.config.Host, s.config.Port)
	log.Printf("Connecting to gRPC server at %s", s.config.GRPCAddress)
	log.Printf("API Documentation available at http://%s:%d/docs", s.config.Host, s.config.Port)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}
	return nil
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	log.Println("Shutting down REST API server...")

	// Close gRPC connection
	if s.grpcConn != nil {
		if err := s.grpcConn.Close(); err != nil {
			log.Printf("Error closing gRPC connection: %v", err)
		}
	}

	// Shutdown HTTP server
	return s.httpServer.Shutdown(ctx)
}

// loggingMiddleware logs all HTTP requests
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		log.Printf("%s %s %d %v", r.Method, r.URL.Path, wrapped.statusCode, duration)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// corsMiddleware adds CORS headers
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			if len(allowedOrigins) == 0 || (len(allowedOrigins) == 1 && allowedOrigins[0] == "*") {
				allowed = true
				origin = "*"
			} else {
				for _, allowedOrigin := range allowedOrigins {
					if allowedOrigin == origin {
						allowed = true
						break
					}
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "3600")
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
