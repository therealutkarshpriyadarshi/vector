"""
Basic Usage Example for Vector Database Python Client
"""

import random
from vector_db import VectorDBClient


def generate_random_vector(dim=768):
    """Generate a random vector for testing"""
    return [random.random() for _ in range(dim)]


def main():
    # Connect to Vector Database
    print("Connecting to Vector Database...")
    client = VectorDBClient("localhost:50051")

    # Check server health
    health = client.health_check()
    print(f"Server status: {health['status']}, Version: {health['version']}")

    # Insert vectors
    print("\nInserting vectors...")
    doc_ids = []

    documents = [
        {"title": "HNSW Algorithm", "category": "algorithms", "text": "HNSW is a graph-based ANN algorithm"},
        {"title": "Vector Database", "category": "databases", "text": "A database optimized for vector similarity search"},
        {"title": "Machine Learning", "category": "ml", "text": "ML is a subset of artificial intelligence"},
        {"title": "Neural Networks", "category": "ml", "text": "Neural networks are inspired by biological neurons"},
        {"title": "Search Algorithms", "category": "algorithms", "text": "Algorithms for finding information efficiently"},
    ]

    for doc in documents:
        vector = generate_random_vector(128)  # Generate 128-dim vector
        doc_id = client.insert(
            namespace="default",
            vector=vector,
            metadata={"title": doc["title"], "category": doc["category"]},
            text=doc["text"]
        )
        doc_ids.append(doc_id)
        print(f"  Inserted: {doc['title']} (ID: {doc_id})")

    # Search for similar vectors
    print("\nSearching for similar vectors...")
    query_vector = generate_random_vector(128)
    results = client.search(
        namespace="default",
        query_vector=query_vector,
        k=3,
        ef_search=50
    )

    print(f"Found {len(results)} results:")
    for i, result in enumerate(results, 1):
        print(f"  {i}. ID={result.id}, Distance={result.distance:.4f}")
        if result.metadata:
            print(f"     Title: {result.metadata.get('title')}")
            print(f"     Category: {result.metadata.get('category')}")

    # Get statistics
    print("\nDatabase statistics:")
    stats = client.get_stats()
    print(f"  Total vectors: {stats['total_vectors']}")
    print(f"  Total namespaces: {stats['total_namespaces']}")
    print(f"  Memory usage: {stats['memory_usage_bytes'] / 1024 / 1024:.2f} MB")

    # Close connection
    client.close()
    print("\nDone!")


if __name__ == "__main__":
    main()
