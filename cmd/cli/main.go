package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	version = "1.0.0"
)

var (
	serverAddr  string
	namespace   string
	timeout     time.Duration
)

func main() {
	if len(os.Args) < 2 {
		showUsage()
		os.Exit(1)
	}

	// Global flags
	flag.StringVar(&serverAddr, "server", "localhost:50051", "gRPC server address")
	flag.StringVar(&namespace, "namespace", "default", "namespace to use")
	flag.DurationVar(&timeout, "timeout", 30*time.Second, "request timeout")

	// Parse command
	command := os.Args[1]

	switch command {
	case "insert":
		handleInsert(os.Args[2:])
	case "search":
		handleSearch(os.Args[2:])
	case "hybrid-search":
		handleHybridSearch(os.Args[2:])
	case "delete":
		handleDelete(os.Args[2:])
	case "update":
		handleUpdate(os.Args[2:])
	case "stats":
		handleStats(os.Args[2:])
	case "health":
		handleHealth(os.Args[2:])
	case "version":
		fmt.Printf("vector-cli version %s\n", version)
	case "help", "-h", "--help":
		showUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		showUsage()
		os.Exit(1)
	}
}

func handleInsert(args []string) {
	fs := flag.NewFlagSet("insert", flag.ExitOnError)
	var (
		vectorStr  = fs.String("vector", "", "vector as JSON array (required)")
		metadataStr = fs.String("metadata", "{}", "metadata as JSON object")
		text       = fs.String("text", "", "text content for full-text search")
	)
	fs.StringVar(&serverAddr, "server", serverAddr, "gRPC server address")
	fs.StringVar(&namespace, "namespace", namespace, "namespace")
	fs.Parse(args)

	if *vectorStr == "" {
		fmt.Println("Error: -vector is required")
		fs.Usage()
		os.Exit(1)
	}

	// Parse vector
	var vector []float64
	if err := json.Unmarshal([]byte(*vectorStr), &vector); err != nil {
		fmt.Printf("Error parsing vector: %v\n", err)
		os.Exit(1)
	}

	// Convert to float32
	vector32 := make([]float32, len(vector))
	for i, v := range vector {
		vector32[i] = float32(v)
	}

	// Parse metadata
	var metadata map[string]string
	if err := json.Unmarshal([]byte(*metadataStr), &metadata); err != nil {
		fmt.Printf("Error parsing metadata: %v\n", err)
		os.Exit(1)
	}

	// Connect to server
	client, conn := connectToServer()
	defer conn.Close()

	// Create request
	req := &proto.InsertRequest{
		Namespace: namespace,
		Vector:    vector32,
		Metadata:  metadata,
	}
	if *text != "" {
		req.Text = text
	}

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.Insert(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Insert failed: %s\n", *resp.Error)
		os.Exit(1)
	}

	fmt.Printf("✓ Inserted vector with ID: %s\n", resp.Id)
}

func handleSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	var (
		queryVectorStr = fs.String("query", "", "query vector as JSON array (required)")
		k             = fs.Int("k", 10, "number of results to return")
		efSearch      = fs.Int("ef", 50, "HNSW efSearch parameter")
		showVector    = fs.Bool("show-vector", false, "show vectors in results")
	)
	fs.StringVar(&serverAddr, "server", serverAddr, "gRPC server address")
	fs.StringVar(&namespace, "namespace", namespace, "namespace")
	fs.Parse(args)

	if *queryVectorStr == "" {
		fmt.Println("Error: -query is required")
		fs.Usage()
		os.Exit(1)
	}

	// Parse query vector
	var queryVector []float64
	if err := json.Unmarshal([]byte(*queryVectorStr), &queryVector); err != nil {
		fmt.Printf("Error parsing query vector: %v\n", err)
		os.Exit(1)
	}

	// Convert to float32
	queryVector32 := make([]float32, len(queryVector))
	for i, v := range queryVector {
		queryVector32[i] = float32(v)
	}

	// Connect to server
	client, conn := connectToServer()
	defer conn.Close()

	// Create request
	req := &proto.SearchRequest{
		Namespace:   namespace,
		QueryVector: queryVector32,
		K:           int32(*k),
		EfSearch:    int32(*efSearch),
	}

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.Search(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Display results
	displaySearchResults(resp, *showVector)
}

