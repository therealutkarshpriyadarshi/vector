package integration

import (
	"context"
	"testing"
	"time"

	grpcserver "github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc"
	"github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
	"github.com/therealutkarshpriyadarshi/vector/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func setupTestServer(t *testing.T) (*grpcserver.Server, proto.VectorDBClient, func()) {
	// Create test configuration
	cfg := config.Default()
	cfg.Server.Port = 50052 // Use different port for testing
	cfg.HNSW.Dimensions = 3  // Small dimensions for testing

	// Create server
	server, err := grpcserver.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Start server
	if err := server.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create client connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "localhost:50052",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		server.Stop()
		t.Fatalf("Failed to connect to server: %v", err)
	}

	client := proto.NewVectorDBClient(conn)

	// Return cleanup function
	cleanup := func() {
		conn.Close()
		server.Stop()
	}

	return server, client, cleanup
}

func TestInsert(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	// Test basic insert
	req := &proto.InsertRequest{
		Namespace: "default",
		Vector:    []float32{0.1, 0.2, 0.3},
		Metadata: map[string]string{
			"title": "Test Document",
			"category": "test",
		},
		Text: stringPtr("This is a test document"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Insert(ctx, req)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if !resp.Success {
		t.Fatalf("Insert returned success=false: %v", resp.Error)
	}

	if resp.Id == "" {
		t.Fatal("Insert returned empty ID")
	}

	t.Logf("Inserted vector with ID: %s", resp.Id)
}

func TestInsertInvalidRequest(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name    string
		req     *proto.InsertRequest
		wantErr bool
	}{
		{
			name: "empty namespace",
			req: &proto.InsertRequest{
				Namespace: "",
				Vector:    []float32{0.1, 0.2, 0.3},
			},
			wantErr: true,
		},
		{
			name: "empty vector",
			req: &proto.InsertRequest{
				Namespace: "default",
				Vector:    []float32{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.Insert(ctx, tt.req)

			if tt.wantErr {
				if err == nil && resp.Success {
					t.Error("Expected error, got success")
				}
			} else {
				if err != nil || !resp.Success {
					t.Errorf("Expected success, got error: %v", err)
				}
			}
		})
	}
}

func TestSearch(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test vectors
	vectors := [][]float32{
		{0.1, 0.2, 0.3},
		{0.2, 0.3, 0.4},
		{0.9, 0.8, 0.7},
	}

	for i, vec := range vectors {
		req := &proto.InsertRequest{
			Namespace: "default",
			Vector:    vec,
			Metadata: map[string]string{
				"index": string(rune('0' + i)),
			},
		}

		if _, err := client.Insert(ctx, req); err != nil {
			t.Fatalf("Failed to insert vector %d: %v", i, err)
		}
	}

	// Wait for indexing
	time.Sleep(100 * time.Millisecond)

	// Search for similar vectors
	searchReq := &proto.SearchRequest{
		Namespace:   "default",
		QueryVector: []float32{0.15, 0.25, 0.35},
		K:           2,
		EfSearch:    50,
	}

	searchResp, err := client.Search(ctx, searchReq)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(searchResp.Results) == 0 {
		t.Fatal("Search returned no results")
	}

	if len(searchResp.Results) > 2 {
		t.Fatalf("Expected at most 2 results, got %d", len(searchResp.Results))
	}

	// Results should be sorted by distance
	for i := 1; i < len(searchResp.Results); i++ {
		if searchResp.Results[i].Distance < searchResp.Results[i-1].Distance {
			t.Error("Results not sorted by distance")
		}
	}

	t.Logf("Found %d results in %.2fms", len(searchResp.Results), searchResp.SearchTimeMs)
}

func TestHybridSearch(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Insert test vectors with text
	testData := []struct {
		vector []float32
		text   string
	}{
		{[]float32{0.1, 0.2, 0.3}, "machine learning and artificial intelligence"},
		{[]float32{0.2, 0.3, 0.4}, "deep neural networks for image recognition"},
		{[]float32{0.9, 0.8, 0.7}, "cooking recipes and food preparation"},
	}

	for i, data := range testData {
		req := &proto.InsertRequest{
			Namespace: "default",
			Vector:    data.vector,
			Text:      &data.text,
			Metadata: map[string]string{
				"index": string(rune('0' + i)),
			},
		}

		if _, err := client.Insert(ctx, req); err != nil {
			t.Fatalf("Failed to insert vector %d: %v", i, err)
		}
	}

	// Wait for indexing
	time.Sleep(100 * time.Millisecond)

	// Hybrid search
	hybridReq := &proto.HybridSearchRequest{
		Namespace:   "default",
		QueryVector: []float32{0.15, 0.25, 0.35},
		QueryText:   "machine learning neural networks",
		K:           2,
		EfSearch:    50,
	}

	hybridResp, err := client.HybridSearch(ctx, hybridReq)
	if err != nil {
		t.Fatalf("Hybrid search failed: %v", err)
	}

	if len(hybridResp.Results) == 0 {
		t.Fatal("Hybrid search returned no results")
	}

	t.Logf("Found %d results in %.2fms", len(hybridResp.Results), hybridResp.SearchTimeMs)
}

func TestDelete(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Insert a vector
	insertReq := &proto.InsertRequest{
		Namespace: "default",
		Vector:    []float32{0.1, 0.2, 0.3},
	}

	insertResp, err := client.Insert(ctx, insertReq)
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	id := insertResp.Id

	// Delete the vector
	deleteReq := &proto.DeleteRequest{
		Namespace: "default",
		Selector:  &proto.DeleteRequest_Id{Id: id},
	}

	deleteResp, err := client.Delete(ctx, deleteReq)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if !deleteResp.Success {
		t.Fatalf("Delete returned success=false: %v", deleteResp.Error)
	}

	if deleteResp.DeletedCount != 1 {
		t.Fatalf("Expected 1 deleted, got %d", deleteResp.DeletedCount)
	}

	t.Logf("Deleted vector %s", id)
}

func TestUpdate(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Insert a vector
	insertReq := &proto.InsertRequest{
		Namespace: "default",
		Vector:    []float32{0.1, 0.2, 0.3},
		Metadata: map[string]string{
			"status": "draft",
		},
	}

	insertResp, err := client.Insert(ctx, insertReq)
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	id := insertResp.Id

	// Update the vector
	updateReq := &proto.UpdateRequest{
		Namespace: "default",
		Id:        id,
		Vector:    []float32{0.2, 0.3, 0.4},
		Metadata: map[string]string{
			"status": "published",
		},
	}

	updateResp, err := client.Update(ctx, updateReq)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !updateResp.Success {
		t.Fatalf("Update returned success=false: %v", updateResp.Error)
	}

	t.Logf("Updated vector %s", id)
}

func TestBatchInsert(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Create batch insert stream
	stream, err := client.BatchInsert(ctx)
	if err != nil {
		t.Fatalf("Failed to create batch insert stream: %v", err)
	}

	// Send multiple vectors
	numVectors := 10
	for i := 0; i < numVectors; i++ {
		req := &proto.InsertRequest{
			Namespace: "default",
			Vector:    []float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3},
			Metadata: map[string]string{
				"batch_index": string(rune('0' + i)),
			},
		}

		if err := stream.Send(req); err != nil {
			t.Fatalf("Failed to send vector %d: %v", i, err)
		}
	}

	// Close stream and get response
	resp, err := stream.CloseAndRecv()
	if err != nil {
		t.Fatalf("Failed to close stream: %v", err)
	}

	if resp.InsertedCount != int32(numVectors) {
		t.Fatalf("Expected %d insertions, got %d", numVectors, resp.InsertedCount)
	}

	if resp.FailedCount != 0 {
		t.Fatalf("Expected 0 failures, got %d: %v", resp.FailedCount, resp.Errors)
	}

	t.Logf("Batch inserted %d vectors in %.2fms", resp.InsertedCount, resp.TotalTimeMs)
}

func TestGetStats(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Insert some vectors
	for i := 0; i < 5; i++ {
		req := &proto.InsertRequest{
			Namespace: "default",
			Vector:    []float32{float32(i) * 0.1, float32(i) * 0.2, float32(i) * 0.3},
		}
		if _, err := client.Insert(ctx, req); err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}
	}

	// Get stats
	statsReq := &proto.StatsRequest{}
	statsResp, err := client.GetStats(ctx, statsReq)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if statsResp.TotalVectors < 5 {
		t.Fatalf("Expected at least 5 vectors, got %d", statsResp.TotalVectors)
	}

	if statsResp.TotalNamespaces < 1 {
		t.Fatal("Expected at least 1 namespace")
	}

	t.Logf("Stats: %d vectors, %d namespaces", statsResp.TotalVectors, statsResp.TotalNamespaces)
}

