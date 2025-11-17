# Vector Database with HNSW Indexing

A production-grade vector database implementing **HNSW (Hierarchical Navigable Small World)** indexing, hybrid search, and multi-tenancy features. Built in **Go** for rapid development and operational simplicity.

## 🎯 Project Overview

This project demonstrates advanced systems programming by building a specialized vector database from scratch. Vector databases are critical infrastructure for modern AI applications including:
- 🤖 **RAG (Retrieval-Augmented Generation)** systems
- 🔍 **Semantic search** engines
- 🖼️ **Image similarity** search
- 📚 **Document retrieval** systems
- 💬 **Recommendation** engines

## ✨ Key Features

### Core Features
- ✅ **HNSW Indexing** - Fast approximate nearest neighbor search (100-1000x faster than brute force)
- ✅ **Hybrid Search** - Combine vector similarity with BM25 full-text search using Reciprocal Rank Fusion
- ✅ **Multiple Distance Metrics** - Cosine similarity, Euclidean distance, Dot product
- ✅ **Dynamic Updates** - Add, update, and delete vectors without full rebuild
- ✅ **Persistent Storage** - BadgerDB for efficient key-value storage

### Production Features
- ✅ **Multi-tenancy** - Namespace isolation for multiple users/applications
- ✅ **Advanced Filtering** - Metadata filters (equals, range, geo-radius, composite)
- ✅ **Batch Operations** - Bulk insert/update for efficiency
- ✅ **Query Caching** - LRU cache for frequent queries
- ✅ **gRPC API** - High-performance API with Protocol Buffers
- ✅ **Quantization** - Memory optimization with scalar/product quantization

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      gRPC API                            │
│  Insert | Search | HybridSearch | Delete | BatchInsert  │
└────────────────────┬────────────────────────────────────┘
                     │
        ┌────────────┴────────────┐
        │                         │
   ┌────▼─────┐           ┌──────▼──────┐
   │  HNSW    │           │  Full-Text   │
   │  Index   │           │  Search      │
   │ (Vector) │           │  (BM25)      │
   └────┬─────┘           └──────┬───────┘
        │                        │
        └────────────┬───────────┘
                     │
              ┌──────▼────────┐
              │ Hybrid Search │
              │     (RRF)     │
              └───────┬───────┘
                      │
              ┌───────▼────────┐
              │   BadgerDB     │
              │  (Persistent)  │
              └────────────────┘
```

## 🚀 Quick Start

### Prerequisites
- Go 1.21 or later
- Protocol Buffers compiler (`protoc`)

### Installation

```bash
# Clone the repository
git clone https://github.com/therealutkarshpriyadarshi/vector.git
cd vector

# Initialize project and install dependencies
make init

# Build the server
make build

# Run tests
make test

# Run the server
make run
```

## 📚 Documentation

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System architecture and design decisions
- **[IMPLEMENTATION_GUIDE.md](IMPLEMENTATION_GUIDE.md)** - Step-by-step implementation guide
- **[PROJECT_STRUCTURE.md](PROJECT_STRUCTURE.md)** - Directory structure and organization
- **[GO_VS_RUST.md](GO_VS_RUST.md)** - Language choice analysis

## 🧪 Testing

```bash
# Run unit tests
make test

# Run benchmarks
make bench

# Test recall accuracy
make recall-test

# Run integration tests
make integration

