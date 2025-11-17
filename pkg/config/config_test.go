package config

import (
	"os"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg == nil {
		t.Fatal("Default() returned nil")
	}

	// Test Server defaults
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 50051 {
		t.Errorf("Expected port 50051, got %d", cfg.Server.Port)
	}
	if cfg.Server.MaxConnections != 1000 {
		t.Errorf("Expected max connections 1000, got %d", cfg.Server.MaxConnections)
	}
	if cfg.Server.RequestTimeout != 30*time.Second {
		t.Errorf("Expected request timeout 30s, got %v", cfg.Server.RequestTimeout)
	}
	if cfg.Server.ShutdownTimeout != 10*time.Second {
		t.Errorf("Expected shutdown timeout 10s, got %v", cfg.Server.ShutdownTimeout)
	}
	if cfg.Server.EnableTLS {
		t.Error("Expected TLS disabled by default")
	}

	// Test HNSW defaults
	if cfg.HNSW.M != 16 {
		t.Errorf("Expected M=16, got %d", cfg.HNSW.M)
	}
	if cfg.HNSW.EfConstruction != 200 {
		t.Errorf("Expected EfConstruction=200, got %d", cfg.HNSW.EfConstruction)
	}
	if cfg.HNSW.DefaultEfSearch != 50 {
		t.Errorf("Expected DefaultEfSearch=50, got %d", cfg.HNSW.DefaultEfSearch)
	}
	if cfg.HNSW.Dimensions != 768 {
		t.Errorf("Expected Dimensions=768, got %d", cfg.HNSW.Dimensions)
	}

	// Test Cache defaults
	if !cfg.Cache.Enabled {
		t.Error("Expected cache enabled by default")
	}
	if cfg.Cache.Capacity != 1000 {
		t.Errorf("Expected cache capacity 1000, got %d", cfg.Cache.Capacity)
	}
	if cfg.Cache.TTL != 5*time.Minute {
		t.Errorf("Expected cache TTL 5m, got %v", cfg.Cache.TTL)
	}

	// Test Database defaults
	if cfg.Database.DataDir != "./data" {
		t.Errorf("Expected data dir ./data, got %s", cfg.Database.DataDir)
	}
	if !cfg.Database.EnableWAL {
		t.Error("Expected WAL enabled by default")
	}
	if cfg.Database.SyncWrites {
		t.Error("Expected sync writes disabled by default")
	}
	if cfg.Database.MaxNamespaces != 100 {
		t.Errorf("Expected max namespaces 100, got %d", cfg.Database.MaxNamespaces)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"VECTOR_HOST", "VECTOR_PORT", "VECTOR_MAX_CONNECTIONS",
		"VECTOR_REQUEST_TIMEOUT", "VECTOR_ENABLE_TLS",
		"VECTOR_HNSW_M", "VECTOR_HNSW_EF_CONSTRUCTION", "VECTOR_DIMENSIONS",
		"VECTOR_CACHE_ENABLED", "VECTOR_CACHE_CAPACITY", "VECTOR_CACHE_TTL",
		"VECTOR_DATA_DIR", "VECTOR_ENABLE_WAL", "VECTOR_SYNC_WRITES",
	}

	for _, key := range envVars {
		originalEnv[key] = os.Getenv(key)
	}

	// Cleanup function
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	}()

	// Test server configuration from env
	os.Setenv("VECTOR_HOST", "127.0.0.1")
	os.Setenv("VECTOR_PORT", "8080")
	os.Setenv("VECTOR_MAX_CONNECTIONS", "5000")
	os.Setenv("VECTOR_REQUEST_TIMEOUT", "60s")
	os.Setenv("VECTOR_ENABLE_TLS", "true")

	// Test HNSW configuration from env
	os.Setenv("VECTOR_HNSW_M", "32")
	os.Setenv("VECTOR_HNSW_EF_CONSTRUCTION", "400")
	os.Setenv("VECTOR_DIMENSIONS", "1536")

	// Test Cache configuration from env
	os.Setenv("VECTOR_CACHE_ENABLED", "false")
	os.Setenv("VECTOR_CACHE_CAPACITY", "5000")
	os.Setenv("VECTOR_CACHE_TTL", "10m")

	// Test Database configuration from env
	os.Setenv("VECTOR_DATA_DIR", "/var/lib/vectordb")
	os.Setenv("VECTOR_ENABLE_WAL", "false")
	os.Setenv("VECTOR_SYNC_WRITES", "true")

	cfg := LoadFromEnv()

	// Verify server configuration
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.MaxConnections != 5000 {
		t.Errorf("Expected max connections 5000, got %d", cfg.Server.MaxConnections)
	}
	if cfg.Server.RequestTimeout != 60*time.Second {
		t.Errorf("Expected request timeout 60s, got %v", cfg.Server.RequestTimeout)
	}
	if !cfg.Server.EnableTLS {
		t.Error("Expected TLS enabled")
	}

	// Verify HNSW configuration
	if cfg.HNSW.M != 32 {
		t.Errorf("Expected M=32, got %d", cfg.HNSW.M)
	}
	if cfg.HNSW.EfConstruction != 400 {
		t.Errorf("Expected EfConstruction=400, got %d", cfg.HNSW.EfConstruction)
	}
	// DefaultEfSearch doesn't have env var, should remain default
	if cfg.HNSW.Dimensions != 1536 {
		t.Errorf("Expected Dimensions=1536, got %d", cfg.HNSW.Dimensions)
	}

	// Verify Cache configuration
	if cfg.Cache.Enabled {
		t.Error("Expected cache disabled")
	}
	if cfg.Cache.Capacity != 5000 {
		t.Errorf("Expected cache capacity 5000, got %d", cfg.Cache.Capacity)
	}
	if cfg.Cache.TTL != 10*time.Minute {
		t.Errorf("Expected cache TTL 10m, got %v", cfg.Cache.TTL)
	}

	// Verify Database configuration
	if cfg.Database.DataDir != "/var/lib/vectordb" {
		t.Errorf("Expected data dir /var/lib/vectordb, got %s", cfg.Database.DataDir)
	}
	if cfg.Database.EnableWAL {
		t.Error("Expected WAL disabled")
	}
	if !cfg.Database.SyncWrites {
		t.Error("Expected sync writes enabled")
	}
}

