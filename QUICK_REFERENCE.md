# Quick Reference Guide

Quick reference for common patterns, commands, and code snippets while building the vector database.

---

## 📦 Common Commands

### Development
```bash
# Setup
make init                    # Initialize project
go mod tidy                  # Update dependencies

# Build
make build                   # Build server
make build-cli               # Build CLI client
go build -o bin/server cmd/server/main.go

# Test
make test                    # Run unit tests
make bench                   # Run benchmarks
make integration             # Run integration tests
make recall-test             # Test HNSW recall accuracy
make coverage                # Generate coverage report

# Run
make run                     # Start server
./bin/vector-server          # Run server binary
./bin/vector-cli search --help

# Profile
make profile-cpu             # CPU profiling
make profile-mem             # Memory profiling
go test -cpuprofile=cpu.prof -bench=.
go tool pprof -http=:8080 cpu.prof

# Lint & Format
make fmt                     # Format code
make vet                     # Run go vet
make lint                    # Run linter
make check                   # Run all checks

# Clean
make clean                   # Remove build artifacts
```

---

## 🧮 Core Algorithms

### Distance Metrics

```go
// Cosine Similarity (for normalized vectors)
func CosineSimilarity(a, b []float32) float32 {
    var dot, normA, normB float32
    for i := range a {
        dot += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }
    return 1.0 - (dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// Euclidean Distance
func EuclideanDistance(a, b []float32) float32 {
    var sum float32
    for i := range a {
        diff := a[i] - b[i]
        sum += diff * diff
    }
    return math.Sqrt(sum)
}

// Dot Product (for pre-normalized vectors)
func DotProduct(a, b []float32) float32 {
    var dot float32
    for i := range a {
        dot += a[i] * b[i]
    }
    return -dot // Negative for similarity
}
```

### HNSW Layer Assignment

```go
// Assign random layer with exponential decay
func (idx *Index) randomLayer() int {
    level := 0
    ml := 1.0 / math.Log(float64(idx.M))
    for rand.Float64() < ml && level < 16 {
        level++
    }
    return level
}
```

### HNSW Insert (Pseudocode)

```go
func (idx *Index) Insert(vec []float32, meta map[string]interface{}) uint64 {
    // 1. Create node with random level
    level := idx.randomLevel()
    node := &Node{ID: nextID(), Vector: vec, Layers: make([][]uint64, level+1)}

    // 2. If first node, set as entry point
    if idx.entryPoint == nil {
        idx.entryPoint = node
        return node.ID
    }

    // 3. Search from top layer to find insertion point
    ep := idx.entryPoint
    for lc := idx.maxLayer; lc > level; lc-- {
        ep = idx.searchLayer(vec, ep, 1, lc)[0]
    }

    // 4. Insert at each layer from level down to 0
    for lc := level; lc >= 0; lc-- {
        candidates := idx.searchLayer(vec, ep, idx.efConstruction, lc)
        neighbors := idx.selectNeighbors(candidates, idx.M)

        // Add bidirectional links
        node.Layers[lc] = neighbors
        for _, nID := range neighbors {
            idx.nodes[nID].AddNeighbor(lc, node.ID)
        }
    }

    return node.ID
}
```

### HNSW Search (Pseudocode)

```go
func (idx *Index) Search(query []float32, k int, efSearch int) []Result {
    ep := idx.entryPoint

    // Traverse down from top layer
    for lc := idx.maxLayer; lc > 0; lc-- {
        ep = idx.searchLayer(query, ep, 1, lc)[0]
    }

    // Search at layer 0
    candidates := idx.searchLayer(query, ep, efSearch, 0)

    // Return top K
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].Distance < candidates[j].Distance
    })

    if len(candidates) > k {
        candidates = candidates[:k]
    }

    return candidates
}
```

### Reciprocal Rank Fusion

```go
func ReciprocalRankFusion(vectorResults, textResults []Result, k int) []Result {
    scores := make(map[string]float64)

    // RRF formula: score = Σ(1 / (rank + 60))
    for rank, res := range vectorResults {
        scores[res.ID] += 1.0 / float64(rank+60)
    }

    for rank, res := range textResults {
        scores[res.ID] += 1.0 / float64(rank+60)
    }

    // Sort by combined score
    var merged []Result
    for id, score := range scores {
        merged = append(merged, Result{ID: id, Score: score})
    }

    sort.Slice(merged, func(i, j int) bool {
        return merged[i].Score > merged[j].Score
    })

    return merged[:min(k, len(merged))]
}
```

---

## 🔒 Concurrency Patterns

### Read-Write Lock for Index

