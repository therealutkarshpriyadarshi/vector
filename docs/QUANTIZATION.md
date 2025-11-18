# Advanced Quantization and Indexing

This document describes the advanced quantization and indexing methods implemented in the vector database.

## Table of Contents

1. [Overview](#overview)
2. [Quantization Methods](#quantization-methods)
3. [Index Types](#index-types)
4. [Performance Comparison](#performance-comparison)
5. [Usage Examples](#usage-examples)
6. [Configuration Guide](#configuration-guide)

## Overview

The vector database now supports multiple quantization methods and index types, providing flexible trade-offs between:

- **Memory usage** (compression ratio)
- **Search speed** (QPS)
- **Search accuracy** (recall@k)
- **Index build time**

### Quick Comparison

| Method | Compression | Recall@10 | Use Case |
|--------|-------------|-----------|----------|
| **Scalar Quantization** | 4x | ~90% | Fast, simple compression |
| **Product Quantization (PQ)** | 8-256x | 70-90% | High compression, good for large datasets |
| **IVF-Flat** | 1x | ~98% | Fast search with filtering |
| **IVF-PQ** | 8-256x | 70-90% | Best of IVF + PQ |
| **SCANN** | 8-256x | 75-95% | State-of-the-art, highest recall |

## Quantization Methods

### 1. Scalar Quantization

**Concept**: Linearly quantize float32 values to int8 (1 byte per dimension).

**Advantages**:
- Simple and fast
- 4x memory reduction
- High recall (~90%)
- No training required (just min/max computation)

**Disadvantages**:
- Limited compression ratio
- Not suitable for very large datasets

**Example**:
```go
import "github.com/therealutkarshpriyadarshi/vector/internal/quantization"

// Create quantizer
sq := quantization.NewScalarQuantizer()

// Train on sample data
err := sq.Train(trainingVectors)

// Quantize vectors
quantized := sq.Quantize(vector)

// Search using quantized distance
dist := quantization.DistanceInt8(queryQuantized, vectorQuantized)
```

**Memory**: 768-dim vector: 3072 bytes → 768 bytes (4x compression)

---

### 2. Product Quantization (PQ)

**Concept**: Divide vectors into subvectors and cluster each independently using k-means. Each vector is represented by m codes (1 byte each).

**Advantages**:
- High compression ratios (8-256x)
- Asymmetric distance computation for fast search
- Flexible compression via numSubvectors parameter
- Minimal recall loss with proper configuration

**Disadvantages**:
- Requires training on representative data
- Training can be slow for large codebooks
- Lower recall than scalar quantization

**Example**:
```go
import "github.com/therealutkarshpriyadarshi/vector/internal/quantization"

// Create PQ: 16 subvectors, 8 bits per code
// 768-dim → 16 bytes = 192x compression
pq := quantization.NewProductQuantizer(16, 8)

// Train on sample data
err := pq.Train(trainingVectors)

// Encode vector
codes := pq.Encode(vector)  // 16 bytes

// Fast search with asymmetric distance
distTable := pq.ComputeDistanceTable(query)
for _, code := range encodedDatabase {
    dist := pq.AsymmetricDistance(distTable, code)
}
```

**Compression Configurations**:
- `PQ(8, 8)`: 8 bytes per vector → 384x compression for 768-dim
- `PQ(16, 8)`: 16 bytes per vector → 192x compression
- `PQ(32, 8)`: 32 bytes per vector → 96x compression

**Recommended**: `PQ(16, 8)` for balanced compression and recall.

---

### 3. Anisotropic Quantization (SCANN)

**Concept**: Enhanced PQ that adapts to data distribution. Used in Google's SCANN algorithm.

**Advantages**:
- Better recall than standard PQ (10-20% improvement)
- Optimized for angular similarity
- Learned projections align with data structure

**Disadvantages**:
- More complex training
- Slightly slower encoding

**Usage**: Automatically used in SCANN index (see below).

---

## Index Types

### 1. IVF-Flat

**Concept**: Inverted File index with uncompressed vectors. Partitions space using k-means, then searches only nearby partitions.

**Advantages**:
- Fast search with pruning
- No compression (maintains accuracy)
- Excellent for filtered searches
- Simple and robust

**Disadvantages**:
- Same memory usage as brute force
- Requires choosing good number of centroids

**Example**:
```go
import (
    "github.com/therealutkarshpriyadarshi/vector/pkg/ivf"
    "github.com/therealutkarshpriyadarshi/vector/internal/quantization"
)

// Configure index
config := ivf.Config{
    NumCentroids: 100,  // sqrt(N) is typical
    Metric:       quantization.EuclideanDistance,
}

index := ivf.NewIVFFlat(config)

// Train on sample data
err := index.Train(trainingVectors)

// Add vectors
ids := []int{0, 1, 2, ...}
err = index.Add(vectors, ids, nil)

// Search with nprobe=10 (search 10 nearest centroids)
resultIDs, distances, err := index.Search(query, k=10, nprobe=10)

// Filtered search
filter := func(metadata map[string]interface{}) bool {
    category := metadata["category"].(string)
    return category == "news"
}
resultIDs, distances, err := index.SearchWithFilter(query, 10, 10, filter)
```

**Recommended nprobe**:
- `nprobe=1`: Fastest, ~50% recall
- `nprobe=10`: Balanced, ~85% recall
- `nprobe=20`: Slower, ~95% recall

---

### 2. IVF-PQ

**Concept**: Combines IVF partitioning with Product Quantization compression.

**Advantages**:
- High compression (same as PQ)
- Fast search (partition pruning)
- Best of both worlds
- Industry standard for billion-scale search

**Disadvantages**:
- Training requires two stages (IVF + PQ)
- Slightly lower recall than IVF-Flat

**Example**:
```go
import (
    "github.com/therealutkarshpriyadarshi/vector/pkg/ivf"
    "github.com/therealutkarshpriyadarshi/vector/internal/quantization"
)

config := ivf.ConfigPQ{
    NumCentroids:  100,  // IVF centroids
    NumSubvectors: 16,   // PQ subvectors
    BitsPerCode:   8,    // PQ bits per code
    Metric:        quantization.EuclideanDistance,
}

index := ivf.NewIVFPQ(config)

// Train (learns both IVF clustering and PQ codebooks)
err := index.Train(trainingVectors)

// Add vectors (automatically compressed)
err = index.Add(vectors, ids, nil)

// Search
resultIDs, distances, err := index.Search(query, 10, 10)
```

**Memory**: 768-dim, 1M vectors
- Original: 1M × 768 × 4 = 3GB
- IVF-PQ(16, 8): 1M × 16 = 16MB (~192x compression!)

---

### 3. SCANN

**Concept**: Google's state-of-the-art algorithm with:
1. **Learned partitioning** (spherical k-means for angular similarity)
2. **Anisotropic quantization** (adapts to data distribution)
3. **Multi-stage scoring** (coarse → fine for accuracy)

**Advantages**:
- Highest recall for same memory/speed
- Optimized for cosine similarity
- Beats IVF-PQ by 10-20% recall
- Production-tested at Google scale

**Disadvantages**:
- More complex implementation
- Longer training time
- Best for cosine/dot product distance

**Example**:
```go
import (
    "github.com/therealutkarshpriyadarshi/vector/pkg/scann"
    "github.com/therealutkarshpriyadarshi/vector/internal/quantization"
)

// Use default config or customize
config := scann.DefaultConfig()
config.NumPartitions = 100
config.NumSubvectors = 16
config.BitsPerCode = 8
config.SphericalKM = true  // Recommended for embeddings

index := scann.NewSCANN(config)

// Train (learns partitions + quantizer)
err := index.Train(trainingVectors)

// Add vectors
err = index.Add(vectors, ids, nil)

// Search
resultIDs, distances, err := index.Search(query, 10, 10)
```

**When to use SCANN**:
- Semantic search with embeddings
- Cosine/dot product similarity
- Need highest possible recall
- Have budget for training time

---

## Performance Comparison

### Compression Ratios

For 768-dimensional vectors (e.g., text embeddings):

| Method | Bytes/Vector | Compression | 1M Vectors |
|--------|--------------|-------------|------------|
| Original (float32) | 3,072 | 1x | 3.0 GB |
| Scalar Quantization | 768 | 4x | 768 MB |
| PQ(8, 8) | 8 | 384x | 8 MB |
| PQ(16, 8) | 16 | 192x | 16 MB |
| PQ(32, 8) | 32 | 96x | 32 MB |
| IVF-Flat | 3,072 | 1x | 3.0 GB |
| IVF-PQ(16, 8) | 16 | 192x | 16 MB |
| SCANN(16, 8) | 16 | 192x | 16 MB |

### Recall vs Speed Trade-offs

Typical results on 1M 768-dim vectors, k=10:

| Method | QPS | Recall@10 | Memory |
|--------|-----|-----------|--------|
| Brute Force | 100 | 100% | 3.0 GB |
| Scalar Quant | 500 | 90% | 768 MB |
| IVF-Flat (nprobe=10) | 5,000 | 85% | 3.0 GB |
| IVF-PQ (nprobe=10) | 8,000 | 75% | 16 MB |
| SCANN (nprobe=10) | 7,000 | 85% | 16 MB |

**Key insight**: SCANN achieves same recall as IVF-Flat (85%) with 192x less memory!

---

## Usage Examples

### Example 1: Maximum Compression

Use case: Billion-scale search with limited memory

```go
// Use PQ with minimal bytes per vector
pq := quantization.NewProductQuantizer(8, 6)  // 8 bytes, 64 clusters
pq.Train(trainingData)

// Or use IVF-PQ for faster search
config := ivf.ConfigPQ{
    NumCentroids:  200,
    NumSubvectors: 8,
    BitsPerCode:   6,
}
index := ivf.NewIVFPQ(config)
```

**Result**: 768-dim → 8 bytes (384x compression)

---

### Example 2: Maximum Recall

Use case: Critical search application where accuracy matters

```go
// Use SCANN with high nprobe
config := scann.DefaultConfig()
config.NumPartitions = 200
config.NumSubvectors = 32  // More subvectors = better accuracy
config.BitsPerCode = 8

index := scann.NewSCANN(config)
index.Train(trainingData)

// Search with higher nprobe
results, _, _ := index.Search(query, 10, nprobe=30)
```

**Result**: ~95% recall@10 with 96x compression

---

### Example 3: Filtered Search

Use case: E-commerce with category filtering

```go
// IVF-Flat excels at filtered search
config := ivf.Config{NumCentroids: 100}
index := ivf.NewIVFFlat(config)

// Add with metadata
metadata := []map[string]interface{}{
    {"category": "electronics", "price": 299},
    {"category": "books", "price": 15},
}
index.Add(vectors, ids, metadata)

// Filter by category and price
filter := func(meta map[string]interface{}) bool {
    return meta["category"] == "electronics" &&
           meta["price"].(int) < 500
}

results, _, _ := index.SearchWithFilter(query, 10, 10, filter)
```

---

## Configuration Guide

### Choosing Number of Centroids (IVF, SCANN)

Rule of thumb: `numCentroids = sqrt(N)` where N = dataset size

| Dataset Size | NumCentroids |
|--------------|--------------|
| 1,000 | 30 |
| 10,000 | 100 |
| 100,000 | 300 |
| 1,000,000 | 1,000 |
| 10,000,000 | 3,000 |

### Choosing PQ Parameters

**numSubvectors**: Controls compression and accuracy
- More subvectors = less compression but better accuracy
- Must divide vector dimension evenly
- Typical: 8, 16, 32

**bitsPerCode**: Controls codebook size
- Higher bits = larger codebooks but better accuracy
- Typical: 6 (64 clusters), 7 (128), 8 (256)
- Memory: codebook size = m × 2^k × (d/m) × 4 bytes

### Choosing nprobe

Balance between speed and recall:

| nprobe | Recall | Speed | Use Case |
|--------|--------|-------|----------|
| 1 | 40-60% | Fastest | First-stage ranking |
| 5 | 70-80% | Fast | Balanced |
| 10 | 80-90% | Medium | Recommended default |
| 20 | 90-95% | Slower | High accuracy needed |
| 50+ | 95-98% | Slow | Near-exact search |

---

## Best Practices

### 1. Training Data

- Use representative sample of your dataset
- Minimum: 10,000 vectors for PQ training
- More data = better codebooks
- Shuffle data before sampling

### 2. Distance Metric

- **Cosine similarity**: Use `CosineDistance` or SCANN with spherical k-means
- **Euclidean distance**: Use `EuclideanDistance`
- **Maximum Inner Product**: Use `DotProductDistance`

### 3. Testing & Benchmarking

```bash
# Run quantization comparison
go test -v -run TestQuantizationComparison ./test/benchmarks/

# Run index comparison
go test -v -run TestIndexComparison ./test/benchmarks/

# Benchmark specific configuration
go test -bench=BenchmarkIVFPQ_Search ./pkg/ivf/
```

### 4. Production Deployment

1. **Start with IVF-PQ**: Best balance of compression, speed, recall
2. **Tune nprobe**: Profile with your query distribution
3. **Monitor recall**: Compare with ground truth on sample queries
4. **Consider SCANN**: If recall is critical and you can afford training time

---

## References

- [Product Quantization for Nearest Neighbor Search](https://lear.inrialpes.fr/pubs/2011/JDS11/jegou_searching_with_quantization.pdf)
- [SCANN: Efficient Vector Similarity Search](https://arxiv.org/abs/1908.10396)
- [IVF-PQ in FAISS](https://github.com/facebookresearch/faiss/wiki/Faster-search)

---

## API Reference

See also:
- [internal/quantization/quantizer.go](../internal/quantization/quantizer.go) - Common interfaces
- [internal/quantization/product.go](../internal/quantization/product.go) - Product Quantization
- [pkg/ivf/](../pkg/ivf/) - IVF-Flat and IVF-PQ indexes
- [pkg/scann/](../pkg/scann/) - SCANN index