func TestLoadFromEnv_InvalidValues(t *testing.T) {
	// Save original environment
	originalPort := os.Getenv("VECTOR_PORT")
	defer func() {
		if originalPort == "" {
			os.Unsetenv("VECTOR_PORT")
		} else {
			os.Setenv("VECTOR_PORT", originalPort)
		}
	}()

	// Test invalid port (should use default)
	os.Setenv("VECTOR_PORT", "invalid")
	cfg := LoadFromEnv()

	if cfg.Server.Port != 50051 {
		t.Errorf("Expected default port 50051 for invalid value, got %d", cfg.Server.Port)
	}
}

func TestLoadFromEnv_DefaultsWhenNotSet(t *testing.T) {
	// Clear all environment variables
	envVars := []string{
		"VECTOR_HOST", "VECTOR_PORT", "VECTOR_MAX_CONNECTIONS",
		"VECTOR_REQUEST_TIMEOUT", "VECTOR_ENABLE_TLS",
		"VECTOR_HNSW_M", "VECTOR_HNSW_EF_CONSTRUCTION", "VECTOR_DIMENSIONS",
		"VECTOR_CACHE_ENABLED", "VECTOR_CACHE_CAPACITY", "VECTOR_CACHE_TTL",
		"VECTOR_DATA_DIR", "VECTOR_ENABLE_WAL", "VECTOR_SYNC_WRITES",
	}

	// Save and clear
	originalEnv := make(map[string]string)
	for _, key := range envVars {
		originalEnv[key] = os.Getenv(key)
		os.Unsetenv(key)
	}

	// Cleanup
	defer func() {
		for key, value := range originalEnv {
			if value != "" {
				os.Setenv(key, value)
			}
		}
	}()

	cfg := LoadFromEnv()

	// Should match defaults
	defaults := Default()

	if cfg.Server.Host != defaults.Server.Host {
		t.Errorf("Expected default host, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != defaults.Server.Port {
		t.Errorf("Expected default port, got %d", cfg.Server.Port)
	}
	if cfg.HNSW.M != defaults.HNSW.M {
		t.Errorf("Expected default M, got %d", cfg.HNSW.M)
	}
	if cfg.Cache.Enabled != defaults.Cache.Enabled {
		t.Errorf("Expected default cache enabled, got %v", cfg.Cache.Enabled)
	}
	if cfg.Database.DataDir != defaults.Database.DataDir {
		t.Errorf("Expected default data dir, got %s", cfg.Database.DataDir)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "Valid default config",
			config:  Default(),
			wantErr: false,
		},
		{
			name: "Invalid port (too low)",
			config: &Config{
				Server: ServerConfig{Port: 0},
			},
			wantErr: true,
		},
		{
			name: "Invalid port (too high)",
			config: &Config{
				Server: ServerConfig{Port: 70000},
			},
			wantErr: true,
		},
		{
			name: "Invalid M (too low)",
			config: &Config{
				Server: ServerConfig{Port: 50051},
				HNSW:   HNSWConfig{M: 0},
			},
			wantErr: true,
		},
		{
			name: "Invalid dimensions",
			config: &Config{
				Server: ServerConfig{Port: 50051},
				HNSW:   HNSWConfig{M: 16, Dimensions: 0},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerConfig_Address(t *testing.T) {
	cfg := ServerConfig{
		Host: "localhost",
		Port: 8080,
	}

	addr := cfg.Address()
	expected := "localhost:8080"

	if addr != expected {
		t.Errorf("Expected address %s, got %s", expected, addr)
	}

	// Test with default config
	defaultCfg := Default()
	addr = defaultCfg.Server.Address()
	expected = "0.0.0.0:50051"

	if addr != expected {
		t.Errorf("Expected default address %s, got %s", expected, addr)
	}
}