```go
type Index struct {
    nodes map[uint64]*Node
    mu    sync.RWMutex
}

// Read operation
func (idx *Index) Search(query []float32, k int) []Result {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    // ... search logic
}

// Write operation
func (idx *Index) Insert(vec []float32) uint64 {
    idx.mu.Lock()
    defer idx.mu.Unlock()
    // ... insert logic
}
```

### Fine-Grained Locking for Nodes

```go
type Node struct {
    ID     uint64
    Vector []float32
    Layers [][]uint64
    mu     sync.RWMutex
}

func (n *Node) AddNeighbor(layer int, id uint64) {
    n.mu.Lock()
    defer n.mu.Unlock()
    n.Layers[layer] = append(n.Layers[layer], id)
}

func (n *Node) GetNeighbors(layer int) []uint64 {
    n.mu.RLock()
    defer n.mu.RUnlock()
    return n.Layers[layer]
}
```

### Parallel Search

```go
func (idx *Index) BatchSearch(queries [][]float32, k int) [][]Result {
    results := make([][]Result, len(queries))
    var wg sync.WaitGroup

    for i, query := range queries {
        wg.Add(1)
        go func(i int, q []float32) {
            defer wg.Done()
            results[i] = idx.Search(q, k, 50)
        }(i, query)
    }

    wg.Wait()
    return results
}
```

### Worker Pool Pattern

```go
type WorkerPool struct {
    workers int
    tasks   chan func()
}

func NewWorkerPool(workers int) *WorkerPool {
    pool := &WorkerPool{
        workers: workers,
        tasks:   make(chan func(), 100),
    }

    for i := 0; i < workers; i++ {
        go pool.worker()
    }

    return pool
}

func (p *WorkerPool) worker() {
    for task := range p.tasks {
        task()
    }
}

func (p *WorkerPool) Submit(task func()) {
    p.tasks <- task
}
```

---

## 💾 Storage Patterns

### BadgerDB Basic Operations

```go
import "github.com/dgraph-io/badger/v4"

// Initialize
db, err := badger.Open(badger.DefaultOptions("/tmp/badger"))
defer db.Close()

// Write
err = db.Update(func(txn *badger.Txn) error {
    return txn.Set([]byte("key"), []byte("value"))
})

// Read
err = db.View(func(txn *badger.Txn) error {
    item, err := txn.Get([]byte("key"))
    if err != nil {
        return err
    }

    return item.Value(func(val []byte) error {
        fmt.Println(string(val))
        return nil
    })
})

// Iterate
err = db.View(func(txn *badger.Txn) error {
    it := txn.NewIterator(badger.DefaultIteratorOptions)
    defer it.Close()

    for it.Rewind(); it.Valid(); it.Next() {
        item := it.Item()
        key := item.Key()
        // Process key
    }
    return nil
})

// Batch Write
wb := db.NewWriteBatch()
defer wb.Cancel()

for _, item := range items {
    wb.Set([]byte(item.Key), []byte(item.Value))
}

err = wb.Flush()
```

### Serialization with encoding/gob

```go
import (
    "bytes"
    "encoding/gob"
)

// Encode
func EncodeVector(vec []float32, meta map[string]interface{}) ([]byte, error) {
    var buf bytes.Buffer
    enc := gob.NewEncoder(&buf)

    data := struct {
        Vector   []float32
        Metadata map[string]interface{}
    }{
        Vector:   vec,
        Metadata: meta,
    }

    if err := enc.Encode(data); err != nil {
        return nil, err
    }

    return buf.Bytes(), nil
}

// Decode
func DecodeVector(data []byte) ([]float32, map[string]interface{}, error) {
    buf := bytes.NewBuffer(data)
    dec := gob.NewDecoder(buf)

    var result struct {
        Vector   []float32
        Metadata map[string]interface{}
    }

    if err := dec.Decode(&result); err != nil {
        return nil, nil, err
    }

    return result.Vector, result.Metadata, nil
}
```

### Namespace Key Pattern

```go
// Key format: {namespace}/vec/{id}
func vectorKey(namespace, id string) []byte {
    return []byte(fmt.Sprintf("%s/vec/%s", namespace, id))
}

// Key format: {namespace}/hnsw/{layer}/{node_id}
func hnswKey(namespace string, layer int, nodeID uint64) []byte {
    return []byte(fmt.Sprintf("%s/hnsw/%d/%d", namespace, layer, nodeID))
}

// Iterate namespace
func (s *Store) IterateNamespace(namespace string, fn func(key, val []byte) error) error {
    prefix := []byte(namespace + "/")

    return s.db.View(func(txn *badger.Txn) error {
        it := txn.NewIterator(badger.DefaultIteratorOptions)
        defer it.Close()

        for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
            item := it.Item()
            key := item.Key()

            err := item.Value(func(val []byte) error {
                return fn(key, val)
            })

            if err != nil {
                return err
            }
        }
        return nil
    })
}
```

---

