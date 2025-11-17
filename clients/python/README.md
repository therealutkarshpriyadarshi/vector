# Vector Database Python Client

Python client library for the Vector Database with HNSW and NSG indexing.

## Installation

```bash
# Install from source
cd clients/python
pip install -e .

# Or install from PyPI (when published)
pip install vector-db-client
```

## Quick Start

```python
from vector_db import VectorDBClient

# Connect to server
client = VectorDBClient("localhost:50051")

# Insert vectors
vector_id = client.insert(
    namespace="default",
    vector=[0.1, 0.2, 0.3, 0.4, ...],
    metadata={"title": "Vector Database Tutorial", "category": "tech"}
)

# Search for similar vectors
results = client.search(
    namespace="default",
    query_vector=[0.1, 0.2, 0.3, 0.4, ...],
    k=10
)

for result in results:
    print(f"ID: {result.id}, Distance: {result.distance}")

# Close connection
client.close()
```

## Usage

### Context Manager

```python
with VectorDBClient("localhost:50051") as client:
    results = client.search(...)
```

### Batch Insert

```python
vectors = [
    ([0.1, 0.2, 0.3], {"title": "Doc 1"}),
    ([0.4, 0.5, 0.6], {"title": "Doc 2"}),
    ([0.7, 0.8, 0.9], {"title": "Doc 3"}),
]

result = client.batch_insert("default", vectors)
print(f"Inserted {result['inserted_count']} vectors in {result['total_time_ms']}ms")
```

### Hybrid Search

```python
results = client.hybrid_search(
    namespace="default",
    query_vector=[0.1, 0.2, 0.3, ...],
    query_text="machine learning vector database",
    k=20,
    vector_weight=0.7,
    text_weight=0.3
)
```

### TLS Connection

```python
client = VectorDBClient(
    address="localhost:50051",
    use_tls=True,
    cert_file="/path/to/server.crt"
)
```

### Get Statistics

```python
stats = client.get_stats()
print(f"Total vectors: {stats['total_vectors']}")
print(f"Memory usage: {stats['memory_usage_bytes'] / 1024 / 1024:.2f} MB")
```

### Health Check

```python
health = client.health_check()
print(f"Status: {health['status']}, Version: {health['version']}")
```

## API Reference

### VectorDBClient

#### `__init__(address, use_tls=False, cert_file=None)`

Create a new client instance.

- `address`: Server address (e.g., "localhost:50051")
- `use_tls`: Whether to use TLS encryption
- `cert_file`: Path to TLS certificate file

#### `insert(namespace, vector, metadata=None, text=None, id=None)`

Insert a single vector.

- `namespace`: Namespace for multi-tenancy
- `vector`: List of floats
- `metadata`: Optional dict of metadata
- `text`: Optional text content for full-text search
- `id`: Optional custom ID

Returns: Vector ID (string)

#### `search(namespace, query_vector, k=10, ef_search=50, filter_dict=None, distance_metric="cosine")`

Search for K nearest neighbors.

- `namespace`: Namespace to search in
- `query_vector`: Query vector (list of floats)
- `k`: Number of results
- `ef_search`: HNSW search accuracy (10-200)
- `distance_metric`: "cosine", "euclidean", or "dot_product"

Returns: List of SearchResult objects

#### `hybrid_search(namespace, query_vector, query_text, k=10, ...)`

Hybrid search with vector + full-text.

Returns: List of SearchResult objects

#### `batch_insert(namespace, vectors)`

Insert multiple vectors efficiently.

- `vectors`: List of (vector, metadata) tuples

Returns: Dict with inserted_count, failed_count, etc.

#### `update(namespace, id, vector=None, metadata=None, text=None)`

Update an existing vector.

Returns: Boolean

#### `delete(namespace, id)`

Delete a vector by ID.

Returns: Number of deleted vectors

#### `get_stats(namespace=None)`

Get database statistics.

Returns: Dict with statistics

#### `health_check()`

Check server health.

Returns: Dict with health status

## Examples

See `examples/` directory for more examples:

- `basic_usage.py` - Basic insert and search
- `batch_operations.py` - Batch insert operations
- `hybrid_search.py` - Hybrid search example
- `rag_system.py` - RAG system implementation

## Development

```bash
# Install dev dependencies
pip install -e ".[dev]"

# Run tests
pytest

# Format code
black vector_db/

# Type checking
mypy vector_db/
```

## License

MIT License

## Support

- GitHub Issues: https://github.com/therealutkarshpriyadarshi/vector/issues
- Documentation: https://docs.vectordb.example.com
