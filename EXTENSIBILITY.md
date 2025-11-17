# Extensibility & Plugin Architecture

How the vector database is designed to support multiple algorithms, distance metrics, and storage backends.

---

## 🎯 Design Philosophy

The project follows **interface-driven design** to make adding new algorithms trivial:

```
Core Interface → Multiple Implementations → Easy Swapping
```

---

## 🔌 Core Interfaces

### 1. Vector Index Interface

```go
// pkg/index/interface.go
package index

// VectorIndex is the core interface all indexes must implement
type VectorIndex interface {
    // Core operations
    Insert(vec []float32, meta map[string]interface{}) (uint64, error)
    Search(query []float32, k int, params SearchParams) ([]Result, error)
    Delete(id uint64) error
    Update(id uint64, vec []float32) error

    // Batch operations
    BatchInsert(vecs [][]float32, metas []map[string]interface{}) ([]uint64, error)
    BatchSearch(queries [][]float32, k int) ([][]Result, error)

    // Persistence
    Save(path string) error
    Load(path string) error

    // Metadata
    Size() int
    Stats() IndexStats
}

// SearchParams allows algorithm-specific parameters
type SearchParams struct {
    EfSearch    int               // HNSW-specific
    Nprobe      int               // IVF-specific
    BeamWidth   int               // DiskANN-specific
    Custom      map[string]interface{}
}

// Result represents a search result
type Result struct {
    ID       uint64
    Distance float32
    Metadata map[string]interface{}
}

// IndexStats provides index statistics
type IndexStats struct {
    NumVectors     int
    MemoryUsage    int64
    DiskUsage      int64
    IndexType      string
    BuildTime      time.Duration
    LastUpdate     time.Time
}
```

---

## 🔧 Implementation Examples

### HNSW Implementation

```go
// pkg/hnsw/index.go
package hnsw

import "github.com/therealutkarshpriyadarshi/vector/pkg/index"

// Ensure HNSWIndex implements VectorIndex
var _ index.VectorIndex = (*HNSWIndex)(nil)

type HNSWIndex struct {
    M              int
    efConstruction int
    distFunc       DistanceFunc
    nodes          map[uint64]*Node
    // ... other fields
}

func New(M, efConstruction int) *HNSWIndex {
    return &HNSWIndex{
        M:              M,
        efConstruction: efConstruction,
        nodes:          make(map[uint64]*Node),
        distFunc:       CosineSimilarity,
    }
}

func (idx *HNSWIndex) Insert(vec []float32, meta map[string]interface{}) (uint64, error) {
    // HNSW-specific implementation
    return id, nil
}

func (idx *HNSWIndex) Search(query []float32, k int, params index.SearchParams) ([]index.Result, error) {
    // Use params.EfSearch if provided, else default
    efSearch := params.EfSearch
    if efSearch == 0 {
        efSearch = 50
    }

    // HNSW search logic
    return results, nil
}
```

### IVF Implementation

```go
// pkg/ivf/index.go
package ivf

import "github.com/therealutkarshpriyadarshi/vector/pkg/index"

var _ index.VectorIndex = (*IVFIndex)(nil)

type IVFIndex struct {
    nlist     int
    centroids [][]float32
    lists     [][]uint64
    // ... other fields
}

func New(nlist int) *IVFIndex {
    return &IVFIndex{
        nlist: nlist,
        lists: make([][]uint64, nlist),
    }
}

func (idx *IVFIndex) Search(query []float32, k int, params index.SearchParams) ([]index.Result, error) {
    // Use params.Nprobe for IVF-specific parameter
    nprobe := params.Nprobe
    if nprobe == 0 {
        nprobe = 10
    }

    // IVF search logic
    return results, nil
}
```

---

## 🏭 Factory Pattern

### Index Factory

```go
// pkg/index/factory.go
package index

type IndexConfig struct {
    Type   string
    Params map[string]interface{}
}

// Factory creates indexes based on configuration
func NewIndex(config IndexConfig) (VectorIndex, error) {
    switch config.Type {
    case "hnsw":
        return createHNSW(config.Params)
    case "ivf":
        return createIVF(config.Params)
    case "nsg":
        return createNSG(config.Params)
    case "diskann":
        return createDiskANN(config.Params)
    default:
        return nil, fmt.Errorf("unknown index type: %s", config.Type)
    }
}

func createHNSW(params map[string]interface{}) (VectorIndex, error) {
    M := getInt(params, "M", 16)
    efConstruction := getInt(params, "efConstruction", 200)
    return hnsw.New(M, efConstruction), nil
}

func createIVF(params map[string]interface{}) (VectorIndex, error) {
    nlist := getInt(params, "nlist", 100)
    return ivf.New(nlist), nil
}

// Helper to extract parameters with defaults
func getInt(params map[string]interface{}, key string, defaultVal int) int {
    if val, ok := params[key]; ok {
        if intVal, ok := val.(int); ok {
            return intVal
        }
    }
    return defaultVal
}
```

