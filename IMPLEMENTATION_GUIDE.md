# Vector Database Implementation Guide

## Getting Started

### Step 1: Project Setup (Day 1)

```bash
# Initialize Go module
go mod init github.com/therealutkarshpriyadarshi/vector

# Install dependencies
go get github.com/dgraph-io/badger/v4      # Storage
go get github.com/blevesearch/bleve/v2     # Full-text search
go get google.golang.org/grpc              # gRPC API
go get google.golang.org/protobuf          # Protocol buffers
go get github.com/stretchr/testify         # Testing
```

### Step 2: Understand HNSW Algorithm (Day 2-3)

**HNSW (Hierarchical Navigable Small World)**

Think of it as a "highway system" for vectors:
- **Layer 0**: All vectors (local roads)
- **Layer 1+**: Subset of vectors (highways, freeways)

**Search Process:**
1. Start at top layer (fastest highways)
2. Find closest neighbor, jump to it
3. Repeat until no closer neighbor found
4. Move down one layer
5. Repeat until reaching Layer 0
6. Return K nearest neighbors

**Key Insight**: Greedy search on hierarchical graph is ~100-1000x faster than brute force!

**Visual Example:**
```
Layer 2:  [1]--------[50]--------[99]
           |           |           |
Layer 1:  [1]--[20]--[50]--[75]--[99]
           |    |     |     |     |
Layer 0:  [1]-[5]-[20]-[35]-[50]-[75]-[99]...
          (all 100 vectors connected)

Query: Find nearest to vector [48]
1. Start at entry point [1] in Layer 2
2. Jump to [50] (closer to 48)
3. No closer neighbor in Layer 2
4. Drop to Layer 1, refine search
5. Find [50] still closest, but check [35] and [75]
6. Drop to Layer 0, find [48] or very close
```

### Step 3: HNSW Implementation Strategy (Week 1-2)

#### 3.1 Core Data Structures

```go
// pkg/hnsw/node.go
type Node struct {
    ID       uint64
    Vector   []float32
    Metadata map[string]interface{}
    Layers   [][]uint64  // Neighbors at each layer
}

// pkg/hnsw/index.go
type Index struct {
    M              int       // Max connections per node
    efConstruction int       // Search depth during build
    ml             float64   // Layer normalization factor
    entryPoint     *Node     // Top layer entry
    nodes          map[uint64]*Node
    maxLayer       int
    mu             sync.RWMutex
}
```

#### 3.2 Distance Functions (Start Here!)

```go
// pkg/hnsw/distance.go
func CosineSimilarity(a, b []float32) float32 {
    var dot, normA, normB float32
    for i := range a {
        dot += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }
    return 1.0 - (dot / (sqrt(normA) * sqrt(normB)))
}

func EuclideanDistance(a, b []float32) float32 {
    var sum float32
    for i := range a {
        diff := a[i] - b[i]
        sum += diff * diff
    }
    return sqrt(sum)
}
```

#### 3.3 Core HNSW Operations

**Insert Algorithm** (Simplified):
```go
func (idx *Index) Insert(vector []float32, metadata map[string]interface{}) uint64 {
    // 1. Assign layer (exponential decay probability)
    layer := idx.randomLayer()

    // 2. Create node
    node := &Node{
        ID:       idx.nextID(),
        Vector:   vector,
        Metadata: metadata,
        Layers:   make([][]uint64, layer+1),
    }

    // 3. Find insertion point (greedy search from top)
    entryPoints := idx.searchLayer(vector, idx.entryPoint, 1, idx.maxLayer)

    // 4. Insert at each layer
    for lc := layer; lc >= 0; lc-- {
        candidates := idx.searchLayer(vector, entryPoints, idx.efConstruction, lc)
        neighbors := idx.selectNeighbors(candidates, idx.M)

        // Add bidirectional links
        node.Layers[lc] = neighbors
        for _, nID := range neighbors {
            idx.nodes[nID].Layers[lc].Add(node.ID)
        }

        entryPoints = candidates
    }

    return node.ID
}
```

**Search Algorithm** (Simplified):
```go
func (idx *Index) Search(query []float32, k int, efSearch int) []Result {
    // 1. Start from entry point at top layer
    ep := idx.entryPoint

    // 2. Traverse down to layer 0
    for lc := idx.maxLayer; lc > 0; lc-- {
        ep = idx.searchLayer(query, []*Node{ep}, 1, lc)[0]
    }

    // 3. Final search at layer 0
    candidates := idx.searchLayer(query, []*Node{ep}, efSearch, 0)

    // 4. Return top K
    return candidates[:k]
}
```

### Step 4: Storage Layer (Week 2)

**BadgerDB Integration:**

```go
// pkg/storage/badger.go
type Store struct {
    db *badger.DB
}

func (s *Store) PutVector(namespace, id string, data []byte) error {
    key := fmt.Sprintf("%s/vec/%s", namespace, id)
    return s.db.Update(func(txn *badger.Txn) error {
        return txn.Set([]byte(key), data)
    })
}

func (s *Store) GetVector(namespace, id string) ([]byte, error) {
    key := fmt.Sprintf("%s/vec/%s", namespace, id)
    var data []byte
    err := s.db.View(func(txn *badger.Txn) error {
        item, err := txn.Get([]byte(key))
        if err != nil {
            return err
        }
        data, err = item.ValueCopy(nil)
        return err
    })
    return data, err
}
```