func handleHybridSearch(args []string) {
	fs := flag.NewFlagSet("hybrid-search", flag.ExitOnError)
	var (
		queryVectorStr = fs.String("query-vector", "", "query vector as JSON array (required)")
		queryText     = fs.String("query-text", "", "query text (required)")
		k             = fs.Int("k", 10, "number of results to return")
		efSearch      = fs.Int("ef", 50, "HNSW efSearch parameter")
		showVector    = fs.Bool("show-vector", false, "show vectors in results")
	)
	fs.StringVar(&serverAddr, "server", serverAddr, "gRPC server address")
	fs.StringVar(&namespace, "namespace", namespace, "namespace")
	fs.Parse(args)

	if *queryVectorStr == "" || *queryText == "" {
		fmt.Println("Error: both -query-vector and -query-text are required")
		fs.Usage()
		os.Exit(1)
	}

	// Parse query vector
	var queryVector []float64
	if err := json.Unmarshal([]byte(*queryVectorStr), &queryVector); err != nil {
		fmt.Printf("Error parsing query vector: %v\n", err)
		os.Exit(1)
	}

	// Convert to float32
	queryVector32 := make([]float32, len(queryVector))
	for i, v := range queryVector {
		queryVector32[i] = float32(v)
	}

	// Connect to server
	client, conn := connectToServer()
	defer conn.Close()

	// Create request
	req := &proto.HybridSearchRequest{
		Namespace:   namespace,
		QueryVector: queryVector32,
		QueryText:   *queryText,
		K:           int32(*k),
		EfSearch:    int32(*efSearch),
	}

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.HybridSearch(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Display results
	displaySearchResults(resp, *showVector)
}

func handleDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	var (
		id = fs.String("id", "", "ID of vector to delete (required)")
	)
	fs.StringVar(&serverAddr, "server", serverAddr, "gRPC server address")
	fs.StringVar(&namespace, "namespace", namespace, "namespace")
	fs.Parse(args)

	if *id == "" {
		fmt.Println("Error: -id is required")
		fs.Usage()
		os.Exit(1)
	}

	// Connect to server
	client, conn := connectToServer()
	defer conn.Close()

	// Create request
	req := &proto.DeleteRequest{
		Namespace: namespace,
		Selector:  &proto.DeleteRequest_Id{Id: *id},
	}

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.Delete(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Delete failed: %s\n", *resp.Error)
		os.Exit(1)
	}

	fmt.Printf("✓ Deleted %d vector(s)\n", resp.DeletedCount)
}