### Usage

```go
// Create different indexes easily
hnswIdx, _ := index.NewIndex(index.IndexConfig{
    Type: "hnsw",
    Params: map[string]interface{}{
        "M":              16,
        "efConstruction": 200,
    },
})

ivfIdx, _ := index.NewIndex(index.IndexConfig{
    Type: "ivf",
    Params: map[string]interface{}{
        "nlist": 100,
    },
})

// Swap indexes without changing application code
var myIndex index.VectorIndex
myIndex = hnswIdx  // or ivfIdx
myIndex.Insert(vec, meta)
```

---

## 📏 Distance Metric Interface

### Pluggable Distance Functions

```go
// pkg/distance/interface.go
package distance

type DistanceFunc func(a, b []float32) float32

// Registry of distance functions
var registry = map[string]DistanceFunc{
    "cosine":    Cosine,
    "euclidean": Euclidean,
    "dot":       DotProduct,
    "manhattan": Manhattan,
    "hamming":   Hamming,
}

// Register allows adding custom distance functions
func Register(name string, fn DistanceFunc) {
    registry[name] = fn
}

// Get retrieves a distance function by name
func Get(name string) (DistanceFunc, error) {
    fn, ok := registry[name]
    if !ok {
        return nil, fmt.Errorf("unknown distance metric: %s", name)
    }
    return fn, nil
}

// Implementations
func Cosine(a, b []float32) float32 {
    // ... implementation
}

func Euclidean(a, b []float32) float32 {
    // ... implementation
}

func DotProduct(a, b []float32) float32 {
    // ... implementation
}

func Manhattan(a, b []float32) float32 {
    var sum float32
    for i := range a {
        sum += abs(a[i] - b[i])
    }
    return sum
}

func Hamming(a, b []float32) float32 {
    var diff int
    for i := range a {
        if a[i] != b[i] {
            diff++
        }
    }
    return float32(diff)
}
```

### Custom Distance Function

```go
// User can add custom metrics
import "github.com/yourproject/vector/pkg/distance"

// Jaccard similarity for binary vectors
func JaccardDistance(a, b []float32) float32 {
    var intersection, union float32
    for i := range a {
        if a[i] > 0 && b[i] > 0 {
            intersection++
        }
        if a[i] > 0 || b[i] > 0 {
            union++
        }
    }
    return 1.0 - (intersection / union)
}

// Register custom metric
distance.Register("jaccard", JaccardDistance)

// Use it
idx := hnsw.New(16, 200)
idx.SetDistanceFunc(distance.Get("jaccard"))
```

---

## 💾 Storage Backend Interface

### Pluggable Storage

```go
// pkg/storage/interface.go
package storage

type StorageBackend interface {
    // Key-value operations
    Put(key, value []byte) error
    Get(key []byte) ([]byte, error)
    Delete(key []byte) error

    // Batch operations
    BatchPut(items map[string][]byte) error
    BatchGet(keys [][]byte) ([][]byte, error)

    // Iteration
    Scan(prefix []byte, fn func(key, value []byte) error) error

    // Transactions
    BeginTx() (Transaction, error)

    // Management
    Close() error
    Stats() StorageStats
}

type Transaction interface {
    Put(key, value []byte) error
    Delete(key []byte) error
    Commit() error
    Rollback() error
}

type StorageStats struct {
    NumKeys     int64
    DiskUsage   int64
    MemoryUsage int64
}
```

### Multiple Storage Implementations

```go
// pkg/storage/badger/backend.go
type BadgerBackend struct {
    db *badger.DB
}

func NewBadger(path string) (*BadgerBackend, error) {
    db, err := badger.Open(badger.DefaultOptions(path))
    if err != nil {
        return nil, err
    }
    return &BadgerBackend{db: db}, nil
}

func (b *BadgerBackend) Put(key, value []byte) error {
    return b.db.Update(func(txn *badger.Txn) error {
        return txn.Set(key, value)
    })
}

// pkg/storage/rocksdb/backend.go (alternative)
type RocksDBBackend struct {
    db *gorocksdb.DB
}

// pkg/storage/memory/backend.go (for testing)
type MemoryBackend struct {
    data map[string][]byte
    mu   sync.RWMutex
}
```

