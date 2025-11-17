"""
Vector Database Python Client

A Python client library for the Vector Database with HNSW and NSG indexing.
"""

from .client import VectorDBClient, SearchResult

__version__ = "1.1.0"
__all__ = ["VectorDBClient", "SearchResult"]
