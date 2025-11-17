package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all server configuration
type Config struct {
	Server   ServerConfig
	HNSW     HNSWConfig
	Cache    CacheConfig
	Database DatabaseConfig
}

// ServerConfig holds gRPC server configuration
type ServerConfig struct {
	Host            string        // Server host (default: "0.0.0.0")
	Port            int           // Server port (default: 50051)
	MaxConnections  int           // Max concurrent connections
	RequestTimeout  time.Duration // Request timeout
	ShutdownTimeout time.Duration // Graceful shutdown timeout
	EnableTLS       bool          // Enable TLS
	CertFile        string        // TLS certificate file
	KeyFile         string        // TLS key file
}

// HNSWConfig holds HNSW index configuration
type HNSWConfig struct {
	M              int // Number of connections per layer (default: 16)
	EfConstruction int // Construction time accuracy (default: 200)
	DefaultEfSearch int // Default search time accuracy (default: 50)
	Dimensions     int // Vector dimensions (default: 768)
}

// CacheConfig holds query cache configuration
type CacheConfig struct {
	Enabled  bool          // Enable query caching
	Capacity int           // Max cache entries
	TTL      time.Duration // Time to live for cache entries
}

// DatabaseConfig holds storage configuration
type DatabaseConfig struct {
	DataDir      string // Data directory path
	EnableWAL    bool   // Enable write-ahead log
	SyncWrites   bool   // Sync writes to disk
	MaxNamespaces int   // Max number of namespaces
}

// Default returns default configuration
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            50051,
			MaxConnections:  1000,
			RequestTimeout:  30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
			EnableTLS:       false,
		},
		HNSW: HNSWConfig{
			M:              16,
			EfConstruction: 200,
			DefaultEfSearch: 50,
			Dimensions:     768,
		},
		Cache: CacheConfig{
			Enabled:  true,
			Capacity: 1000,
			TTL:      5 * time.Minute,
		},
		Database: DatabaseConfig{
			DataDir:      "./data",
			EnableWAL:    true,
			SyncWrites:   false,
			MaxNamespaces: 100,
		},
	}
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	cfg := Default()

	// Server configuration
	if host := os.Getenv("VECTOR_HOST"); host != "" {
		cfg.Server.Host = host
	}
	if port := os.Getenv("VECTOR_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.Server.Port = p
		}
	}
	if maxConn := os.Getenv("VECTOR_MAX_CONNECTIONS"); maxConn != "" {
		if mc, err := strconv.Atoi(maxConn); err == nil {
			cfg.Server.MaxConnections = mc
		}
	}
	if timeout := os.Getenv("VECTOR_REQUEST_TIMEOUT"); timeout != "" {
		if t, err := time.ParseDuration(timeout); err == nil {
			cfg.Server.RequestTimeout = t
		}
	}
	if enableTLS := os.Getenv("VECTOR_ENABLE_TLS"); enableTLS == "true" {
		cfg.Server.EnableTLS = true
		cfg.Server.CertFile = os.Getenv("VECTOR_TLS_CERT")
		cfg.Server.KeyFile = os.Getenv("VECTOR_TLS_KEY")
	}

	// HNSW configuration
	if m := os.Getenv("VECTOR_HNSW_M"); m != "" {
		if mVal, err := strconv.Atoi(m); err == nil {
			cfg.HNSW.M = mVal
		}
	}
	if ef := os.Getenv("VECTOR_HNSW_EF_CONSTRUCTION"); ef != "" {
		if efVal, err := strconv.Atoi(ef); err == nil {
			cfg.HNSW.EfConstruction = efVal
		}
	}
	if dims := os.Getenv("VECTOR_DIMENSIONS"); dims != "" {
		if d, err := strconv.Atoi(dims); err == nil {
			cfg.HNSW.Dimensions = d
		}
	}

	// Cache configuration
	if cacheEnabled := os.Getenv("VECTOR_CACHE_ENABLED"); cacheEnabled == "false" {
		cfg.Cache.Enabled = false
	}
	if capacity := os.Getenv("VECTOR_CACHE_CAPACITY"); capacity != "" {
		if c, err := strconv.Atoi(capacity); err == nil {
			cfg.Cache.Capacity = c
		}
	}
	if ttl := os.Getenv("VECTOR_CACHE_TTL"); ttl != "" {
		if t, err := time.ParseDuration(ttl); err == nil {
			cfg.Cache.TTL = t
		}
	}

	// Database configuration
	if dataDir := os.Getenv("VECTOR_DATA_DIR"); dataDir != "" {
		cfg.Database.DataDir = dataDir
	}
	if wal := os.Getenv("VECTOR_ENABLE_WAL"); wal == "false" {
		cfg.Database.EnableWAL = false
	}
	if sync := os.Getenv("VECTOR_SYNC_WRITES"); sync == "true" {
		cfg.Database.SyncWrites = true
	}

	return cfg
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Server validation
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", c.Server.Port)
	}
	if c.Server.MaxConnections < 1 {
		return fmt.Errorf("invalid max connections: %d (must be > 0)", c.Server.MaxConnections)
	}
	if c.Server.EnableTLS {
		if c.Server.CertFile == "" || c.Server.KeyFile == "" {
			return fmt.Errorf("TLS enabled but cert or key file not specified")
		}
	}

	// HNSW validation
	if c.HNSW.M < 2 || c.HNSW.M > 100 {
		return fmt.Errorf("invalid HNSW M: %d (recommended: 16)", c.HNSW.M)
	}
	if c.HNSW.EfConstruction < 10 {
		return fmt.Errorf("invalid HNSW efConstruction: %d (must be >= 10)", c.HNSW.EfConstruction)
	}
	if c.HNSW.Dimensions < 1 {
		return fmt.Errorf("invalid dimensions: %d (must be > 0)", c.HNSW.Dimensions)
	}

	// Cache validation
	if c.Cache.Enabled && c.Cache.Capacity < 1 {
		return fmt.Errorf("invalid cache capacity: %d (must be > 0)", c.Cache.Capacity)
	}

	// Database validation
	if c.Database.DataDir == "" {
		return fmt.Errorf("data directory not specified")
	}

	return nil
}

// Address returns the server address (host:port)
func (c *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