### Storage Factory

```go
// pkg/storage/factory.go
func NewStorage(storageType, path string) (StorageBackend, error) {
    switch storageType {
    case "badger":
        return badger.NewBadger(path)
    case "rocksdb":
        return rocksdb.NewRocksDB(path)
    case "memory":
        return memory.NewMemory()
    case "s3":
        return s3.NewS3Backend(path)
    default:
        return nil, fmt.Errorf("unknown storage type: %s", storageType)
    }
}
```

---

## 🔍 Filter Interface

### Pluggable Filters

```go
// pkg/filter/interface.go
package filter

type Filter interface {
    Matches(metadata map[string]interface{}) bool
}

// Basic filters
type EqualFilter struct {
    Field string
    Value interface{}
}

func (f *EqualFilter) Matches(meta map[string]interface{}) bool {
    return meta[f.Field] == f.Value
}

type RangeFilter struct {
    Field string
    Min   float64
    Max   float64
}

func (f *RangeFilter) Matches(meta map[string]interface{}) bool {
    val, ok := meta[f.Field].(float64)
    return ok && val >= f.Min && val <= f.Max
}

// Composite filters
type AndFilter struct {
    Filters []Filter
}

func (f *AndFilter) Matches(meta map[string]interface{}) bool {
    for _, filter := range f.Filters {
        if !filter.Matches(meta) {
            return false
        }
    }
    return true
}

type OrFilter struct {
    Filters []Filter
}

func (f *OrFilter) Matches(meta map[string]interface{}) bool {
    for _, filter := range f.Filters {
        if filter.Matches(meta) {
            return true
        }
    }
    return false
}
```

### Filter Builder

```go
// pkg/filter/builder.go
type Builder struct {
    filters []Filter
}

func New() *Builder {
    return &Builder{}
}

func (b *Builder) Equal(field string, value interface{}) *Builder {
    b.filters = append(b.filters, &EqualFilter{Field: field, Value: value})
    return b
}

func (b *Builder) Range(field string, min, max float64) *Builder {
    b.filters = append(b.filters, &RangeFilter{Field: field, Min: min, Max: max})
    return b
}

func (b *Builder) Build() Filter {
    if len(b.filters) == 1 {
        return b.filters[0]
    }
    return &AndFilter{Filters: b.filters}
}

// Usage
filter := filter.New().
    Equal("category", "tech").
    Range("price", 10.0, 100.0).
    Build()

results := idx.SearchWithFilter(query, k, filter)
```

---

## 🔌 Plugin System

### Dynamic Algorithm Loading

```go
// pkg/plugin/loader.go
package plugin

import "plugin"

type AlgorithmPlugin interface {
    Name() string
    Version() string
    NewIndex(params map[string]interface{}) (index.VectorIndex, error)
}

func LoadPlugin(path string) (AlgorithmPlugin, error) {
    p, err := plugin.Open(path)
    if err != nil {
        return nil, err
    }

    sym, err := p.Lookup("Algorithm")
    if err != nil {
        return nil, err
    }

    alg, ok := sym.(AlgorithmPlugin)
    if !ok {
        return nil, fmt.Errorf("invalid plugin")
    }

    return alg, nil
}

// Usage
alg, _ := plugin.LoadPlugin("./plugins/my_custom_algo.so")
idx, _ := alg.NewIndex(params)
```

### Creating a Plugin

```go
// plugins/myalgo/plugin.go
package main

import "github.com/therealutkarshpriyadarshi/vector/pkg/index"

type MyAlgorithm struct{}

func (a *MyAlgorithm) Name() string {
    return "MyCustomAlgorithm"
}

func (a *MyAlgorithm) Version() string {
    return "1.0.0"
}

func (a *MyAlgorithm) NewIndex(params map[string]interface{}) (index.VectorIndex, error) {
    return NewMyIndex(params), nil
}

// Export symbol
var Algorithm MyAlgorithm

// Build: go build -buildmode=plugin -o myalgo.so plugin.go
```

---

## 📝 Configuration System

### YAML Configuration

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 9000
  tls:
    enabled: false

index:
  type: "hnsw"  # Can swap: hnsw, ivf, nsg, diskann
  params:
    M: 16
    efConstruction: 200
  distance: "cosine"  # cosine, euclidean, dot

storage:
  backend: "badger"  # badger, rocksdb, memory
  path: "/var/lib/vectordb"
  cache_size: 1GB