# Generate coverage report
make coverage
```

## 📊 Performance

**Target Performance** (1M vectors, 768 dimensions):
- **Latency**: <10ms (p95)
- **Throughput**: >1000 QPS
- **Recall@10**: >95% vs brute force
- **Memory**: <1GB for 1M vectors

## 🛠️ Tech Stack

| Component | Technology | Why |
|-----------|-----------|-----|
| **Language** | Go 1.21+ | Fast development, great concurrency, operational simplicity |
| **Storage** | BadgerDB | Pure Go, high performance, embedded KV store |
| **Full-Text** | Bleve | Go-native BM25 implementation for hybrid search |
| **API** | gRPC | High-performance RPC with Protocol Buffers |
| **Indexing** | HNSW | State-of-the-art approximate nearest neighbor search |

## 🗓️ Development Timeline

This project follows a **6-week development plan**:

| Week | Focus | Deliverables |
|------|-------|--------------|
| **1-2** | Core HNSW | Index implementation, insert/search, distance metrics |
| **3** | Hybrid Search | Bleve integration, RRF, metadata filtering |
| **4** | API Layer | gRPC server, handlers, protocol buffers |
| **5** | Production | Multi-tenancy, caching, batch ops, optimizations |
| **6** | Polish | Testing, benchmarking, documentation, examples |

See [IMPLEMENTATION_GUIDE.md](IMPLEMENTATION_GUIDE.md) for detailed week-by-week tasks.

## 🎓 Learning Outcomes

By building this project, you'll learn:

1. **Advanced Algorithms**
   - HNSW approximate nearest neighbor search
   - Graph-based indexing structures
   - Reciprocal Rank Fusion for hybrid ranking

2. **Systems Programming**
   - Persistent storage design
   - Concurrent data structures
   - Memory optimization techniques

3. **Backend Engineering**
   - Multi-tenant architecture
   - gRPC API design
   - Performance profiling and optimization

4. **Production Systems**
   - Benchmarking and testing strategies
   - Query optimization and caching
   - Operational monitoring

## 🔍 Why HNSW?

HNSW (Hierarchical Navigable Small World) is the state-of-the-art algorithm for approximate nearest neighbor search:

- **Fast**: 100-1000x faster than brute force
- **Accurate**: >95% recall in practice
- **Scalable**: Handles millions to billions of vectors
- **Dynamic**: Supports insertions and deletions

**Used by**: Weaviate, Qdrant, Milvus, Pinecone, and others.

## 📖 Example Usage

```go
package main

import "github.com/therealutkarshpriyadarshi/vector/pkg/hnsw"

func main() {
    // Create index
    idx := hnsw.New(16, 200) // M=16, efConstruction=200

    // Insert vectors
    idx.Insert([]float32{0.1, 0.2, 0.3}, map[string]interface{}{
        "title": "Document 1",
    })

    // Search
    results := idx.Search([]float32{0.15, 0.25, 0.35}, 10, 50)

    for _, res := range results {
        fmt.Printf("ID: %d, Distance: %.4f\n", res.ID, res.Distance)
    }
}
```

## 🌟 Project Highlights

### Why This Project Stands Out

1. **Real-World Relevance** - Vector databases are critical for modern AI (RAG, semantic search)
2. **Technical Depth** - Implements complex algorithms (HNSW) from scratch
3. **Production Quality** - Includes multi-tenancy, persistence, API design
4. **Performance Focus** - Optimization, benchmarking, profiling
5. **Beyond CRUD** - Shows system design and algorithmic thinking

### Competitive Advantages

- ✅ Go implementation easier to understand than Qdrant (Rust) or Faiss (C++)
- ✅ Hybrid search outperforms pure vector search in benchmarks
- ✅ Production features (multi-tenancy, filtering) show enterprise thinking
- ✅ Complete system (storage, API, indexing) demonstrates full-stack skills

## 🔬 Real-World Comparisons

| Feature | This Project | Qdrant | Weaviate | pgvector |
|---------|--------------|--------|----------|----------|
| **Language** | Go | Rust | Go | PostgreSQL |
| **HNSW** | ✅ | ✅ | ✅ | ✅ |
| **Hybrid Search** | ✅ | ✅ | ✅ | ❌ |
| **Multi-tenancy** | ✅ | ✅ | ✅ | ⚠️ |
| **Production Ready** | 🎯 Goal | ✅ | ✅ | ✅ |

## 🤝 Contributing

This is a personal learning project, but suggestions and feedback are welcome!

## 📄 License

MIT License - see LICENSE file for details

## 🙏 Acknowledgments

- **HNSW Paper**: Malkov & Yashunin (2018)
- **Weaviate**: Inspiration for Go-based vector DB
- **Qdrant**: Production features reference
- **hnswlib**: C++ reference implementation

## 📚 Further Reading

- [HNSW Paper](https://arxiv.org/abs/1603.09320)
- [Weaviate Blog](https://weaviate.io/blog)
- [Pinecone Learning Center](https://www.pinecone.io/learn/)
- [Approximate Nearest Neighbors - Oh Yeah!](http://ann-benchmarks.com/)

---

**Built with ❤️ and lots of ☕**

*Vector databases are the future of AI infrastructure. Let's build one from scratch!*