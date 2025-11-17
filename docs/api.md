# Vector Database API Reference

Complete gRPC API documentation for the Vector Database.

## Table of Contents

- [Overview](#overview)
- [Connection](#connection)
- [Authentication](#authentication)
- [API Endpoints](#api-endpoints)
  - [Insert](#insert)
  - [Search](#search)
  - [HybridSearch](#hybridsearch)
  - [BatchInsert](#batchinsert)
  - [Update](#update)
  - [Delete](#delete)
  - [GetStats](#getstats)
  - [HealthCheck](#healthcheck)
- [Data Types](#data-types)
- [Filters](#filters)
- [Error Handling](#error-handling)
- [Code Examples](#code-examples)

---

## Overview

The Vector Database provides a gRPC API for high-performance vector similarity search with hybrid search capabilities. All operations support multi-tenancy through namespaces.

**Default Endpoint**: `localhost:50051`
**Protocol**: gRPC with Protocol Buffers
**Version**: 1.0.0

---

## Connection

### Go Client

```go
import (
    "google.golang.org/grpc"
    "github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
)

conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

client := proto.NewVectorDBClient(conn)
```

### Python Client

```python
import grpc
from proto import vector_pb2, vector_pb2_grpc

channel = grpc.insecure_channel('localhost:50051')
client = vector_pb2_grpc.VectorDBStub(channel)
```

### With TLS

```go
creds, err := credentials.NewClientTLSFromFile("server.crt", "")
conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(creds))
```

---

## Authentication

Currently supports:
- **Namespace isolation**: Multi-tenant data separation
- **TLS**: Encrypted connections (optional)

Future releases will include:
- API key authentication
- OAuth 2.0 integration
- Role-based access control (RBAC)

---

## API Endpoints

### Insert

Insert a single vector with metadata.

**RPC**: `Insert(InsertRequest) returns (InsertResponse)`

**Request**:
```protobuf
message InsertRequest {
  string namespace = 1;              // Namespace (default: "default")
  repeated float vector = 2;         // Vector embedding (required)
  map<string, string> metadata = 3;  // Metadata key-value pairs
  optional string id = 4;            // Custom ID (auto-generated if not provided)
  optional string text = 5;          // Text content for full-text search
}
```

**Response**:
```protobuf
message InsertResponse {
  string id = 1;         // ID of inserted vector
  bool success = 2;      // Operation success status
  optional string error = 3;  // Error message if failed
}
```

**Example**:
```go
resp, err := client.Insert(ctx, &proto.InsertRequest{
    Namespace: "default",
    Vector:    []float32{0.1, 0.2, 0.3, ...},
    Metadata: map[string]string{
        "title": "Vector Database Tutorial",
        "category": "tech",
        "date": "2025-01-15",
    },
    Text: "Learn how to build a vector database with HNSW",
})

if err != nil {
    log.Fatal(err)
}
fmt.Printf("Inserted vector with ID: %s\n", resp.Id)
```

**Performance**:
- Latency: ~4.5ms per vector
- Throughput: ~200 inserts/sec (single-threaded)

---

### Search

Search for k nearest neighbors using vector similarity.

**RPC**: `Search(SearchRequest) returns (SearchResponse)`

**Request**:
```protobuf
message SearchRequest {
  string namespace = 1;              // Namespace to search in
  repeated float query_vector = 2;   // Query vector (required)
  int32 k = 3;                       // Number of results (required)
  int32 ef_search = 4;               // HNSW ef_search (default: 50)
  optional Filter filter = 5;        // Metadata filter
  optional string distance_metric = 6; // "cosine", "euclidean", "dot_product"
}
```

**Response**:
```protobuf
message SearchResponse {
  repeated SearchResult results = 1;  // Search results
  int32 total_results = 2;           // Total results found
  float search_time_ms = 3;          // Search time in ms
  optional string error = 4;         // Error message if failed
}

message SearchResult {
  string id = 1;                     // Vector ID
  float distance = 2;                // Distance/similarity score
  repeated float vector = 3;         // Original vector
  map<string, string> metadata = 4;  // Metadata
  optional string text = 5;          // Text content if available
}
```

**Example**:
```go
resp, err := client.Search(ctx, &proto.SearchRequest{
    Namespace:   "default",
    QueryVector: queryVector,
    K:           10,
    EfSearch:    100, // Higher = more accurate, slower
    Filter: &proto.Filter{
        FilterType: &proto.Filter_Comparison{
            Comparison: &proto.ComparisonFilter{
                Field:    "category",
                Operator: "eq",
                Value:    "tech",
            },
        },
    },
    DistanceMetric: "cosine",
})

for _, result := range resp.Results {
    fmt.Printf("ID: %s, Distance: %.4f, Title: %s\n",
        result.Id, result.Distance, result.Metadata["title"])
}
```

**Performance** (1M vectors, 768 dims):
- p50 latency: 3.2ms
- p95 latency: 8.5ms
- p99 latency: 12.1ms
- Recall@10: 96.5%

**ef_search Tuning**:
- `ef_search=10`: Fast but lower recall (~85%)
- `ef_search=50`: Balanced (default) (~95% recall)
- `ef_search=100`: Accurate but slower (~98% recall)
- `ef_search=200`: Very accurate, 2-3x slower (~99.5% recall)

---

### HybridSearch

Combine vector similarity and full-text search using Reciprocal Rank Fusion (RRF).

**RPC**: `HybridSearch(HybridSearchRequest) returns (SearchResponse)`

**Request**:
```protobuf
message HybridSearchRequest {
  string namespace = 1;              // Namespace to search in
  repeated float query_vector = 2;   // Query vector (required)
  string query_text = 3;             // Query text (required)
  int32 k = 4;                       // Number of results
  int32 ef_search = 5;               // HNSW ef_search
  optional Filter filter = 6;        // Metadata filter
  optional HybridSearchConfig config = 7; // Fusion config
}

message HybridSearchConfig {
  string fusion_method = 1;  // "rrf" (default) or "weighted"
  float vector_weight = 2;   // Weight for vector results (0.0-1.0)
  float text_weight = 3;     // Weight for text results (0.0-1.0)
  int32 rrf_k = 4;          // RRF k parameter (default: 60)
}
```

**Example**:
```go
resp, err := client.HybridSearch(ctx, &proto.HybridSearchRequest{
    Namespace:   "default",
    QueryVector: queryVector,
    QueryText:   "machine learning vector database",
    K:           20,
    EfSearch:    100,
    Config: &proto.HybridSearchConfig{
        FusionMethod: "rrf",
        VectorWeight: 0.7,  // Favor vector similarity
        TextWeight:   0.3,
        RrfK:         60,
    },
})

for _, result := range resp.Results {
    fmt.Printf("ID: %s, Distance: %.4f, VectorScore: %.4f, TextScore: %.4f\n",
        result.Id, result.Distance,
        result.VectorScore, result.TextScore)
}
```

**Use Cases**:
- Semantic search with keyword filtering
- RAG (Retrieval-Augmented Generation) systems
- Document retrieval with metadata filtering
- Recommendation systems with text context

**Performance**:
- Latency: 1.5-2x vector-only search
- Cache hit speedup: 2-5x on repeated queries
- Recall improvement: +5-15% over vector-only

---

### BatchInsert

Insert multiple vectors efficiently using streaming.

**RPC**: `BatchInsert(stream InsertRequest) returns (BatchInsertResponse)`

**Response**:
```protobuf
message BatchInsertResponse {
  int32 inserted_count = 1;      // Successful insertions
  int32 failed_count = 2;        // Failed insertions
  repeated string inserted_ids = 3; // IDs of inserted vectors
  repeated string errors = 4;    // Error messages
  float total_time_ms = 5;       // Total time in ms
}
```

**Example**:
```go
stream, err := client.BatchInsert(ctx)
if err != nil {
    log.Fatal(err)
}

// Send vectors
for _, vec := range vectors {
    err := stream.Send(&proto.InsertRequest{
        Namespace: "default",
        Vector:    vec.Embedding,
        Metadata:  vec.Metadata,
        Text:      vec.Text,
    })
    if err != nil {
        log.Printf("Failed to send: %v", err)
    }
}

// Close stream and get response
resp, err := stream.CloseAndRecv()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Inserted %d vectors in %.2fms (%.2f vectors/sec)\n",
    resp.InsertedCount, resp.TotalTimeMs,
    float64(resp.InsertedCount)/(resp.TotalTimeMs/1000))
```

**Performance**:
- Speedup: 4.5x faster than individual inserts
- Throughput: ~900 vectors/sec
- Memory: Buffers 100 vectors at a time

**Best Practices**:
- Batch size: 100-1000 vectors optimal
- Use for initial data loading
- Enable error handling for partial failures

---

### Update

Update an existing vector's embedding or metadata.

**RPC**: `Update(UpdateRequest) returns (UpdateResponse)`

**Request**:
```protobuf
message UpdateRequest {
  string namespace = 1;              // Namespace
  string id = 2;                     // Vector ID to update (required)
  repeated float vector = 3;         // New vector (empty if not updating)
  map<string, string> metadata = 4;  // New metadata (empty if not updating)
  optional string text = 5;          // New text content
}
```

**Example**:
```go
// Update metadata only
resp, err := client.Update(ctx, &proto.UpdateRequest{
    Namespace: "default",
    Id:        "12345",
    Metadata: map[string]string{
        "status": "published",
        "updated_at": "2025-01-15T10:30:00Z",
    },
})

// Update vector and metadata
resp, err := client.Update(ctx, &proto.UpdateRequest{
    Namespace: "default",
    Id:        "12345",
    Vector:    newEmbedding,
    Metadata:  updatedMetadata,
    Text:      "Updated content",
})
```

**Note**: Updating the vector triggers HNSW graph reconstruction for that node.

---

### Delete

Delete vectors by ID or filter.

**RPC**: `Delete(DeleteRequest) returns (DeleteResponse)`

**Request**:
```protobuf
message DeleteRequest {
  string namespace = 1;
  oneof selector {
    string id = 2;         // Delete by ID
    Filter filter = 3;     // Delete by filter
  }
}
```

**Response**:
```protobuf
message DeleteResponse {
  int32 deleted_count = 1;   // Number of vectors deleted
  bool success = 2;          // Operation success
  optional string error = 3; // Error message if failed
}
```

**Example**:
```go
// Delete by ID
resp, err := client.Delete(ctx, &proto.DeleteRequest{
    Namespace: "default",
    Selector: &proto.DeleteRequest_Id{
        Id: "12345",
    },
})

// Delete by filter (bulk delete)
resp, err := client.Delete(ctx, &proto.DeleteRequest{
    Namespace: "default",
    Selector: &proto.DeleteRequest_Filter{
        Filter: &proto.Filter{
            FilterType: &proto.Filter_Comparison{
                Comparison: &proto.ComparisonFilter{
                    Field:    "status",
                    Operator: "eq",
                    Value:    "expired",
                },
            },
        },
    },
})

fmt.Printf("Deleted %d vectors\n", resp.DeletedCount)
```

---

### GetStats

Retrieve database statistics.

**RPC**: `GetStats(StatsRequest) returns (StatsResponse)`

**Example**:
```go
resp, err := client.GetStats(ctx, &proto.StatsRequest{
    Namespace: "", // Empty = all namespaces
})

fmt.Printf("Total vectors: %d\n", resp.TotalVectors)
fmt.Printf("Total namespaces: %d\n", resp.TotalNamespaces)
fmt.Printf("Memory usage: %.2f MB\n", float64(resp.MemoryUsageBytes)/(1024*1024))

for ns, stats := range resp.NamespaceStats {
    fmt.Printf("Namespace '%s': %d vectors, %d dimensions\n",
        ns, stats.VectorCount, stats.Dimensions)
}
```

---

### HealthCheck

Check server health status.

**RPC**: `HealthCheck(HealthCheckRequest) returns (HealthCheckResponse)`

**Response**:
```protobuf
message HealthCheckResponse {
  string status = 1;              // "healthy", "degraded", "unhealthy"
  string version = 2;             // Server version
  int64 uptime_seconds = 3;       // Server uptime
  map<string, string> details = 4; // Additional details
}
```

**Example**:
```go
resp, err := client.HealthCheck(ctx, &proto.HealthCheckRequest{})
fmt.Printf("Status: %s, Version: %s, Uptime: %ds\n",
    resp.Status, resp.Version, resp.UptimeSeconds)
```

---

## Data Types

### Vector Format

Vectors must be:
- Type: `float32[]`
- Dimensions: Consistent within namespace (default: 768)
- Normalized: Recommended for cosine similarity
- Range: Typically [-1, 1] or [0, 1]

**Supported Dimensions**:
- Small: 128, 256, 384
- Medium: 512, 768 (default)
- Large: 1024, 1536
- Very Large: 2048, 4096

### Metadata

Metadata fields support:
- **String**: Any text value
- **Numeric**: Stored as string, parsed for comparisons
- **Boolean**: "true" or "false"
- **Date**: ISO 8601 format recommended
- **Geographic**: Format: `"lat,lon"` (e.g., "37.7749,-122.4194")

**Reserved Fields**:
- `_id`: Vector ID (auto-generated)
- `_namespace`: Namespace name
- `_created_at`: Creation timestamp
- `_updated_at`: Last update timestamp

---

## Filters

### Comparison Filter

Equality and inequality checks.

```go
&proto.Filter{
    FilterType: &proto.Filter_Comparison{
        Comparison: &proto.ComparisonFilter{
            Field:    "category",
            Operator: "eq",  // eq, ne, gt, lt, gte, lte
            Value:    "tech",
        },
    },
}
```

**Operators**:
- `eq`: Equal
- `ne`: Not equal
- `gt`: Greater than (numeric)
- `lt`: Less than (numeric)
- `gte`: Greater than or equal
- `lte`: Less than or equal

### Range Filter

Numeric range queries.

```go
&proto.Filter{
    FilterType: &proto.Filter_Range{
        Range: &proto.RangeFilter{
            Field: "price",
            Gte:   "10",
            Lte:   "100",
        },
    },
}
```

### List Filter

IN / NOT IN operations.

```go
&proto.Filter{
    FilterType: &proto.Filter_List{
        List: &proto.ListFilter{
            Field:    "tags",
            Operator: "in",  // "in" or "not_in"
            Values:   []string{"golang", "database", "vector"},
        },
    },
}
```

### Geo Radius Filter

Geographic radius queries.

```go
&proto.Filter{
    FilterType: &proto.Filter_GeoRadius{
        GeoRadius: &proto.GeoRadiusFilter{
            Field:     "location",
            Latitude:  37.7749,   // San Francisco
            Longitude: -122.4194,
            RadiusKm:  50.0,      // 50km radius
        },
    },
}
```

**Metadata Format**: Store as `"lat,lon"`:
```go
Metadata: map[string]string{
    "location": "37.7749,-122.4194",
}
```

### Composite Filter

Combine multiple filters with AND, OR, NOT.

```go
&proto.Filter{
    FilterType: &proto.Filter_Composite{
        Composite: &proto.CompositeFilter{
            Operator: "and",  // "and", "or", "not"
            Filters: []*proto.Filter{
                {FilterType: &proto.Filter_Comparison{...}},
                {FilterType: &proto.Filter_Range{...}},
            },
        },
    },
}
```

**Example** (category = "tech" AND price >= 10 AND price <= 100):
```go
&proto.Filter{
    FilterType: &proto.Filter_Composite{
        Composite: &proto.CompositeFilter{
            Operator: "and",
            Filters: []*proto.Filter{
                {FilterType: &proto.Filter_Comparison{
                    Comparison: &proto.ComparisonFilter{
                        Field: "category", Operator: "eq", Value: "tech",
                    },
                }},
                {FilterType: &proto.Filter_Range{
                    Range: &proto.RangeFilter{
                        Field: "price", Gte: "10", Lte: "100",
                    },
                }},
            },
        },
    },
}
```

---

## Error Handling

### Status Codes

gRPC status codes returned:
- `OK` (0): Success
- `INVALID_ARGUMENT` (3): Invalid request parameters
- `NOT_FOUND` (5): Vector or namespace not found
- `ALREADY_EXISTS` (6): Duplicate ID
- `RESOURCE_EXHAUSTED` (8): Quota exceeded
- `INTERNAL` (13): Server error
- `UNAVAILABLE` (14): Server unavailable

### Error Response

Errors are returned in the response message:
```go
if resp.Error != nil && *resp.Error != "" {
    log.Printf("Error: %s", *resp.Error)
}
```

### Retry Strategy

```go
import "google.golang.org/grpc/codes"
import "google.golang.org/grpc/status"

func shouldRetry(err error) bool {
    st, ok := status.FromError(err)
    if !ok {
        return false
    }

    switch st.Code() {
    case codes.Unavailable, codes.ResourceExhausted:
        return true
    default:
        return false
    }
}

// Exponential backoff
for attempt := 0; attempt < maxRetries; attempt++ {
    resp, err := client.Search(ctx, req)
    if err == nil {
        return resp, nil
    }

    if !shouldRetry(err) {
        return nil, err
    }

    time.Sleep(time.Duration(math.Pow(2, float64(attempt))) * time.Second)
}
```

---

## Code Examples

### Complete RAG System

```go
package main

import (
    "context"
    "fmt"
    "log"

    "google.golang.org/grpc"
    pb "github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
)

func main() {
    // Connect
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := pb.NewVectorDBClient(conn)
    ctx := context.Background()

    // 1. Index documents
    documents := []struct {
        Text      string
        Embedding []float32
        Metadata  map[string]string
    }{
        {
            Text:      "HNSW is a graph-based ANN algorithm",
            Embedding: getEmbedding("HNSW is a graph-based ANN algorithm"),
            Metadata:  map[string]string{"source": "docs", "topic": "algorithms"},
        },
        // ... more documents
    }

    stream, _ := client.BatchInsert(ctx)
    for _, doc := range documents {
        stream.Send(&pb.InsertRequest{
            Namespace: "knowledge_base",
            Vector:    doc.Embedding,
            Text:      doc.Text,
            Metadata:  doc.Metadata,
        })
    }
    resp, _ := stream.CloseAndRecv()
    fmt.Printf("Indexed %d documents\n", resp.InsertedCount)

    // 2. Hybrid search for RAG
    query := "How does HNSW algorithm work?"
    queryEmbedding := getEmbedding(query)

    searchResp, err := client.HybridSearch(ctx, &pb.HybridSearchRequest{
        Namespace:   "knowledge_base",
        QueryVector: queryEmbedding,
        QueryText:   query,
        K:           5,
        EfSearch:    100,
        Config: &pb.HybridSearchConfig{
            FusionMethod: "rrf",
            VectorWeight: 0.7,
            TextWeight:   0.3,
        },
    })

    if err != nil {
        log.Fatal(err)
    }

    // 3. Generate response with retrieved context
    context := ""
    for i, result := range searchResp.Results {
        context += fmt.Sprintf("%d. %s\n", i+1, *result.Text)
    }

    response := generateWithLLM(query, context)
    fmt.Printf("Answer: %s\n", response)
}

func getEmbedding(text string) []float32 {
    // Use your embedding model (OpenAI, Cohere, etc.)
    return []float32{...}
}

func generateWithLLM(query, context string) string {
    // Use your LLM (GPT-4, Claude, etc.)
    prompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s\n\nAnswer:", context, query)
    return callLLM(prompt)
}
```

### Semantic Search with Filters

```go
func semanticSearch(client pb.VectorDBClient, query string, filters map[string]string) {
    queryEmbedding := getEmbedding(query)

    // Build composite filter
    var filterProtos []*pb.Filter
    for field, value := range filters {
        filterProtos = append(filterProtos, &pb.Filter{
            FilterType: &pb.Filter_Comparison{
                Comparison: &pb.ComparisonFilter{
                    Field:    field,
                    Operator: "eq",
                    Value:    value,
                },
            },
        })
    }

    compositeFilter := &pb.Filter{
        FilterType: &pb.Filter_Composite{
            Composite: &pb.CompositeFilter{
                Operator: "and",
                Filters:  filterProtos,
            },
        },
    }

    resp, err := client.Search(context.Background(), &pb.SearchRequest{
        Namespace:   "default",
        QueryVector: queryEmbedding,
        K:           10,
        EfSearch:    100,
        Filter:      compositeFilter,
    })

    if err != nil {
        log.Fatal(err)
    }

    for _, result := range resp.Results {
        fmt.Printf("%.4f - %s\n", result.Distance, result.Metadata["title"])
    }
}
```

### Real-time Updates

```go
func updateDocumentEmbedding(client pb.VectorDBClient, docID, newText string) {
    // Generate new embedding
    newEmbedding := getEmbedding(newText)

    // Update vector and metadata
    _, err := client.Update(context.Background(), &pb.UpdateRequest{
        Namespace: "documents",
        Id:        docID,
        Vector:    newEmbedding,
        Text:      newText,
        Metadata: map[string]string{
            "updated_at": time.Now().Format(time.RFC3339),
            "version":    "2",
        },
    })

    if err != nil {
        log.Printf("Failed to update %s: %v", docID, err)
    }
}
```

---

## Performance Tips

1. **Batch Operations**: Use `BatchInsert` for bulk loading (4.5x faster)
2. **Tune ef_search**: Balance speed vs accuracy
   - Production: 50-100
   - High accuracy: 150-200
   - Real-time: 20-30
3. **Use Caching**: Enable query cache for repeated searches (2-5x speedup)
4. **Metadata Filters**: Apply filters before vector search when possible
5. **Connection Pooling**: Reuse gRPC connections
6. **Namespace Isolation**: Separate large datasets into namespaces
7. **Monitoring**: Use `GetStats` to track performance

---

## Next Steps

- [Deployment Guide](deployment.md) - Production deployment
- [Algorithms](algorithms.md) - HNSW and NSG deep dive
- [Benchmarks](benchmarks.md) - Performance testing
- [Troubleshooting](troubleshooting.md) - Common issues

---

**Version**: 1.1.0
**Last Updated**: 2025-01-15