search:
  default_k: 10
  default_ef_search: 50
  enable_caching: true
  cache_size: 1000

multi_tenancy:
  enabled: true
  default_quota:
    max_vectors: 1000000
    max_storage: 10GB
```

### Configuration Loading

```go
// pkg/config/config.go
package config

import (
    "gopkg.in/yaml.v3"
    "io/ioutil"
)

type Config struct {
    Server struct {
        Host string `yaml:"host"`
        Port int    `yaml:"port"`
    } `yaml:"server"`

    Index struct {
        Type     string                 `yaml:"type"`
        Params   map[string]interface{} `yaml:"params"`
        Distance string                 `yaml:"distance"`
    } `yaml:"index"`

    Storage struct {
        Backend   string `yaml:"backend"`
        Path      string `yaml:"path"`
        CacheSize string `yaml:"cache_size"`
    } `yaml:"storage"`
}

func Load(path string) (*Config, error) {
    data, err := ioutil.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}

// Usage
cfg, _ := config.Load("config.yaml")

idx, _ := index.NewIndex(index.IndexConfig{
    Type:   cfg.Index.Type,
    Params: cfg.Index.Params,
})

storage, _ := storage.NewStorage(cfg.Storage.Backend, cfg.Storage.Path)
```

---

## 🧪 Testing Framework

### Algorithm-Agnostic Tests

```go
// pkg/index/interface_test.go
package index_test

import (
    "testing"
    "github.com/yourproject/vector/pkg/hnsw"
    "github.com/yourproject/vector/pkg/ivf"
    "github.com/yourproject/vector/pkg/nsg"
)

// TestAllAlgorithms tests all implementations
func TestAllAlgorithms(t *testing.T) {
    algorithms := []struct {
        name  string
        index index.VectorIndex
    }{
        {"HNSW", hnsw.New(16, 200)},
        {"IVF", ivf.New(100)},
        {"NSG", nsg.New()},
    }

    for _, alg := range algorithms {
        t.Run(alg.name, func(t *testing.T) {
            testInsert(t, alg.index)
            testSearch(t, alg.index)
            testDelete(t, alg.index)
            testPersistence(t, alg.index)
        })
    }
}

func testInsert(t *testing.T, idx index.VectorIndex) {
    vec := []float32{0.1, 0.2, 0.3}
    id, err := idx.Insert(vec, nil)
    assert.NoError(t, err)
    assert.Greater(t, id, uint64(0))
}

func testSearch(t *testing.T, idx index.VectorIndex) {
    // Insert test vectors
    // Search
    // Verify results
}
```

---

## 🎯 Benefits of This Architecture

### 1. **Easy to Add New Algorithms**
```
1. Implement VectorIndex interface
2. Register in factory
3. Done! Works with all existing code
```

### 2. **Easy to Benchmark**
```go
for _, algo := range algorithms {
    benchmark(algo)
}
```

### 3. **Easy to A/B Test**
```go
// Production: Route 10% traffic to new algorithm
if rand.Float64() < 0.1 {
    return newAlgorithm.Search(query, k)
} else {
    return currentAlgorithm.Search(query, k)
}
```

### 4. **Easy to Extend**
- Add distance metrics: Implement `DistanceFunc`
- Add storage: Implement `StorageBackend`
- Add filters: Implement `Filter`
- Add algorithms: Implement `VectorIndex`

---

## 🚀 Real-World Example

### Multi-Algorithm Database

```go
// cmd/server/main.go
func main() {
    // Load config
    cfg := config.Load("config.yaml")

    // Create indexes for different use cases
    indexes := map[string]index.VectorIndex{
        "fast":     ivf.New(100),              // Fast but lower recall
        "accurate": hnsw.New(32, 400),         // Accurate but more memory
        "large":    diskann.New(...),          // For billion-scale
    }

    // Route requests based on requirements
    http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
        mode := r.URL.Query().Get("mode") // fast, accurate, large

        idx := indexes[mode]
        if idx == nil {
            idx = indexes["accurate"] // default
        }

        results := idx.Search(query, k, params)
        json.NewEncoder(w).Encode(results)
    })
}
```

---

## 📚 Summary

This architecture enables:

✅ **Multiple algorithms** co-existing
✅ **Easy swapping** without code changes
✅ **Custom extensions** via plugins
✅ **Algorithm-agnostic** application code
✅ **Easy benchmarking** and comparison
✅ **Future-proof** for new research

**The key**: Design for interfaces, not implementations! 🎯
