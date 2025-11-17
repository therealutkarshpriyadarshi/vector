"""
Vector Database Python Client

Main client for interacting with the Vector Database gRPC API.
"""

import grpc
from typing import List, Dict, Optional, Tuple
from dataclasses import dataclass
import sys
import os

# Add proto path for imports
sys.path.append(os.path.join(os.path.dirname(__file__), '..', '..', '..', 'pkg', 'api', 'grpc', 'proto'))

try:
    import vector_pb2
    import vector_pb2_grpc
except ImportError:
    print("Error: gRPC proto files not found. Please generate them first:")
    print("  cd pkg/api/grpc/proto && python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. vector.proto")
    raise


@dataclass
class SearchResult:
    """Search result with ID, distance, and optional vector/metadata"""
    id: str
    distance: float
    vector: Optional[List[float]] = None
    metadata: Optional[Dict[str, str]] = None
    text: Optional[str] = None
    vector_score: Optional[float] = None
    text_score: Optional[float] = None


class VectorDBClient:
    """
    Vector Database Client

    A Python client for interacting with the Vector Database gRPC API.

    Example:
        >>> client = VectorDBClient("localhost:50051")
        >>> vector_id = client.insert(
        ...     namespace="default",
        ...     vector=[0.1, 0.2, 0.3, ...],
        ...     metadata={"title": "Example"}
        ... )
        >>> results = client.search(
        ...     namespace="default",
        ...     query_vector=[0.1, 0.2, 0.3, ...],
        ...     k=10
        ... )
    """

    def __init__(self, address: str = "localhost:50051", use_tls: bool = False,
                 cert_file: Optional[str] = None):
        """
        Initialize Vector Database client

        Args:
            address: Server address (host:port)
            use_tls: Whether to use TLS encryption
            cert_file: Path to TLS certificate file (if use_tls=True)
        """
        self.address = address

        if use_tls:
            if cert_file:
                with open(cert_file, 'rb') as f:
                    credentials = grpc.ssl_channel_credentials(f.read())
            else:
                credentials = grpc.ssl_channel_credentials()
            self.channel = grpc.secure_channel(address, credentials)
        else:
            self.channel = grpc.insecure_channel(address)

        self.stub = vector_pb2_grpc.VectorDBStub(self.channel)

    def close(self):
        """Close the gRPC channel"""
        self.channel.close()

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()

    def insert(self, namespace: str, vector: List[float],
               metadata: Optional[Dict[str, str]] = None,
               text: Optional[str] = None,
               id: Optional[str] = None) -> str:
        """
        Insert a single vector

        Args:
            namespace: Namespace for multi-tenancy
            vector: Vector embedding (list of floats)
            metadata: Optional metadata key-value pairs
            text: Optional text content for full-text search
            id: Optional custom ID (auto-generated if not provided)

        Returns:
            ID of inserted vector

        Example:
            >>> id = client.insert(
            ...     namespace="default",
            ...     vector=[0.1, 0.2, 0.3, 0.4],
            ...     metadata={"category": "tech", "title": "Vector DB"}
            ... )
        """
        request = vector_pb2.InsertRequest(
            namespace=namespace,
            vector=vector,
            metadata=metadata or {},
            text=text,
            id=id
        )

        response = self.stub.Insert(request)

        if not response.success:
            raise Exception(f"Insert failed: {response.error}")

        return response.id

    def search(self, namespace: str, query_vector: List[float], k: int = 10,
               ef_search: int = 50,
               filter_dict: Optional[Dict] = None,
               distance_metric: str = "cosine") -> List[SearchResult]:
        """
        Search for K nearest neighbors

        Args:
            namespace: Namespace to search in
            query_vector: Query vector embedding
            k: Number of results to return
            ef_search: HNSW ef_search parameter (accuracy vs speed)
            filter_dict: Optional metadata filter
            distance_metric: "cosine", "euclidean", or "dot_product"

        Returns:
            List of SearchResult objects

        Example:
            >>> results = client.search(
            ...     namespace="default",
            ...     query_vector=[0.1, 0.2, 0.3, 0.4],
            ...     k=10,
            ...     ef_search=100
            ... )
            >>> for result in results:
            ...     print(f"ID: {result.id}, Distance: {result.distance}")
        """
        request = vector_pb2.SearchRequest(
            namespace=namespace,
            query_vector=query_vector,
            k=k,
            ef_search=ef_search,
            distance_metric=distance_metric
        )

        # TODO: Add filter support

        response = self.stub.Search(request)

        if response.error:
            raise Exception(f"Search failed: {response.error}")

        return [
            SearchResult(
                id=r.id,
                distance=r.distance,
                vector=list(r.vector) if r.vector else None,
                metadata=dict(r.metadata) if r.metadata else None,
                text=r.text if r.text else None
            )
            for r in response.results
        ]

    def hybrid_search(self, namespace: str, query_vector: List[float],
                     query_text: str, k: int = 10,
                     ef_search: int = 50,
                     fusion_method: str = "rrf",
                     vector_weight: float = 0.7,
                     text_weight: float = 0.3) -> List[SearchResult]:
        """
        Hybrid search combining vector similarity and full-text search

        Args:
            namespace: Namespace to search in
            query_vector: Query vector embedding
            query_text: Query text for full-text search
            k: Number of results to return
            ef_search: HNSW ef_search parameter
            fusion_method: "rrf" or "weighted"
            vector_weight: Weight for vector results (0.0-1.0)
            text_weight: Weight for text results (0.0-1.0)

        Returns:
            List of SearchResult objects

        Example:
            >>> results = client.hybrid_search(
            ...     namespace="default",
            ...     query_vector=[0.1, 0.2, 0.3, 0.4],
            ...     query_text="machine learning vector database",
            ...     k=20
            ... )
        """
        config = vector_pb2.HybridSearchConfig(
            fusion_method=fusion_method,
            vector_weight=vector_weight,
            text_weight=text_weight,
            rrf_k=60
        )

        request = vector_pb2.HybridSearchRequest(
            namespace=namespace,
            query_vector=query_vector,
            query_text=query_text,
            k=k,
            ef_search=ef_search,
            config=config
        )

        response = self.stub.HybridSearch(request)

        if response.error:
            raise Exception(f"Hybrid search failed: {response.error}")

        return [
            SearchResult(
                id=r.id,
                distance=r.distance,
                vector=list(r.vector) if r.vector else None,
                metadata=dict(r.metadata) if r.metadata else None,
                text=r.text if r.text else None,
                vector_score=r.vector_score if hasattr(r, 'vector_score') else None,
                text_score=r.text_score if hasattr(r, 'text_score') else None
            )
            for r in response.results
        ]

    def batch_insert(self, namespace: str, vectors: List[Tuple[List[float], Dict[str, str]]]) -> Dict:
        """
        Insert multiple vectors efficiently

        Args:
            namespace: Namespace for vectors
            vectors: List of (vector, metadata) tuples

        Returns:
            Dictionary with inserted_count, failed_count, and inserted_ids

        Example:
            >>> vectors = [
            ...     ([0.1, 0.2, 0.3], {"title": "Doc 1"}),
            ...     ([0.4, 0.5, 0.6], {"title": "Doc 2"}),
            ... ]
            >>> result = client.batch_insert("default", vectors)
            >>> print(f"Inserted {result['inserted_count']} vectors")
        """
        def request_generator():
            for vector, metadata in vectors:
                yield vector_pb2.InsertRequest(
                    namespace=namespace,
                    vector=vector,
                    metadata=metadata
                )

        response = self.stub.BatchInsert(request_generator())

        return {
            "inserted_count": response.inserted_count,
            "failed_count": response.failed_count,
            "inserted_ids": list(response.inserted_ids),
            "errors": list(response.errors),
            "total_time_ms": response.total_time_ms
        }

    def update(self, namespace: str, id: str,
               vector: Optional[List[float]] = None,
               metadata: Optional[Dict[str, str]] = None,
               text: Optional[str] = None) -> bool:
        """
        Update an existing vector

        Args:
            namespace: Namespace
            id: Vector ID to update
            vector: New vector (None to keep existing)
            metadata: New metadata (None to keep existing)
            text: New text content (None to keep existing)

        Returns:
            True if successful
        """
        request = vector_pb2.UpdateRequest(
            namespace=namespace,
            id=id,
            vector=vector or [],
            metadata=metadata or {},
            text=text
        )

        response = self.stub.Update(request)

        if not response.success:
            raise Exception(f"Update failed: {response.error}")

        return True

    def delete(self, namespace: str, id: str) -> int:
        """
        Delete a vector by ID

        Args:
            namespace: Namespace
            id: Vector ID to delete

        Returns:
            Number of vectors deleted
        """
        request = vector_pb2.DeleteRequest(
            namespace=namespace,
            id=id
        )

        response = self.stub.Delete(request)

        if not response.success:
            raise Exception(f"Delete failed: {response.error}")

        return response.deleted_count

    def get_stats(self, namespace: Optional[str] = None) -> Dict:
        """
        Get database statistics

        Args:
            namespace: Optional namespace (all if not specified)

        Returns:
            Dictionary with statistics
        """
        request = vector_pb2.StatsRequest(namespace=namespace)
        response = self.stub.GetStats(request)

        return {
            "total_vectors": response.total_vectors,
            "total_namespaces": response.total_namespaces,
            "memory_usage_bytes": response.memory_usage_bytes,
            "namespace_stats": {
                ns: {
                    "vector_count": stats.vector_count,
                    "memory_bytes": stats.memory_bytes,
                    "dimensions": stats.dimensions
                }
                for ns, stats in response.namespace_stats.items()
            }
        }

    def health_check(self) -> Dict:
        """
        Check server health

        Returns:
            Dictionary with health status
        """
        request = vector_pb2.HealthCheckRequest()
        response = self.stub.HealthCheck(request)

        return {
            "status": response.status,
            "version": response.version,
            "uptime_seconds": response.uptime_seconds,
            "details": dict(response.details)
        }