## 🌐 gRPC Patterns

### Server Setup

```go
import (
    "google.golang.org/grpc"
    "net"
)

// Create server
s := grpc.NewServer(
    grpc.MaxRecvMsgSize(10 * 1024 * 1024), // 10MB
    grpc.MaxSendMsgSize(10 * 1024 * 1024),
)

// Register service
pb.RegisterVectorDBServer(s, &vectorServer{})

// Listen
lis, err := net.Listen("tcp", ":9000")
if err != nil {
    log.Fatal(err)
}

// Serve
if err := s.Serve(lis); err != nil {
    log.Fatal(err)
}
```

### Client Setup

```go
// Connect
conn, err := grpc.Dial("localhost:9000", grpc.WithInsecure())
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// Create client
client := pb.NewVectorDBClient(conn)

// Call
resp, err := client.Insert(context.Background(), &pb.InsertRequest{
    Namespace: "default",
    Vector:    []float32{0.1, 0.2, 0.3},
    Metadata:  map[string]string{"title": "doc1"},
})
```

### Streaming

```go
// Server-side streaming
func (s *server) StreamSearch(req *pb.SearchRequest, stream pb.VectorDB_StreamSearchServer) error {
    results := s.index.Search(req.Query, req.K, 50)

    for _, res := range results {
        if err := stream.Send(&pb.SearchResult{
            Id:       res.ID,
            Distance: res.Distance,
        }); err != nil {
            return err
        }
    }

    return nil
}

// Client-side streaming
func (s *server) BatchInsert(stream pb.VectorDB_BatchInsertServer) error {
    count := 0

    for {
        req, err := stream.Recv()
        if err == io.EOF {
            return stream.SendAndClose(&pb.BatchInsertResponse{
                Count: int32(count),
            })
        }
        if err != nil {
            return err
        }

        s.index.Insert(req.Vector, req.Metadata)
        count++
    }
}
```

---

## 🧪 Testing Patterns

### Table-Driven Tests

```go
func TestCosineSimilarity(t *testing.T) {
    tests := []struct {
        name     string
        a, b     []float32
        expected float32
        delta    float32
    }{
        {
            name:     "identical vectors",
            a:        []float32{1, 0, 0},
            b:        []float32{1, 0, 0},
            expected: 0.0,
            delta:    0.001,
        },
        {
            name:     "orthogonal vectors",
            a:        []float32{1, 0, 0},
            b:        []float32{0, 1, 0},
            expected: 1.0,
            delta:    0.001,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := CosineSimilarity(tt.a, tt.b)
            assert.InDelta(t, tt.expected, result, tt.delta)
        })
    }
}
```

### Benchmarks

```go
func BenchmarkHNSW_Insert(b *testing.B) {
    idx := hnsw.New(16, 200)
    vec := make([]float32, 768)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        idx.Insert(vec, nil)
    }
}

func BenchmarkHNSW_Search(b *testing.B) {
    idx := buildIndex(10000) // Helper to build index
    query := make([]float32, 768)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        idx.Search(query, 10, 50)
    }
}
```

### Recall Testing

```go
func TestRecall(t *testing.T) {
    // Generate test data
    vectors := generateRandomVectors(1000, 128)
    idx := hnsw.New(16, 200)

    // Build index
    for _, vec := range vectors {
        idx.Insert(vec, nil)
    }

    // Test queries
    totalRecall := 0.0
    numQueries := 100

    for i := 0; i < numQueries; i++ {
        query := generateRandomVector(128)

        // HNSW search
        hnswResults := idx.Search(query, 10, 50)

        // Brute force ground truth
        groundTruth := bruteForceSearch(query, vectors, 10)

        // Calculate recall
        recall := calculateRecall(hnswResults, groundTruth)
        totalRecall += recall
    }

    avgRecall := totalRecall / float64(numQueries)
    assert.Greater(t, avgRecall, 0.95, "Recall should be >95%")
}
```

---

## 📊 Performance Tips

### Memory Profiling

```bash
# Generate memory profile
go test -memprofile=mem.prof -bench=.

# Analyze
go tool pprof mem.prof
> top10
> list FunctionName
> web
```

### CPU Profiling

```bash
# Generate CPU profile
go test -cpuprofile=cpu.prof -bench=.

# Analyze
go tool pprof cpu.prof
> top10
> list FunctionName

# Web view
go tool pprof -http=:8080 cpu.prof
```

### Optimizing Distance Calculations

```go
// Slow: Many allocations
func slowDistance(a, b []float32) float32 {
    diff := make([]float32, len(a))
    for i := range a {
        diff[i] = a[i] - b[i]
    }
    return norm(diff)
}

// Fast: No allocations
func fastDistance(a, b []float32) float32 {
    var sum float32
    for i := range a {
        diff := a[i] - b[i]
        sum += diff * diff
    }
    return math.Sqrt(sum)
}
```

