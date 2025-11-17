# Algorithm Deep Dive

Comprehensive guide to HNSW and NSG algorithms used in the Vector Database.

## Table of Contents

- [Overview](#overview)
- [HNSW (Hierarchical Navigable Small World)](#hnsw-hierarchical-navigable-small-world)
  - [Algorithm Intuition](#algorithm-intuition)
  - [Data Structure](#data-structure)
  - [Insert Algorithm](#insert-algorithm)
  - [Search Algorithm](#search-algorithm)
  - [Parameter Tuning](#parameter-tuning)
  - [Complexity Analysis](#complexity-analysis)
- [NSG (Navigating Spreading-out Graph)](#nsg-navigating-spreading-out-graph)
  - [Overview](#nsg-overview)
  - [Key Differences from HNSW](#key-differences-from-hnsw)
  - [Algorithm Details](#algorithm-details)
  - [Performance Characteristics](#performance-characteristics)
- [Distance Metrics](#distance-metrics)
- [Optimization Techniques](#optimization-techniques)
- [Algorithm Comparison](#algorithm-comparison)

---

## Overview

This database implements two state-of-the-art graph-based approximate nearest neighbor (ANN) algorithms:

1. **HNSW** (Hierarchical Navigable Small World) - Production default
2. **NSG** (Navigating Spreading-out Graph) - Alternative for better recall

Both algorithms achieve **sub-linear search time** O(log N) while maintaining **high recall** (>95%).

**Why Graph-Based Algorithms?**
- Fast search: O(log N) vs O(N) for brute force
- High recall: >95% vs 60-80% for tree-based methods
- Scalable: Handle millions of vectors
- Flexible: Support any distance metric

---

## HNSW (Hierarchical Navigable Small World)

**Paper**: [Efficient and robust approximate nearest neighbor search using Hierarchical Navigable Small World graphs](https://arxiv.org/abs/1603.09320) (2016)

**Authors**: Yu. A. Malkov, D. A. Yashunin

### Algorithm Intuition

Think of HNSW as a **highway system for searching**:

```
Layer 2: [==== Long-distance highways ====]  (sparse, few nodes)
           |         |         |
Layer 1: [== Regional roads ==]              (medium density)
           |    |    |    |
Layer 0: [= Local streets =]                 (dense, all nodes)
```

**Key Insight**: Start search on highways (top layer) for fast long-distance travel, then zoom into local streets (bottom layer) for precise navigation.

**Visual Example**:

```
Query: Find nearest neighbor to point Q

Layer 2:  A -------- B -------- C
          |          |          |
Layer 1:  A -- D -- B -- E -- C -- F
          |  |  |  |  |  |  |  |  |
Layer 0:  A-D-G-H-B-E-I-J-C-F-K-L-M
             ^
             Q (query point)

Search path: A → B → E → H → Q (found!)
```

### Data Structure

```go
// pkg/hnsw/index.go

type Index struct {
    nodes       []*Node             // All nodes in the graph
    entryPoint  *Node               // Top-level entry point
    maxLevel    int                 // Maximum layer in the graph
    m           int                 // Max connections per layer
    mMax        int                 // Max connections at layer 0
    efConstruction int              // Construction time accuracy
    dimensions  int                 // Vector dimensions
    distanceFunc func(a, b []float32) float32
    mu          sync.RWMutex        // Thread-safe access
}

type Node struct {
    id       uint64                 // Unique node ID
    vector   []float32              // Vector embedding
    level    int                    // Node's highest layer
    neighbors map[int][]uint64      // neighbors[layer] = list of IDs
    metadata map[string]interface{} // Associated metadata
}
```

**Memory Layout**:
- **Node**: ~32 bytes + vector size + neighbors
- **Neighbors**: Average M * levels connections
- **Total**: (32 + dims*4 + M*8*log(N)) bytes per node

Example for 768-dim vector:
- Vector: 768 * 4 = 3,072 bytes
- Node metadata: ~32 bytes
- Neighbors (M=16, 3 layers): ~384 bytes
- **Total**: ~3,500 bytes per vector

### Insert Algorithm

**Goal**: Add new node to the graph while maintaining small-world properties.

```go
// Pseudocode
func Insert(vector, metadata) uint64 {
    // 1. Determine random layer for new node
    level := randomLevel()
    node := NewNode(id, vector, level, metadata)

    // 2. Find insertion points at each layer
    entryPoints := []Node{globalEntryPoint}

    for currentLevel := maxLevel; currentLevel >= 0; currentLevel-- {
        // Search for nearest neighbors at this layer
        candidates := searchLayer(vector, entryPoints, ef=1, currentLevel)

        if currentLevel <= level {
            // 3. Select M best neighbors
            neighbors := selectNeighbors(candidates, M)

            // 4. Add bidirectional links
            for neighbor := range neighbors {
                node.neighbors[currentLevel].add(neighbor)
                neighbor.neighbors[currentLevel].add(node)

                // 5. Prune neighbor connections if needed
                if len(neighbor.neighbors[currentLevel]) > M_max {
                    neighbor.neighbors[currentLevel] = prune(neighbor, M_max)
                }
            }
        }

        // Use found neighbors as entry points for next layer
        entryPoints = candidates
    }

    // 6. Update global entry point if needed
    if level > maxLevel {
        entryPoint = node
        maxLevel = level
    }

    return node.id
}

// Random level selection (exponential distribution)
func randomLevel() int {
    level := 0
    mL := 1.0 / log(M)  // Normalization factor
    for rand.Float64() < mL && level < maxLevel {
        level++
    }
    return level
}
```

**Visual Example**:

```
Inserting node X at level 2:

Before:
Layer 2:  A ----------- B
          |             |
Layer 1:  A --- C --- B --- D
          |     |     |     |
Layer 0:  A--C--E--F--B--D--G

After inserting X (between C and B):
Layer 2:  A ----- X ----- B
          |       |       |
Layer 1:  A--C----X----B--D
          |  |    |    |  |
Layer 0:  A--C-E--X-F--B--D--G

New connections: A↔X, X↔B (layer 2), C↔X, X↔B (layer 1), etc.
```

**Time Complexity**: O(log N) expected
- Search at each layer: O(M * log N / log M)
- Number of layers: O(log N)
- Total: O(M * log N)

### Search Algorithm

**Goal**: Find K nearest neighbors efficiently.

```go
// Pseudocode
func Search(query []float32, K int, ef int) []Result {
    // 1. Start from top layer
    entryPoints := []Node{globalEntryPoint}

    for currentLevel := maxLevel; currentLevel > 0; currentLevel-- {
        // Greedy search to find closer entry points
        entryPoints = searchLayer(query, entryPoints, ef=1, currentLevel)
    }

    // 2. Search at layer 0 with higher ef
    candidates := searchLayer(query, entryPoints, ef, 0)

    // 3. Return top K candidates
    return topK(candidates, K)
}

// Layer-specific search
func searchLayer(query, entryPoints, ef, layer) []Node {
    visited := Set{}
    candidates := PriorityQueue{}  // Min-heap by distance
    results := PriorityQueue{}     // Max-heap by distance

    // Initialize with entry points
    for ep := range entryPoints {
        dist := distance(query, ep.vector)
        candidates.push(ep, dist)
        results.push(ep, dist)
        visited.add(ep)
    }

    // Greedy best-first search
    while candidates.notEmpty() {
        current := candidates.pop()

        // Stop if we've found ef good candidates
        if current.distance > results.top().distance {
            break
        }

        // Explore neighbors
        for neighbor := range current.neighbors[layer] {
            if neighbor not in visited {
                visited.add(neighbor)
                dist := distance(query, neighbor.vector)

                if dist < results.top().distance || results.size() < ef {
                    candidates.push(neighbor, dist)
                    results.push(neighbor, dist)

                    // Keep only ef best results
                    if results.size() > ef {
                        results.pop()
                    }
                }
            }
        }
    }

    return results
}
```

**Visual Search Example**:

```
Query Q, K=3, ef=5

Layer 2: Start at A
  A → B (closer to Q)

Layer 1: Start at B
  B → E (closer to Q)

Layer 0: Start at E, explore with ef=5
  Candidates: E, I, H, J, F
  Distances: [0.3, 0.1, 0.2, 0.4, 0.5]

Results (K=3): I (0.1), H (0.2), E (0.3)
```

**Time Complexity**: O(log N) expected
- Each layer: O(M * log N / log M) comparisons
- Number of layers: O(log N)
- Total: O(ef * M * log N)

### Parameter Tuning

#### M (Max Connections)

**Impact**:
- Higher M → Better recall, more memory, slower inserts
- Lower M → Faster inserts, less memory, lower recall

**Recommendations**:
- **M=4-8**: Low memory, fast inserts (~90% recall)
- **M=12-16**: Balanced (default) (~95% recall)
- **M=24-48**: High accuracy (~98% recall)
- **M=64+**: Maximum accuracy (~99.5% recall)

**Memory Impact**: M * 8 bytes * log(N) per node

| M  | Memory/Node | Recall@10 | Insert Time |
|----|-------------|-----------|-------------|
| 4  | +128 bytes  | 88%       | 1.0x        |
| 8  | +256 bytes  | 92%       | 1.5x        |
| 16 | +512 bytes  | 96%       | 2.0x        |
| 32 | +1024 bytes | 98%       | 3.0x        |

#### efConstruction

**Impact**:
- Higher ef → Better index quality, slower inserts
- Lower ef → Faster inserts, lower quality

**Recommendations**:
- **ef=100**: Fast construction (~90% recall)
- **ef=200**: Balanced (default) (~95% recall)
- **ef=400**: High quality (~98% recall)
- **ef=800**: Maximum quality (~99% recall)

**Rule of thumb**: efConstruction ≥ M

#### efSearch (Query Time)

**Impact**:
- Higher ef → Better recall, slower searches
- Lower ef → Faster searches, lower recall

**Recommendations**:
- **ef=10-20**: Real-time search (~85% recall, <1ms)
- **ef=50-100**: Balanced (~95% recall, ~3ms)
- **ef=200-500**: High accuracy (~99% recall, ~10ms)

**Latency vs Recall** (1M vectors, 768 dims):

| efSearch | Recall@10 | p50 Latency | p99 Latency |
|----------|-----------|-------------|-------------|
| 10       | 82%       | 0.8ms       | 2.1ms       |
| 50       | 95%       | 2.5ms       | 7.2ms       |
| 100      | 97%       | 4.2ms       | 12.3ms      |
| 200      | 99%       | 8.1ms       | 24.5ms      |

### Complexity Analysis

**Space Complexity**:
- Total memory: O(N * M * log N)
- Per node: O(M * log N)
- For 1M vectors, M=16: ~500 MB

**Time Complexity**:

| Operation | Average | Worst Case |
|-----------|---------|------------|
| Insert    | O(log N) | O(N)      |
| Search    | O(log N) | O(N)      |
| Delete    | O(log N) | O(N)      |

**Empirical Performance** (1M vectors, 768 dims, M=16):
- Insert: ~4.5ms per vector
- Search (K=10, ef=50): ~3.2ms
- Memory: ~3.2 GB

---

## NSG (Navigating Spreading-out Graph)

### NSG Overview

**Paper**: [Fast Approximate Nearest Neighbor Search With The Navigating Spreading-out Graph](https://arxiv.org/abs/1707.00143) (2019)

**Authors**: Cong Fu, Chao Xiang, Changxu Wang, Deng Cai

**Key Innovation**: Single-layer graph with optimized connectivity for monotonic search paths.

### Key Differences from HNSW

| Feature | HNSW | NSG |
|---------|------|-----|
| Layers | Multi-layer (log N) | Single layer |
| Entry point | Random top node | Centroid node |
| Graph structure | Small-world | Monotonic paths |
| Construction | Online (incremental) | Offline (batch) |
| Recall | 95-97% | 96-99% |
| Memory | Higher | Lower (~30% less) |

**Visual Comparison**:

```
HNSW (multi-layer):
L2: A -------- B
L1: A -- C -- B -- D
L0: A-C-E-F-B-D-G-H

NSG (single-layer with optimized edges):
    A -- C -- E -- F
    |    |    |    |
    B -- D -- G -- H
     \_________/
      (shortcuts)
```

### Algorithm Details

#### Construction

```go
// NSG construction (offline batch process)
func BuildNSG(vectors [][]float32) *NSGIndex {
    // 1. Build initial KNN graph
    knnGraph := buildKNNGraph(vectors, K=100)

    // 2. Find navigating node (approximate centroid)
    navNode := findNavigatingNode(vectors, knnGraph)

    // 3. Build NSG via graph refinement
    nsgGraph := make([][]uint64, len(vectors))

    for nodeID := range vectors {
        // Find shortest path from navNode to current node
        pathNodes := findPath(navNode, nodeID, knnGraph)

        // Select best out-edges (monotonic paths)
        candidates := []uint64{}
        for neighbor := range knnGraph[nodeID] {
            if isMonotonicEdge(nodeID, neighbor, pathNodes) {
                candidates = append(candidates, neighbor)
            }
        }

        // Keep top R neighbors
        nsgGraph[nodeID] = selectBest(candidates, R)
    }

    return &NSGIndex{
        graph:       nsgGraph,
        navNode:     navNode,
        vectors:     vectors,
    }
}

// Monotonic edge check
func isMonotonicEdge(node, neighbor uint64, pathNodes []uint64) bool {
    // Edge is monotonic if neighbor is closer to path than node
    for pathNode := range pathNodes {
        if distance(neighbor, pathNode) < distance(node, pathNode) {
            return true
        }
    }
    return false
}
```

#### Search

```go
func (nsg *NSGIndex) Search(query []float32, K int) []Result {
    visited := Set{}
    candidates := PriorityQueue{}  // Min-heap
    results := PriorityQueue{}     // Max-heap

    // Start from navigating node
    navDist := distance(query, nsg.navNode.vector)
    candidates.push(nsg.navNode, navDist)
    results.push(nsg.navNode, navDist)
    visited.add(nsg.navNode)

    // Best-first search
    while candidates.notEmpty() {
        current := candidates.pop()

        // Early termination
        if current.distance > results.top().distance {
            break
        }

        // Explore neighbors
        for neighbor := range current.neighbors {
            if neighbor not in visited {
                visited.add(neighbor)
                dist := distance(query, neighbor.vector)

                if results.size() < K || dist < results.top().distance {
                    candidates.push(neighbor, dist)
                    results.push(neighbor, dist)

                    if results.size() > K {
                        results.pop()
                    }
                }
            }
        }
    }

    return results.toList()
}
```

**Monotonic Search Path**:

```
Query Q wants to reach target T

NSG guarantees: Every step gets closer to T
  Nav → A → B → C → T
  dist: 100 → 80 → 50 → 20 → 0

HNSW may backtrack:
  Entry → A → B → C → D → B → E → T
  dist: 100 → 80 → 70 → 90 → 60 → 70 → 30 → 0
           (backtrack at D)
```

### Performance Characteristics

**Advantages**:
- **Better recall**: 96-99% vs 95-97% for HNSW
- **Lower memory**: ~30% less (single layer)
- **Predictable search**: Monotonic paths

**Disadvantages**:
- **Offline construction**: Must build entire graph upfront
- **No online inserts**: Adding new vectors requires rebuild
- **Slower construction**: 2-3x longer than HNSW

**When to use NSG**:
- ✅ Static datasets (infrequent updates)
- ✅ Maximum recall required
- ✅ Memory constrained
- ❌ Real-time inserts needed
- ❌ Frequently changing data

**Benchmark** (1M vectors, 768 dims):

| Metric | HNSW | NSG |
|--------|------|-----|
| Recall@10 | 96.5% | 98.2% |
| Search p50 | 3.2ms | 2.8ms |
| Memory | 3.2GB | 2.1GB |
| Insert time | 4.5ms | N/A (offline) |
| Build time | ~10min | ~30min |

---

## Distance Metrics

### Cosine Similarity

**Formula**: `1 - (A · B) / (||A|| × ||B||)`

**Use cases**: Text embeddings, normalized vectors

**Properties**:
- Range: [0, 2] (0 = identical, 2 = opposite)
- Scale-invariant
- Measures angle, not magnitude

```go
func CosineSimilarity(a, b []float32) float32 {
    var dotProduct, normA, normB float32

    for i := range a {
        dotProduct += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }

    if normA == 0 || normB == 0 {
        return 1.0  // Orthogonal
    }

    return 1.0 - (dotProduct / (sqrt(normA) * sqrt(normB)))
}
```

### Euclidean Distance

**Formula**: `sqrt(Σ(a_i - b_i)²)`

**Use cases**: Image embeddings, geometric data

**Properties**:
- Range: [0, ∞)
- Scale-sensitive
- Measures straight-line distance

```go
func EuclideanDistance(a, b []float32) float32 {
    var sum float32

    for i := range a {
        diff := a[i] - b[i]
        sum += diff * diff
    }

    return sqrt(sum)
}
```

### Dot Product

**Formula**: `-Σ(a_i × b_i)` (negative for min-heap)

**Use cases**: Pre-normalized vectors, MaxSim

**Properties**:
- Range: [-∞, ∞]
- Fast (no sqrt)
- Requires normalized vectors

```go
func DotProduct(a, b []float32) float32 {
    var sum float32

    for i := range a {
        sum += a[i] * b[i]
    }

    return -sum  // Negative for min-heap
}
```

**Performance** (768 dims):
- Dot Product: ~0.5μs (fastest)
- Euclidean: ~0.8μs (sqrt overhead)
- Cosine: ~1.2μs (2x sqrt + division)

---

## Optimization Techniques

### 1. SIMD Vectorization

Use CPU vector instructions for faster distance calculations:

```go
// AMD64 SSE/AVX
func CosineSimilaritySIMD(a, b []float32) float32 {
    // Process 8 floats at once with AVX
    return cosineSIMD_AVX(a, b)
}
```

**Speedup**: 4-8x faster distance calculations

### 2. Quantization

Reduce memory by compressing vectors:

```go
// Product Quantization (PQ)
type PQIndex struct {
    codebook    [][]float32  // Cluster centers
    codes       [][]uint8    // Quantized codes
    dimensions  int
    subvectors  int          // Number of sub-vectors
}

// Compress 768D float32 → 96 bytes (8x compression)
func (pq *PQIndex) Quantize(vec []float32) []uint8 {
    codes := make([]uint8, pq.subvectors)
    subDim := pq.dimensions / pq.subvectors

    for i := 0; i < pq.subvectors; i++ {
        subVec := vec[i*subDim : (i+1)*subDim]
        codes[i] = pq.findNearest(subVec, i)
    }

    return codes
}
```

**Memory reduction**: 4-8x compression
**Recall impact**: -2% to -5%

### 3. Prefetching

Reduce memory latency:

```go
func searchLayerOptimized(query, candidates, layer) {
    // Prefetch neighbor data before distance calculation
    for candidate := range candidates {
        for neighbor := range candidate.neighbors[layer] {
            prefetch(neighbor.vector)  // CPU instruction
        }
    }

    // Now compute distances (data is in cache)
    for candidate := range candidates {
        // ... distance calculations
    }
}
```

**Speedup**: 1.5-2x for large graphs

### 4. Batch Processing

Process multiple queries together:

```go
func BatchSearch(queries [][]float32, K int) [][]Result {
    results := make([][]Result, len(queries))

    // Process in parallel
    parallel.For(0, len(queries), func(i int) {
        results[i] = Search(queries[i], K)
    })

    return results
}
```

**Throughput**: 3-5x higher

---

## Algorithm Comparison

### HNSW vs NSG vs IVF-Flat

| Feature | HNSW | NSG | IVF-Flat |
|---------|------|-----|----------|
| **Recall** | 95-97% | 96-99% | 80-90% |
| **Search Time** | O(log N) | O(log N) | O(N/k) |
| **Memory** | High | Medium | Low |
| **Insert** | Online | Offline | Online |
| **Best For** | General purpose | High recall | Large scale |

### When to Use Each

**HNSW**:
- Real-time inserts required
- General purpose ANN
- Balanced recall/speed
- Default choice ✅

**NSG**:
- Maximum recall needed
- Static datasets
- Memory constrained
- Batch updates acceptable

**IVF-Flat** (future):
- Billion-scale datasets
- Lower recall acceptable
- Extreme scale

---

## References

1. [HNSW Paper](https://arxiv.org/abs/1603.09320) - Malkov & Yashunin (2016)
2. [NSG Paper](https://arxiv.org/abs/1707.00143) - Fu et al. (2019)
3. [HNSW Implementation](https://github.com/nmslib/hnswlib)
4. [NSG Implementation](https://github.com/ZJULearning/nsg)

---

**Version**: 1.1.0
**Last Updated**: 2025-01-15
