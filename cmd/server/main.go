package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	grpcserver "github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc"
	"github.com/therealutkarshpriyadarshi/vector/pkg/config"
)

var (
	version = "1.0.0"
	commit  = "dev"
)

func main() {
	// Parse command-line flags
	var (
		showVersion = flag.Bool("version", false, "show version and exit")
		showHelp    = flag.Bool("help", false, "show help and exit")
		configFile  = flag.String("config", "", "path to configuration file (optional)")
		host        = flag.String("host", "", "server host (overrides config/env)")
		port        = flag.Int("port", 0, "server port (overrides config/env)")
	)
	flag.Parse()

	// Show version
	if *showVersion {
		fmt.Printf("Vector Database Server v%s (commit: %s)\n", version, commit)
		os.Exit(0)
	}

	// Show help
	if *showHelp {
		showUsage()
		os.Exit(0)
	}

	// Print banner
	printBanner()

	// Load configuration
	cfg := loadConfig(*configFile)

	// Override with command-line flags
	if *host != "" {
		cfg.Server.Host = *host
	}
	if *port > 0 {
		cfg.Server.Port = *port
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// Create server
	log.Println("Initializing Vector Database server...")
	server, err := grpcserver.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	log.Println("Starting gRPC server...")
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Print startup info
	printStartupInfo(cfg)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	// Wait for shutdown signal
	log.Println("Server is ready. Press Ctrl+C to stop.")
	sig := <-sigChan
	log.Printf("Received signal: %v", sig)

	// Graceful shutdown
	log.Println("Shutting down gracefully...")
	if err := server.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Server stopped. Goodbye!")
}

func loadConfig(configFile string) *config.Config {
	// TODO: support loading from YAML/JSON config file
	if configFile != "" {
		log.Printf("Warning: config file support not yet implemented, using environment variables")
	}

	// Load from environment variables
	cfg := config.LoadFromEnv()

	return cfg
}

func printBanner() {
	banner := `
╔═══════════════════════════════════════════════════════════╗
║                                                           ║
║   __     __        _              ____  ____              ║
║   \ \   / /__  ___| |_ ___  _ __ |  _ \| __ )             ║
║    \ \ / / _ \/ __| __/ _ \| '__|| | | |  _ \             ║
║     \ V /  __/ (__| || (_) | |   | |_| | |_) |            ║
║      \_/ \___|\___|\__\___/|_|   |____/|____/             ║
║                                                           ║
║   Production-Grade Vector Database with HNSW & Hybrid    ║
║   Search                                                  ║
║                                                           ║
╚═══════════════════════════════════════════════════════════╝
`
	fmt.Println(banner)
	fmt.Printf("Version: %s (commit: %s)\n\n", version, commit)
}

func printStartupInfo(cfg *config.Config) {
	fmt.Println("\n╔════════════════════════════════════════════════════════╗")
	fmt.Println("║               Server Configuration                     ║")
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Printf("║ Address:          %-35s ║\n", cfg.Server.Address())
	fmt.Printf("║ TLS Enabled:      %-35v ║\n", cfg.Server.EnableTLS)
	fmt.Printf("║ Max Connections:  %-35d ║\n", cfg.Server.MaxConnections)
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Println("║               HNSW Configuration                       ║")
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Printf("║ M:                %-35d ║\n", cfg.HNSW.M)
	fmt.Printf("║ efConstruction:   %-35d ║\n", cfg.HNSW.EfConstruction)
	fmt.Printf("║ efSearch:         %-35d ║\n", cfg.HNSW.DefaultEfSearch)
	fmt.Printf("║ Dimensions:       %-35d ║\n", cfg.HNSW.Dimensions)
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Println("║               Cache Configuration                      ║")
	fmt.Println("╠════════════════════════════════════════════════════════╣")
	fmt.Printf("║ Enabled:          %-35v ║\n", cfg.Cache.Enabled)
	fmt.Printf("║ Capacity:         %-35d ║\n", cfg.Cache.Capacity)
	fmt.Printf("║ TTL:              %-35s ║\n", cfg.Cache.TTL)
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()
}

func showUsage() {
	fmt.Println("Vector Database Server - Production-grade vector search with HNSW")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  vector-server [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -help             Show this help message")
	fmt.Println("  -version          Show version information")
	fmt.Println("  -config PATH      Path to configuration file (YAML/JSON)")
	fmt.Println("  -host HOST        Server host (default: 0.0.0.0)")
	fmt.Println("  -port PORT        Server port (default: 50051)")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  VECTOR_HOST                Server host")
	fmt.Println("  VECTOR_PORT                Server port")
	fmt.Println("  VECTOR_MAX_CONNECTIONS     Max concurrent connections")
	fmt.Println("  VECTOR_REQUEST_TIMEOUT     Request timeout (e.g., 30s)")
	fmt.Println("  VECTOR_ENABLE_TLS          Enable TLS (true/false)")
	fmt.Println("  VECTOR_TLS_CERT            TLS certificate file")
	fmt.Println("  VECTOR_TLS_KEY             TLS key file")
	fmt.Println("  VECTOR_HNSW_M              HNSW M parameter")
	fmt.Println("  VECTOR_HNSW_EF_CONSTRUCTION HNSW efConstruction")
	fmt.Println("  VECTOR_DIMENSIONS          Vector dimensions")
	fmt.Println("  VECTOR_CACHE_ENABLED       Enable query cache (true/false)")
	fmt.Println("  VECTOR_CACHE_CAPACITY      Cache capacity")
	fmt.Println("  VECTOR_CACHE_TTL           Cache TTL (e.g., 5m)")
	fmt.Println("  VECTOR_DATA_DIR            Data directory path")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Start with default configuration")
	fmt.Println("  vector-server")
	fmt.Println()
	fmt.Println("  # Start on custom port")
	fmt.Println("  vector-server -port 8080")
	fmt.Println()
	fmt.Println("  # Start with environment variables")
	fmt.Println("  VECTOR_PORT=8080 VECTOR_HNSW_M=32 vector-server")
	fmt.Println()
	fmt.Println("  # Start with config file")
	fmt.Println("  vector-server -config config.yaml")
	fmt.Println()
}