func TestHealthCheck(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Health check
	healthReq := &proto.HealthCheckRequest{}
	healthResp, err := client.HealthCheck(ctx, healthReq)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if healthResp.Status != "healthy" {
		t.Fatalf("Expected status 'healthy', got '%s'", healthResp.Status)
	}

	if healthResp.Version == "" {
		t.Error("Version is empty")
	}

	t.Logf("Health: %s (version %s, uptime %ds)",
		healthResp.Status, healthResp.Version, healthResp.UptimeSeconds)
}

func TestMultipleNamespaces(t *testing.T) {
	_, client, cleanup := setupTestServer(t)
	defer cleanup()

	ctx := context.Background()

	// Insert vectors in different namespaces
	namespaces := []string{"ns1", "ns2", "ns3"}
	for _, ns := range namespaces {
		req := &proto.InsertRequest{
			Namespace: ns,
			Vector:    []float32{0.1, 0.2, 0.3},
		}
		if _, err := client.Insert(ctx, req); err != nil {
			t.Fatalf("Failed to insert in namespace %s: %v", ns, err)
		}
	}

	// Get stats to verify namespaces
	statsReq := &proto.StatsRequest{}
	statsResp, err := client.GetStats(ctx, statsReq)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if int(statsResp.TotalNamespaces) < len(namespaces) {
		t.Fatalf("Expected at least %d namespaces, got %d",
			len(namespaces), statsResp.TotalNamespaces)
	}

	t.Logf("Created %d namespaces successfully", len(namespaces))
}

func stringPtr(s string) *string {
	return &s
}
