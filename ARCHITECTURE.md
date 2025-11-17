# Vector Database Architecture

## Overview
A production-grade vector database implementing HNSW indexing, hybrid search, and multi-tenancy.

## Tech Stack
- **Language**: Go 1.21+
- **Storage**: BadgerDB (pure Go, easier than RocksDB)
- **Full-Text Search**: Bleve (Go BM25 implementation)
- **API**: gRPC + Protocol Buffers
- **Distance Metrics**: Custom SIMD-optimized implementations

## Core Components

### 1. HNSW Index (`pkg/hnsw/`)
```
hnsw/
├── index.go          # Main HNSW implementation
├── node.go           # Graph node structure
├── layer.go          # Hierarchical layers
├── search.go         # k-NN search algorithm
└── distance.go       # Distance metrics (cosine, euclidean, dot)
```

**Key Parameters:**
- `M`: Number of bidirectional links (16-64)
- `efConstruction`: Search depth during build (100-400)
- `efSearch`: Search depth during query (50-500)

### 2. Storage Layer (`pkg/storage/`)
```
storage/
├── badger.go         # BadgerDB wrapper
├── schema.go         # Data serialization
├── namespace.go      # Multi-tenancy isolation
└── cache.go          # Query result caching
```

**Data Model:**
- Vectors: `{namespace}/{vector_id} -> {vector, metadata}`
- HNSW Graph: `{namespace}/hnsw/{layer}/{node_id} -> {neighbors}`
- Inverted Index: Managed by Bleve

### 3. Hybrid Search (`pkg/search/`)
```
search/
├── hybrid.go         # RRF (Reciprocal Rank Fusion)
├── vector.go         # Pure vector search
├── fulltext.go       # BM25 full-text search
└── filter.go         # Metadata filtering
```

**Hybrid Search Algorithm:**
1. Execute vector search (HNSW) → top K results
2. Execute BM25 full-text search → top K results
3. Apply metadata filters
4. Combine with RRF: score = Σ(1/(rank + 60))
5. Return top N merged results

### 4. API Layer (`pkg/api/`)
```
api/
├── grpc/
│   ├── server.go     # gRPC server
│   ├── handlers.go   # RPC handlers
│   └── proto/        # Protocol buffers
└── rest/
    └── gateway.go    # Optional REST gateway
```

### 5. Multi-tenancy (`pkg/tenant/`)
```
tenant/
├── manager.go        # Namespace management
├── isolation.go      # Data isolation enforcement
└── quotas.go         # Resource limits per tenant
```

## Performance Optimizations

### Phase 1: Core Functionality
- Basic HNSW implementation
- Single distance metric (cosine)
- Simple storage without optimization

### Phase 2: Production Features
- **SIMD Vectorization**: Use `golang.org/x/sys/cpu` for AVX2/SSE
- **Goroutine Pool**: Limit concurrent searches
- **Batch Operations**: Bulk insert with write buffering
- **Query Caching**: LRU cache for frequent queries

### Phase 3: Advanced Features
- **Quantization**:
  - Scalar Quantization (SQ): float32 → int8 (4x memory reduction)
  - Product Quantization (PQ): Complex but 8-32x reduction
- **Index Sharding**: Split large indexes across nodes
- **Incremental Updates**: Add/delete without full rebuild

## Distance Metrics

| Metric | Use Case | Formula |
|--------|----------|---------|
| **Cosine** | Text embeddings (normalized) | 1 - (A·B)/(‖A‖‖B‖) |
| **Euclidean** | General purpose | √Σ(ai - bi)² |
| **Dot Product** | Pre-normalized vectors | -(A·B) |

## Metadata Filtering

Support advanced filters:
```json
{
  "and": [
    {"field": "category", "op": "eq", "value": "technology"},
    {"field": "date", "op": "range", "value": {"gte": "2024-01-01"}},
    {"field": "location", "op": "geo_radius", "value": {"lat": 37.7, "lon": -122.4, "radius": "10km"}}
  ]
}
```

## Benchmarking Targets

- **Latency**: <10ms for 1M vectors (p95)
- **Throughput**: >1000 QPS on single core
- **Recall@10**: >95% vs brute force
- **Memory**: <1GB for 1M vectors (768 dimensions)

## Testing Strategy

1. **Unit Tests**: Each component isolated
2. **Integration Tests**: End-to-end workflows
3. **Benchmark Tests**: Performance regression detection
4. **Recall Tests**: Accuracy vs brute force
5. **Load Tests**: Concurrent access, multi-tenancy

## Development Phases (3-6 weeks)

### Week 1-2: Core HNSW + Storage
- [ ] HNSW index implementation
- [ ] BadgerDB integration
- [ ] Basic vector insert/search
- [ ] Single distance metric

### Week 3-4: Hybrid Search + API
- [ ] Bleve full-text integration
- [ ] Reciprocal Rank Fusion
- [ ] gRPC API design
- [ ] Metadata filtering

### Week 5: Production Features
- [ ] Multi-tenancy (namespaces)
- [ ] Batch operations
- [ ] Query caching
- [ ] SIMD optimizations

### Week 6: Polish + Validation
- [ ] Comprehensive testing
- [ ] Benchmarking vs competitors
- [ ] Documentation
- [ ] Example applications

## References

- **HNSW Paper**: https://arxiv.org/abs/1603.09320
- **Weaviate (Go)**: https://github.com/weaviate/weaviate
- **hnswlib (C++)**: https://github.com/nmslib/hnswlib
- **Reciprocal Rank Fusion**: https://plg.uwaterloo.ca/~gvcormac/cormacksigir09-rrf.pdf
