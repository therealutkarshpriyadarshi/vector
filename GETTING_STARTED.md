# Getting Started: Your First Week

This guide will help you get started building the vector database. Follow this day-by-day plan for your first week.

## 🎯 Prerequisites

Before you start, make sure you have:
- ✅ Go 1.21+ installed (`go version`)
- ✅ Basic understanding of Go syntax
- ✅ Git installed
- ✅ Code editor (VS Code, GoLand, etc.)

**New to Go?** Take these courses first:
1. [Tour of Go](https://go.dev/tour/) - 2-3 hours
2. [Effective Go](https://go.dev/doc/effective_go) - 1 hour read

## 📅 Week 1: Foundation (Days 1-7)

### Day 1: Setup & Understanding

**Morning: Project Setup**
```bash
cd vector
make init
go mod tidy
```

**Afternoon: Learn HNSW**
1. Read the HNSW section in `IMPLEMENTATION_GUIDE.md`
2. Watch: [HNSW Explained](https://www.youtube.com/results?search_query=hnsw+algorithm)
3. Understand the "highway analogy"

**Key Concept**: HNSW is like a highway system for vectors. You start on the fastest highway (top layer), then take progressively slower roads until you reach your exact destination (layer 0).

**Evening: Plan Your Work**
- [ ] Read `ARCHITECTURE.md` (20 mins)
- [ ] Skim the HNSW paper (focus on figures, not math)
- [ ] Create a notebook for notes and questions

### Day 2: Distance Metrics

**Goal**: Implement the foundation - distance functions.

**Create**: `pkg/hnsw/distance.go`

```go
package hnsw

import "math"

// CosineSimilarity calculates 1 - cosine similarity (lower = more similar)
func CosineSimilarity(a, b []float32) float32 {
    var dot, normA, normB float32
    for i := range a {
        dot += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }
    if normA == 0 || normB == 0 {
        return 1.0 // Maximum distance
    }
    return 1.0 - (dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB)))))
}

// EuclideanDistance calculates L2 distance
func EuclideanDistance(a, b []float32) float32 {
    var sum float32
    for i := range a {
        diff := a[i] - b[i]
        sum += diff * diff
    }
    return float32(math.Sqrt(float64(sum)))
}

// DotProduct calculates negative dot product (for normalized vectors)
func DotProduct(a, b []float32) float32 {
    var dot float32
    for i := range a {
        dot += a[i] * b[i]
    }
    return -dot // Negative because we want lower = more similar
}
```

**Create**: `pkg/hnsw/distance_test.go`

```go
package hnsw

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestCosineSimilarity(t *testing.T) {
    // Identical vectors
    v1 := []float32{1.0, 0.0, 0.0}
    v2 := []float32{1.0, 0.0, 0.0}
    assert.InDelta(t, 0.0, CosineSimilarity(v1, v2), 0.001)

    // Orthogonal vectors (90 degrees)
    v3 := []float32{1.0, 0.0, 0.0}
    v4 := []float32{0.0, 1.0, 0.0}
    assert.InDelta(t, 1.0, CosineSimilarity(v3, v4), 0.001)

    // Opposite vectors (180 degrees)
    v5 := []float32{1.0, 0.0, 0.0}
    v6 := []float32{-1.0, 0.0, 0.0}
    assert.InDelta(t, 2.0, CosineSimilarity(v5, v6), 0.001)
}

func TestEuclideanDistance(t *testing.T) {
    v1 := []float32{0.0, 0.0, 0.0}
    v2 := []float32{3.0, 4.0, 0.0}
    assert.InDelta(t, 5.0, EuclideanDistance(v1, v2), 0.001)
}

func BenchmarkCosineSimilarity(b *testing.B) {
    v1 := make([]float32, 768) // OpenAI embedding size
    v2 := make([]float32, 768)
    for i := range v1 {
        v1[i] = float32(i) * 0.01
        v2[i] = float32(i) * 0.01
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        CosineSimilarity(v1, v2)
    }
}
```

**Test it**:
```bash
go test ./pkg/hnsw -v
go test ./pkg/hnsw -bench=.
```

**Success criteria**: All tests pass, benchmark runs

### Day 3: Data Structures

**Goal**: Define the core HNSW structures.

**Create**: `pkg/hnsw/node.go`

```go
package hnsw

import "sync"

// Node represents a single vector in the HNSW graph
type Node struct {
    ID       uint64                 // Unique identifier
    Vector   []float32              // The actual vector
    Metadata map[string]interface{} // User metadata
    Layers   [][]uint64             // Neighbors at each layer
    mu       sync.RWMutex           // For concurrent access
}

// AddNeighbor adds a neighbor at a specific layer
func (n *Node) AddNeighbor(layer int, neighborID uint64) {
    n.mu.Lock()
    defer n.mu.Unlock()

    // Ensure layer exists
    for len(n.Layers) <= layer {
        n.Layers = append(n.Layers, []uint64{})
    }

    // Add neighbor if not already present
    for _, nid := range n.Layers[layer] {
        if nid == neighborID {
            return // Already exists
        }
    }

    n.Layers[layer] = append(n.Layers[layer], neighborID)
}

// GetNeighbors returns neighbors at a specific layer
func (n *Node) GetNeighbors(layer int) []uint64 {
    n.mu.RLock()
    defer n.mu.RUnlock()

    if layer >= len(n.Layers) {
        return []uint64{}
    }
    return n.Layers[layer]
}
```

**Create**: `pkg/hnsw/index.go`

```go
package hnsw

import (
    "math"
    "math/rand"
    "sync"
    "sync/atomic"
)

// DistanceFunc is a function that calculates distance between two vectors
type DistanceFunc func(a, b []float32) float32

// Index is the main HNSW index
type Index struct {
    M              int          // Max number of connections per node
    efConstruction int          // Size of dynamic candidate list during construction
    ml             float64      // Normalization factor for level generation
    maxLayer       int          // Current maximum layer
    entryPointID   uint64       // ID of entry point node
    nodes          map[uint64]*Node
    nextID         uint64       // Atomic counter for node IDs
    distFunc       DistanceFunc // Distance function to use
    mu             sync.RWMutex // Global lock for index modifications
}

// New creates a new HNSW index
func New(M, efConstruction int) *Index {
    return &Index{
        M:              M,
        efConstruction: efConstruction,
        ml:             1.0 / math.Log(float64(M)),
        maxLayer:       0,
        nodes:          make(map[uint64]*Node),
        nextID:         1,
        distFunc:       CosineSimilarity, // Default to cosine
    }
}

// SetDistanceFunc changes the distance function
func (idx *Index) SetDistanceFunc(f DistanceFunc) {
    idx.distFunc = f
}

// randomLevel generates a random level for a new node
func (idx *Index) randomLevel() int {
    level := 0
    for rand.Float64() < idx.ml && level < 16 { // Max 16 layers
        level++
    }
    return level
}

// getNextID atomically increments and returns the next ID
func (idx *Index) getNextID() uint64 {
    return atomic.AddUint64(&idx.nextID, 1)
}

// Size returns the number of nodes in the index
func (idx *Index) Size() int {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    return len(idx.nodes)
}
```

**Test it**:
```go
// pkg/hnsw/index_test.go
package hnsw

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
    idx := New(16, 200)
    assert.Equal(t, 16, idx.M)
    assert.Equal(t, 200, idx.efConstruction)
    assert.Equal(t, 0, idx.Size())
}

func TestRandomLevel(t *testing.T) {
    idx := New(16, 200)

    // Generate 1000 levels, check distribution
    levels := make(map[int]int)
    for i := 0; i < 1000; i++ {
        level := idx.randomLevel()
        levels[level]++
        assert.LessOrEqual(t, level, 16)
    }

    // Most should be at level 0 (exponential decay)
    assert.Greater(t, levels[0], 700)
}
```

**Run tests**:
```bash
go test ./pkg/hnsw -v
```

**Success criteria**: Tests pass, you understand the structures

### Day 4-5: HNSW Insert (Part 1)

**Goal**: Implement basic insertion logic.

This is the most complex part! Don't worry if it takes time.

**Create**: `pkg/hnsw/insert.go`

```go
package hnsw

// Insert adds a vector to the index
func (idx *Index) Insert(vector []float32, metadata map[string]interface{}) uint64 {
    node := &Node{
        ID:       idx.getNextID(),
        Vector:   vector,
        Metadata: metadata,
        Layers:   make([][]uint64, 0),
    }

    // Determine layer for this node
    nodeLevel := idx.randomLevel()

    idx.mu.Lock()
    defer idx.mu.Unlock()

    // First node - becomes entry point
    if len(idx.nodes) == 0 {
        idx.entryPointID = node.ID
        idx.maxLayer = nodeLevel
        node.Layers = make([][]uint64, nodeLevel+1)
        idx.nodes[node.ID] = node
        return node.ID
    }

    // TODO: Full insertion algorithm (Day 5)
    // For now, just store the node
    idx.nodes[node.ID] = node

    return node.ID
}
```

**Day 5**: Complete the full insertion algorithm (see IMPLEMENTATION_GUIDE.md for details).

**Test it**:
```bash
go test ./pkg/hnsw -v -run TestInsert
```

### Day 6-7: HNSW Search (Basic)

**Goal**: Implement basic k-NN search.

**Create**: `pkg/hnsw/search.go`

```go
package hnsw

import (
    "container/heap"
)

// Result represents a search result
type Result struct {
    ID       uint64
    Distance float32
}

// Search finds k nearest neighbors
func (idx *Index) Search(query []float32, k int, efSearch int) []Result {
    idx.mu.RLock()
    defer idx.mu.RUnlock()

    if len(idx.nodes) == 0 {
        return []Result{}
    }

    // Start from entry point
    // Traverse down layers
    // Return k best results

    // TODO: Implement full search (see IMPLEMENTATION_GUIDE.md)

    return []Result{}
}

// Priority queue for search (implement heap.Interface)
type priorityQueue []*Result

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
    return pq[i].Distance < pq[j].Distance
}

func (pq priorityQueue) Swap(i, j int) {
    pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x interface{}) {
    *pq = append(*pq, x.(*Result))
}

func (pq *priorityQueue) Pop() interface{} {
    old := *pq
    n := len(old)
    item := old[n-1]
    *pq = old[0 : n-1]
    return item
}
```

## 🎓 End of Week 1 Checklist

By end of week 1, you should have:
- ✅ Project set up and running
- ✅ Distance functions implemented and tested
- ✅ Core data structures defined
- ✅ Basic understanding of HNSW algorithm
- ✅ Started on insert/search (even if incomplete)

**Don't worry if insert/search aren't perfect yet!** Week 2 is for refinement.

## 🚀 Week 2 Preview

In week 2, you'll:
1. Complete insert algorithm with proper neighbor selection
2. Complete search algorithm with layer traversal
3. Add extensive testing (correctness + recall)
4. Integrate BadgerDB for persistence
5. Benchmark your implementation

## 💡 Tips for Success

### When You Get Stuck
1. **Read the code examples** in IMPLEMENTATION_GUIDE.md
2. **Look at hnswlib** (C++ but clear): https://github.com/nmslib/hnswlib
3. **Debug visually**: Print out layers, neighbors, distances
4. **Start simple**: Test with 3 vectors in 2D space first

### Common Mistakes to Avoid
- ❌ Don't optimize too early - make it work first
- ❌ Don't skip tests - they catch bugs early
- ❌ Don't copy-paste without understanding
- ✅ Do run benchmarks frequently
- ✅ Do commit often
- ✅ Do ask questions (create GitHub issues)

### Debugging Tips
```go
// Add debug printing in your code
func (idx *Index) debugPrint() {
    fmt.Printf("Index stats: nodes=%d, maxLayer=%d\n", len(idx.nodes), idx.maxLayer)
    for id, node := range idx.nodes {
        fmt.Printf("  Node %d: layers=%d\n", id, len(node.Layers))
    }
}
```

## 📚 Learning Resources

### HNSW Algorithm
- **Paper**: https://arxiv.org/abs/1603.09320 (focus on figures 1-4)
- **Video**: Search YouTube for "HNSW algorithm explained"
- **Interactive**: https://github.com/nmslib/hnswlib (check examples/)

### Go Concurrency
- **Blog**: https://go.dev/blog/pipelines
- **Patterns**: https://go.dev/blog/context

### Testing in Go
- **Guide**: https://go.dev/doc/tutorial/add-a-test
- **Testify**: https://github.com/stretchr/testify

## 🎯 Daily Routine

**Morning** (2-3 hours):
- Code implementation
- Write tests
- Run benchmarks

**Afternoon** (1-2 hours):
- Read documentation
- Study reference implementations
- Plan next steps

**Evening** (30 mins):
- Review progress
- Update notes
- Commit code

## ✅ Success Metrics

After Week 1, test your understanding:
1. Can you explain HNSW to a friend? (the highway analogy)
2. Can you draw the multi-layer graph structure?
3. Do your distance functions pass all tests?
4. Can you insert and retrieve a vector?

If yes to all 4, you're on track! 🎉

## 🆘 Getting Help

Stuck? Check these in order:
1. IMPLEMENTATION_GUIDE.md (detailed algorithms)
2. ARCHITECTURE.md (high-level design)
3. Go documentation (https://pkg.go.dev/)
4. hnswlib source code (reference implementation)

Good luck! Remember: **progress over perfection**.

The goal is to learn, not to build a perfect production system in week 1. 🚀
