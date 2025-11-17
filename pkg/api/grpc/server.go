package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
	"github.com/therealutkarshpriyadarshi/vector/pkg/config"
	"github.com/therealutkarshpriyadarshi/vector/pkg/hnsw"
	"github.com/therealutkarshpriyadarshi/vector/pkg/search"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

// Server represents the gRPC server
type Server struct {
	proto.UnimplementedVectorDBServer
	config      *config.Config
	grpcServer  *grpc.Server
	listener    net.Listener
	startTime   time.Time
	shutdownMu  sync.Mutex
	isShutdown  bool

	// Database components
	indexes      map[string]*hnsw.Index       // namespace -> HNSW index
	textIndexes  map[string]*search.FullTextIndex // namespace -> text index
	hybridSearch map[string]*search.CachedHybridSearch // namespace -> cached hybrid search
	metadata     map[string]map[uint64]map[string]interface{} // namespace -> id -> metadata
	mu           sync.RWMutex                 // Protects indexes maps
}

// NewServer creates a new gRPC server
func NewServer(cfg *config.Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	s := &Server{
		config:       cfg,
		indexes:      make(map[string]*hnsw.Index),
		textIndexes:  make(map[string]*search.FullTextIndex),
		hybridSearch: make(map[string]*search.CachedHybridSearch),
		metadata:     make(map[string]map[uint64]map[string]interface{}),
		startTime:    time.Now(),
	}

	// Initialize default namespace
	if err := s.initNamespace("default"); err != nil {
		return nil, fmt.Errorf("failed to initialize default namespace: %w", err)
	}

	return s, nil
}

// initNamespace initializes indexes for a namespace
func (s *Server) initNamespace(namespace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if namespace already exists
	if _, exists := s.indexes[namespace]; exists {
		return nil
	}

	// Create HNSW index with default config
	indexConfig := hnsw.DefaultConfig()
	indexConfig.M = s.config.HNSW.M
	index := hnsw.New(indexConfig)
	s.indexes[namespace] = index

	// Create metadata store for this namespace
	s.metadata[namespace] = make(map[uint64]map[string]interface{})

	// Create full-text index
	textIndex := search.NewFullTextIndex()
	s.textIndexes[namespace] = textIndex

	// Create cached hybrid search
	var cachedSearch *search.CachedHybridSearch
	if s.config.Cache.Enabled {
		cachedSearch = search.NewCachedHybridSearch(
			index,
			textIndex,
			s.config.Cache.Capacity,
			s.config.Cache.TTL,
		)
	} else {
		// Create with zero capacity cache (effectively disabled)
		cachedSearch = search.NewCachedHybridSearch(index, textIndex, 0, 0)
	}
	s.hybridSearch[namespace] = cachedSearch

	log.Printf("Initialized namespace: %s (M=%d, efConstruction=%d, dimensions=%d)",
		namespace, s.config.HNSW.M, s.config.HNSW.EfConstruction, s.config.HNSW.Dimensions)

	return nil
}

// getNamespaceIndexes returns indexes for a namespace (creates if not exists)
func (s *Server) getNamespaceIndexes(namespace string) (*hnsw.Index, *search.FullTextIndex, *search.CachedHybridSearch, error) {
	s.mu.RLock()
	index, indexExists := s.indexes[namespace]
	textIndex, textExists := s.textIndexes[namespace]
	hybridSearch, hybridExists := s.hybridSearch[namespace]
	s.mu.RUnlock()

	if !indexExists || !textExists || !hybridExists {
		// Initialize namespace if it doesn't exist
		if err := s.initNamespace(namespace); err != nil {
			return nil, nil, nil, err
		}

		// Get again after initialization
		s.mu.RLock()
		index = s.indexes[namespace]
		textIndex = s.textIndexes[namespace]
		hybridSearch = s.hybridSearch[namespace]
		s.mu.RUnlock()
	}

	return index, textIndex, hybridSearch, nil
}

// Start starts the gRPC server
func (s *Server) Start() error {
	var opts []grpc.ServerOption

	// Configure TLS if enabled
	if s.config.Server.EnableTLS {
		cert, err := tls.LoadX509KeyPair(s.config.Server.CertFile, s.config.Server.KeyFile)
		if err != nil {
			return fmt.Errorf("failed to load TLS certificates: %w", err)
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.Creds(creds))
		log.Println("TLS enabled")
	}

	// Configure keepalive
	kaParams := keepalive.ServerParameters{
		MaxConnectionIdle: 15 * time.Second,
		MaxConnectionAge:  30 * time.Second,
		Time:              5 * time.Second,
		Timeout:           1 * time.Second,
	}
	opts = append(opts, grpc.KeepaliveParams(kaParams))

	// Configure max connections
	opts = append(opts, grpc.MaxConcurrentStreams(uint32(s.config.Server.MaxConnections)))

	// Create gRPC server
	s.grpcServer = grpc.NewServer(opts...)
	proto.RegisterVectorDBServer(s.grpcServer, s)

	// Enable reflection for debugging (e.g., with grpcurl)
	reflection.Register(s.grpcServer)

	// Create listener
	addr := s.config.Server.Address()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	log.Printf("Vector Database gRPC server listening on %s", addr)

	// Serve in a goroutine
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	s.shutdownMu.Lock()
	defer s.shutdownMu.Unlock()

	if s.isShutdown {
		return nil
	}

	log.Println("Shutting down server...")

	// Create shutdown context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.config.Server.ShutdownTimeout)
	defer cancel()

	// Graceful stop with timeout
	stopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-stopped:
		log.Println("Server stopped gracefully")
	case <-ctx.Done():
		log.Println("Shutdown timeout exceeded, forcing stop")
		s.grpcServer.Stop()
	}

	s.isShutdown = true
	return nil
}

// Wait blocks until the server is stopped
func (s *Server) Wait() {
	if s.listener != nil {
		// This will block until the listener is closed
		<-make(chan struct{})
	}
}

// Uptime returns server uptime
func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}

// Stats returns server statistics
func (s *Server) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]interface{}{
		"uptime_seconds":  s.Uptime().Seconds(),
		"namespaces":      len(s.indexes),
		"cache_enabled":   s.config.Cache.Enabled,
		"namespace_stats": make(map[string]map[string]interface{}),
	}

	// Collect per-namespace stats
	for ns, idx := range s.indexes {
		nodeCount := 0
		if idx != nil {
			nodeCount = int(idx.Size())
		}

		nsStats := map[string]interface{}{
			"vector_count": nodeCount,
			"dimensions":   s.config.HNSW.Dimensions,
		}

		// Add cache stats if available
		if hybridSearch, ok := s.hybridSearch[ns]; ok {
			cacheStats := hybridSearch.CacheStats()
			nsStats["cache_hits"] = cacheStats.Hits
			nsStats["cache_misses"] = cacheStats.Misses
			nsStats["cache_hit_rate"] = cacheStats.HitRate
		}

		stats["namespace_stats"].(map[string]map[string]interface{})[ns] = nsStats
	}

	return stats
}