func handleUpdate(args []string) {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	var (
		id          = fs.String("id", "", "ID of vector to update (required)")
		vectorStr   = fs.String("vector", "", "new vector as JSON array")
		metadataStr = fs.String("metadata", "", "new metadata as JSON object")
		text        = fs.String("text", "", "new text content")
	)
	fs.StringVar(&serverAddr, "server", serverAddr, "gRPC server address")
	fs.StringVar(&namespace, "namespace", namespace, "namespace")
	fs.Parse(args)

	if *id == "" {
		fmt.Println("Error: -id is required")
		fs.Usage()
		os.Exit(1)
	}

	// Connect to server
	client, conn := connectToServer()
	defer conn.Close()

	// Create request
	req := &proto.UpdateRequest{
		Namespace: namespace,
		Id:        *id,
	}

	// Parse and add vector if provided
	if *vectorStr != "" {
		var vector []float64
		if err := json.Unmarshal([]byte(*vectorStr), &vector); err != nil {
			fmt.Printf("Error parsing vector: %v\n", err)
			os.Exit(1)
		}
		vector32 := make([]float32, len(vector))
		for i, v := range vector {
			vector32[i] = float32(v)
		}
		req.Vector = vector32
	}

	// Parse and add metadata if provided
	if *metadataStr != "" {
		var metadata map[string]string
		if err := json.Unmarshal([]byte(*metadataStr), &metadata); err != nil {
			fmt.Printf("Error parsing metadata: %v\n", err)
			os.Exit(1)
		}
		req.Metadata = metadata
	}

	// Add text if provided
	if *text != "" {
		req.Text = text
	}

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.Update(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if !resp.Success {
		fmt.Printf("Update failed: %s\n", *resp.Error)
		os.Exit(1)
	}

	fmt.Printf("✓ Updated vector %s\n", *id)
}

func handleStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	fs.StringVar(&serverAddr, "server", serverAddr, "gRPC server address")
	fs.Parse(args)

	// Connect to server
	client, conn := connectToServer()
	defer conn.Close()

	// Create request
	req := &proto.StatsRequest{}

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.GetStats(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Display stats
	fmt.Println("=== Database Statistics ===")
	fmt.Printf("Total Vectors:     %d\n", resp.TotalVectors)
	fmt.Printf("Total Namespaces:  %d\n", resp.TotalNamespaces)
	fmt.Printf("Memory Usage:      %d bytes\n", resp.MemoryUsageBytes)
	fmt.Println("\nNamespace Statistics:")
	for ns, stats := range resp.NamespaceStats {
		fmt.Printf("  %s:\n", ns)
		fmt.Printf("    Vectors:    %d\n", stats.VectorCount)
		fmt.Printf("    Dimensions: %d\n", stats.Dimensions)
		fmt.Printf("    Memory:     %d bytes\n", stats.MemoryBytes)
	}
}

func handleHealth(args []string) {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	fs.StringVar(&serverAddr, "server", serverAddr, "gRPC server address")
	fs.Parse(args)

	// Connect to server
	client, conn := connectToServer()
	defer conn.Close()

	// Create request
	req := &proto.HealthCheckRequest{}

	// Send request
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := client.HealthCheck(ctx, req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Display health status
	fmt.Printf("Status:  %s\n", resp.Status)
	fmt.Printf("Version: %s\n", resp.Version)
	fmt.Printf("Uptime:  %d seconds\n", resp.UptimeSeconds)
	if len(resp.Details) > 0 {
		fmt.Println("Details:")
		for k, v := range resp.Details {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}

	if resp.Status != "healthy" {
		os.Exit(1)
	}
}

func connectToServer() (proto.VectorDBClient, *grpc.ClientConn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		fmt.Printf("Failed to connect to server at %s: %v\n", serverAddr, err)
		os.Exit(1)
	}

	return proto.NewVectorDBClient(conn), conn
}

func displaySearchResults(resp *proto.SearchResponse, showVector bool) {
	if resp.Error != nil && *resp.Error != "" {
		fmt.Printf("Search error: %s\n", *resp.Error)
		os.Exit(1)
	}

	fmt.Printf("Found %d results (search took %.2fms)\n\n", resp.TotalResults, resp.SearchTimeMs)

	if len(resp.Results) == 0 {
		fmt.Println("No results found")
		return
	}

	for i, result := range resp.Results {
		fmt.Printf("Result %d:\n", i+1)
		fmt.Printf("  ID:       %s\n", result.Id)
		fmt.Printf("  Distance: %.6f\n", result.Distance)

		if len(result.Metadata) > 0 {
			fmt.Println("  Metadata:")
			for k, v := range result.Metadata {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}

		if result.Text != nil && *result.Text != "" {
			fmt.Printf("  Text:     %s\n", truncateString(*result.Text, 80))
		}

		if showVector && len(result.Vector) > 0 {
			vectorStr := formatVector(result.Vector)
			fmt.Printf("  Vector:   %s\n", vectorStr)
		}

		fmt.Println()
	}
}

func formatVector(vector []float32) string {
	if len(vector) == 0 {
		return "[]"
	}

	// Show first 5 and last 5 elements if vector is long
	if len(vector) > 10 {
		first := make([]string, 5)
		last := make([]string, 5)
		for i := 0; i < 5; i++ {
			first[i] = fmt.Sprintf("%.4f", vector[i])
			last[i] = fmt.Sprintf("%.4f", vector[len(vector)-5+i])
		}
		return fmt.Sprintf("[%s ... %s] (dim=%d)",
			strings.Join(first, ", "),
			strings.Join(last, ", "),
			len(vector))
	}

	// Show all elements
	elements := make([]string, len(vector))
	for i, v := range vector {
		elements[i] = fmt.Sprintf("%.4f", v)
	}
	return fmt.Sprintf("[%s]", strings.Join(elements, ", "))
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func showUsage() {
	fmt.Println(`Vector Database CLI - Client for vector database gRPC server

Usage:
  vector-cli <command> [options]

Commands:
  insert          Insert a vector with metadata
  search          Search for similar vectors
  hybrid-search   Hybrid search (vector + text)
  delete          Delete a vector by ID
  update          Update a vector
  stats           Get database statistics
  health          Check server health
  version         Show version
  help            Show this help message

Global Options:
  -server ADDRESS   gRPC server address (default: localhost:50051)
  -namespace NAME   Namespace to use (default: default)
  -timeout DURATION Request timeout (default: 30s)

Examples:

  # Insert a vector
  vector-cli insert \
    -vector '[0.1, 0.2, 0.3]' \
    -metadata '{"title": "Document 1", "category": "tech"}' \
    -text "This is a test document"

  # Search for similar vectors
  vector-cli search \
    -query '[0.15, 0.25, 0.35]' \
    -k 10 \
    -ef 50

  # Hybrid search (vector + text)
  vector-cli hybrid-search \
    -query-vector '[0.1, 0.2, 0.3]' \
    -query-text "machine learning" \
    -k 10

  # Delete a vector
  vector-cli delete -id 12345

  # Update a vector
  vector-cli update \
    -id 12345 \
    -metadata '{"category": "updated"}' \
    -text "Updated text"

  # Get database statistics
  vector-cli stats

  # Check server health
  vector-cli health

  # Use custom server and namespace
  vector-cli search \
    -server my-server:50051 \
    -namespace production \
    -query '[0.1, 0.2]'

For more information, visit: https://github.com/therealutkarshpriyadarshi/vector`)
}

func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}