### Step 5: Hybrid Search (Week 3)

**Reciprocal Rank Fusion:**

```go
// pkg/search/hybrid.go
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

### Step 6: gRPC API (Week 3-4)

**Protocol Buffer Definition:**

```protobuf
// api/proto/vector.proto
syntax = "proto3";

service VectorDB {
  rpc Insert(InsertRequest) returns (InsertResponse);
  rpc Search(SearchRequest) returns (SearchResponse);
  rpc HybridSearch(HybridSearchRequest) returns (SearchResponse);
  rpc Delete(DeleteRequest) returns (DeleteResponse);
}

message InsertRequest {
  string namespace = 1;
  repeated float vector = 2;
  map<string, string> metadata = 3;
  string text = 4;  // For full-text indexing
}

message SearchRequest {
  string namespace = 1;
  repeated float query = 2;
  int32 k = 3;
  Filter filter = 4;
}

message HybridSearchRequest {
  string namespace = 1;
  repeated float vector_query = 2;
  string text_query = 3;
  int32 k = 4;
  Filter filter = 5;
}
```

## Common Pitfalls to Avoid

### 1. **HNSW: Connection Pruning**
❌ **Wrong**: Connect to ALL nearest neighbors
✅ **Right**: Use heuristic to maintain graph connectivity
- Select diverse neighbors (avoid clustering)
- Ensure long-range and short-range connections

### 2. **Concurrency**
❌ **Wrong**: Single global lock for index
✅ **Right**: Fine-grained locking per node layer
```go
// Lock only during neighbor list modification
node.mu.Lock()
node.Layers[layer] = append(node.Layers[layer], newNeighbor)
node.mu.Unlock()
```

### 3. **Memory Usage**
❌ **Wrong**: Load all vectors into RAM
✅ **Right**:
- Keep only HNSW graph in memory
- Load vectors on-demand from BadgerDB
- Use mmap for large datasets

### 4. **Distance Metrics**
❌ **Wrong**: Use Euclidean for normalized embeddings
✅ **Right**: Use cosine similarity for text embeddings (OpenAI, Sentence-BERT)

## Testing Your Implementation

### Correctness Test
```go
func TestHNSW_Recall(t *testing.T) {
    // Generate 10,000 random vectors
    vectors := generateRandomVectors(10000, 768)

    // Build index
    idx := hnsw.New(16, 200)
    for _, vec := range vectors {
        idx.Insert(vec, nil)
    }

    // Test 100 random queries
    for i := 0; i < 100; i++ {
        query := generateRandomVector(768)

        // HNSW search
        hnswResults := idx.Search(query, 10, 50)

        // Brute force ground truth
        bruteResults := bruteForceSearch(query, vectors, 10)

        // Calculate recall@10
        recall := calculateRecall(hnswResults, bruteResults)
        assert.Greater(t, recall, 0.95) // >95% recall
    }
}
```

### Performance Benchmark
```go
func BenchmarkHNSW_Search(b *testing.B) {
    idx := buildIndex(1000000, 768) // 1M vectors
    query := generateRandomVector(768)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        idx.Search(query, 10, 50)
    }
}
```

## Production Considerations

### 1. Persistence
- Save HNSW graph to BadgerDB on shutdown
- Load graph on startup (faster than rebuild)
- Implement WAL (Write-Ahead Log) for crash recovery

### 2. Monitoring
- Track query latency (p50, p95, p99)
- Monitor index size and memory usage
- Alert on recall degradation

### 3. Scaling
- **Vertical**: Single machine with 256GB RAM can handle 100M vectors
- **Horizontal**: Shard by namespace or vector ID ranges
- **Replication**: Read replicas for high QPS

## Resources for Deep Dive

### Must-Read Papers
1. **HNSW**: "Efficient and robust approximate nearest neighbor search using Hierarchical Navigable Small World graphs" (2018)
2. **Product Quantization**: "Product Quantization for Nearest Neighbor Search" (2011)
3. **Reciprocal Rank Fusion**: Cormack et al., SIGIR 2009

### Code References
- **hnswlib**: https://github.com/nmslib/hnswlib (C++, reference implementation)
- **Weaviate**: https://github.com/weaviate/weaviate (Go, production system)
- **Milvus**: https://github.com/milvus-io/milvus (Go/C++, distributed)

### Tools
- **ann-benchmarks**: Compare your implementation to state-of-the-art
- **Grafana + Prometheus**: Monitor production metrics
- **pprof**: Profile CPU and memory usage in Go

## Next Steps

1. **Start with Day 1-3**: Set up project, implement distance functions
2. **Week 1-2**: Core HNSW (insert + search)
3. **Week 2**: Integrate BadgerDB persistence
4. **Week 3**: Add Bleve for full-text, implement RRF
5. **Week 4**: Build gRPC API
6. **Week 5**: Multi-tenancy, filtering, batch ops
7. **Week 6**: Optimize, benchmark, document

Let's start coding! 🚀