### Batch Processing

```go
// Slow: Individual inserts
for _, vec := range vectors {
    idx.Insert(vec, nil)
}

// Fast: Batch insert
idx.BatchInsert(vectors, metadatas)

// Implementation
func (idx *Index) BatchInsert(vectors [][]float32, metas []map[string]interface{}) {
    // Build all nodes first
    nodes := make([]*Node, len(vectors))
    for i, vec := range vectors {
        nodes[i] = &Node{...}
    }

    // Single lock for all insertions
    idx.mu.Lock()
    defer idx.mu.Unlock()

    for _, node := range nodes {
        // Insert logic
    }
}
```

---

## 🔍 Debugging Tips

### Enable Debug Logging

```go
import "log"

func (idx *Index) debugInsert(node *Node) {
    log.Printf("[DEBUG] Inserting node %d at level %d", node.ID, len(node.Layers)-1)
    log.Printf("[DEBUG] Entry point: %d, maxLayer: %d", idx.entryPointID, idx.maxLayer)
}
```

### Visualize HNSW Graph

```go
func (idx *Index) DebugPrint() {
    fmt.Printf("Index: %d nodes, maxLayer=%d\n", len(idx.nodes), idx.maxLayer)

    for layer := idx.maxLayer; layer >= 0; layer-- {
        fmt.Printf("Layer %d:\n", layer)
        for id, node := range idx.nodes {
            if layer < len(node.Layers) {
                fmt.Printf("  Node %d: %d neighbors\n", id, len(node.Layers[layer]))
            }
        }
    }
}
```

### Race Detector

```bash
# Run with race detector
go test -race ./...
go run -race cmd/server/main.go
```

---

## 📝 Common Mistakes

### ❌ Wrong: Modifying slice while iterating
```go
for _, neighbor := range node.Layers[layer] {
    node.Layers[layer] = append(node.Layers[layer], newNeighbor) // WRONG!
}
```

### ✅ Right: Copy first
```go
neighbors := make([]uint64, len(node.Layers[layer]))
copy(neighbors, node.Layers[layer])

for _, neighbor := range neighbors {
    // Safe to modify now
}
```

### ❌ Wrong: Holding locks too long
```go
idx.mu.Lock()
// Expensive computation
time.Sleep(1 * time.Second)
idx.mu.Unlock()
```

### ✅ Right: Minimize lock time
```go
result := expensiveComputation()

idx.mu.Lock()
idx.apply(result)
idx.mu.Unlock()
```

### ❌ Wrong: Not normalizing vectors for cosine
```go
// Using cosine on unnormalized vectors
distance := CosineSimilarity(a, b)
```

### ✅ Right: Normalize first
```go
func normalize(v []float32) []float32 {
    var norm float32
    for _, x := range v {
        norm += x * x
    }
    norm = math.Sqrt(norm)

    result := make([]float32, len(v))
    for i, x := range v {
        result[i] = x / norm
    }
    return result
}

a = normalize(a)
b = normalize(b)
distance := CosineSimilarity(a, b)
```

---

## 🚀 Performance Targets

### Development Milestones

| Milestone | Vectors | Dims | Insert | Search p95 | Memory | Recall |
|-----------|---------|------|--------|------------|--------|--------|
| **Week 1** | 1K | 128 | <1ms | <2ms | <10MB | >90% |
| **Week 2** | 10K | 768 | <1ms | <5ms | <100MB | >95% |
| **Week 3** | 100K | 768 | <2ms | <10ms | <1GB | >95% |
| **Week 5** | 1M | 768 | <5ms | <20ms | <5GB | >95% |
| **Week 6** | 1M | 768 | <2ms | <10ms | <2GB | >95% |

### Production Targets (End of Week 6)

```
Dataset: 1M vectors, 768 dimensions
Throughput: >1000 QPS
Latency:
  - p50: <5ms
  - p95: <10ms
  - p99: <20ms
Memory: <2GB (with quantization)
Recall@10: >95%
```

---

## 📚 Helpful Resources

### Go Documentation
- [Go by Example](https://gobyexample.com/)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)

### Libraries
- [BadgerDB Docs](https://dgraph.io/docs/badger/)
- [Bleve Docs](https://blevesearch.com/docs/)
- [gRPC Go Tutorial](https://grpc.io/docs/languages/go/quickstart/)

### Papers
- [HNSW Paper](https://arxiv.org/abs/1603.09320)
- [Product Quantization](https://hal.inria.fr/inria-00514462v2/document)

### Tools
- [pprof Tutorial](https://go.dev/blog/pprof)
- [Delve Debugger](https://github.com/go-delve/delve)

---

**Keep this reference handy during development!** 🚀
