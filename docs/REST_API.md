# REST API Documentation

## Overview

The Vector Database now includes a comprehensive HTTP/JSON REST API wrapper around the gRPC service. This provides an easier way to interact with the database using standard HTTP methods and JSON payloads.

## Features

### 1. HTTP/JSON Wrapper
- RESTful API design with standard HTTP methods
- JSON request/response format
- Automatic conversion between REST and gRPC
- Full support for all vector operations

### 2. Authentication & Authorization
- JWT-based authentication
- Configurable public paths (no auth required)
- Admin role support for privileged operations
- Bearer token authentication

### 3. Rate Limiting
- Token bucket algorithm
- Per-IP rate limiting
- Per-user rate limiting (requires authentication)
- Global rate limiting option
- Configurable requests per second and burst size

### 4. OpenAPI/Swagger Documentation
- Interactive API documentation at `/docs`
- OpenAPI 3.0 specification at `/docs/openapi.yaml`
- Auto-generated from service definitions

### 5. Additional Features
- CORS support with configurable origins
- Request logging middleware
- Graceful shutdown support
- Health check endpoint

## Quick Start

### 1. Start the Server

```bash
# Start with default configuration (REST API enabled on port 8080)
./bin/vector-server

# Start with custom REST port
VECTOR_REST_PORT=9000 ./bin/vector-server

# Disable REST API
VECTOR_REST_ENABLED=false ./bin/vector-server
```

### 2. Access API Documentation

Open your browser and navigate to:
```
http://localhost:8080/docs
```

This will display the interactive Swagger UI with all available endpoints.

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `VECTOR_REST_ENABLED` | `true` | Enable/disable REST API |
| `VECTOR_REST_HOST` | `0.0.0.0` | REST server host |
| `VECTOR_REST_PORT` | `8080` | REST server port |
| `VECTOR_CORS_ENABLED` | `true` | Enable CORS |
| `VECTOR_AUTH_ENABLED` | `false` | Enable JWT authentication |
| `VECTOR_JWT_SECRET` | `change-this-secret-in-production` | JWT signing secret |
| `VECTOR_RATE_LIMIT_ENABLED` | `true` | Enable rate limiting |
| `VECTOR_RATE_LIMIT_PER_SEC` | `10.0` | Requests per second |
| `VECTOR_RATE_LIMIT_BURST` | `20` | Burst size |

### Example with Authentication Enabled

```bash
export VECTOR_AUTH_ENABLED=true
export VECTOR_JWT_SECRET="your-super-secret-key"
export VECTOR_RATE_LIMIT_PER_SEC=100
export VECTOR_RATE_LIMIT_BURST=200

./bin/vector-server
```

## API Endpoints

### Health & Stats

#### Health Check
```bash
GET /v1/health
```

Example:
```bash
curl http://localhost:8080/v1/health
```

Response:
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime_seconds": 3600,
  "details": {}
}
```

#### Get Statistics
```bash
GET /v1/stats
GET /v1/stats/{namespace}
```

Example:
```bash
curl http://localhost:8080/v1/stats
curl http://localhost:8080/v1/stats/my-namespace
```

### Vector Operations

#### Insert Vector
```bash
POST /v1/vectors
Content-Type: application/json

{
  "namespace": "my-namespace",
  "vector": [0.1, 0.2, 0.3, ...],
  "metadata": {
    "title": "Document title",
    "author": "John Doe"
  },
  "text": "Optional text for full-text search",
  "id": "optional-custom-id"
}
```

Example:
```bash
curl -X POST http://localhost:8080/v1/vectors \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "documents",
    "vector": [0.1, 0.2, 0.3, 0.4, 0.5],
    "metadata": {
      "title": "My Document",
      "category": "tech"
    },
    "text": "This is a sample document"
  }'
```

Response:
```json
{
  "id": "generated-uuid",
  "success": true
}
```

#### Search Vectors
```bash
POST /v1/vectors/search
Content-Type: application/json

{
  "namespace": "my-namespace",
  "query_vector": [0.1, 0.2, 0.3, ...],
  "k": 10,
  "ef_search": 50,
  "distance_metric": "cosine",
  "filter": {
    "comparison": {
      "field": "category",
      "operator": "eq",
      "value": "tech"
    }
  }
}
```

Example:
```bash
curl -X POST http://localhost:8080/v1/vectors/search \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "documents",
    "query_vector": [0.1, 0.2, 0.3, 0.4, 0.5],
    "k": 5,
    "ef_search": 50
  }'
```

#### Hybrid Search (Vector + Full-Text)
```bash
POST /v1/vectors/hybrid-search
Content-Type: application/json

{
  "namespace": "my-namespace",
  "query_vector": [0.1, 0.2, 0.3, ...],
  "query_text": "search terms",
  "k": 10,
  "ef_search": 50,
  "config": {
    "fusion_method": "rrf",
    "rrf_k": 60
  }
}
```

Example:
```bash
curl -X POST http://localhost:8080/v1/vectors/hybrid-search \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "documents",
    "query_vector": [0.1, 0.2, 0.3, 0.4, 0.5],
    "query_text": "sample document",
    "k": 5,
    "ef_search": 50
  }'
```

#### Update Vector
```bash
PUT /v1/vectors/{namespace}/{id}
PATCH /v1/vectors/{namespace}/{id}
Content-Type: application/json

{
  "vector": [0.1, 0.2, 0.3, ...],
  "metadata": {
    "updated": "true"
  },
  "text": "Updated text"
}
```

Example:
```bash
curl -X PUT http://localhost:8080/v1/vectors/documents/abc-123 \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {
      "title": "Updated Title",
      "updated": "true"
    }
  }'
```

#### Delete Vector
```bash
DELETE /v1/vectors/{namespace}/{id}
```

Or delete by filter:
```bash
POST /v1/vectors/delete
Content-Type: application/json

{
  "namespace": "my-namespace",
  "filter": {
    "comparison": {
      "field": "category",
      "operator": "eq",
      "value": "outdated"
    }
  }
}
```

Example:
```bash
# Delete by ID
curl -X DELETE http://localhost:8080/v1/vectors/documents/abc-123

# Delete by filter
curl -X POST http://localhost:8080/v1/vectors/delete \
  -H "Content-Type: application/json" \
  -d '{
    "namespace": "documents",
    "filter": {
      "comparison": {
        "field": "category",
        "operator": "eq",
        "value": "temp"
      }
    }
  }'
```

#### Batch Insert
```bash
POST /v1/vectors/batch
Content-Type: application/json

[
  {
    "namespace": "my-namespace",
    "vector": [0.1, 0.2, 0.3, ...],
    "metadata": {"key": "value"}
  },
  {
    "namespace": "my-namespace",
    "vector": [0.4, 0.5, 0.6, ...],
    "metadata": {"key": "value2"}
  }
]
```

Example:
```bash
curl -X POST http://localhost:8080/v1/vectors/batch \
  -H "Content-Type: application/json" \
  -d '[
    {
      "namespace": "documents",
      "vector": [0.1, 0.2, 0.3, 0.4, 0.5],
      "metadata": {"title": "Doc 1"}
    },
    {
      "namespace": "documents",
      "vector": [0.6, 0.7, 0.8, 0.9, 1.0],
      "metadata": {"title": "Doc 2"}
    }
  ]'
```

## Authentication

When authentication is enabled, include a JWT token in the Authorization header:

```bash
curl -X POST http://localhost:8080/v1/vectors/search \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{...}'
```

### Generating Tokens (Development)

For development/testing, you can use the included JWT generation utility or any JWT library:

```go
package main

import (
    "fmt"
    "github.com/therealutkarshpriyadarshi/vector/pkg/api/rest/middleware"
)

func main() {
    token, _ := middleware.GenerateToken(
        "user123",           // User ID
        "john.doe",          // Username
        []string{"user"},    // Roles
        "my-namespace",      // Namespace
        "your-secret-key",   // Secret
    )
    fmt.Println("Token:", token)
}
```

## Rate Limiting

Rate limit headers are included in all responses:

```
X-RateLimit-Limit: 20
X-RateLimit-Remaining: 15
```

When rate limit is exceeded:
```json
{
  "error": "Rate limit exceeded for 192.168.1.1",
  "status": 429
}
```

And the response includes:
```
Retry-After: 60
```

## Filters

The API supports various filter types for metadata filtering:

### Comparison Filter
```json
{
  "comparison": {
    "field": "category",
    "operator": "eq",
    "value": "tech"
  }
}
```

Operators: `eq`, `ne`, `gt`, `lt`, `gte`, `lte`

### Range Filter
```json
{
  "range": {
    "field": "price",
    "gte": "100",
    "lte": "500"
  }
}
```

### List Filter
```json
{
  "list": {
    "field": "tags",
    "operator": "in",
    "values": ["ai", "ml", "nlp"]
  }
}
```

### Geographic Filter
```json
{
  "geo_radius": {
    "field": "location",
    "latitude": 40.7128,
    "longitude": -74.0060,
    "radius_km": 10.0
  }
}
```

### Composite Filter
```json
{
  "composite": {
    "operator": "and",
    "filters": [
      {
        "comparison": {
          "field": "category",
          "operator": "eq",
          "value": "tech"
        }
      },
      {
        "range": {
          "field": "year",
          "gte": "2020"
        }
      }
    ]
  }
}
```

## Error Handling

All errors return JSON with the following format:

```json
{
  "error": "Error message",
  "status": 400
}
```

Common HTTP status codes:
- `200` - Success
- `201` - Created
- `400` - Bad Request
- `401` - Unauthorized
- `403` - Forbidden
- `404` - Not Found
- `429` - Too Many Requests
- `500` - Internal Server Error

## Production Considerations

### Security

1. **Always set a strong JWT secret:**
   ```bash
   export VECTOR_JWT_SECRET="$(openssl rand -base64 32)"
   ```

2. **Enable authentication in production:**
   ```bash
   export VECTOR_AUTH_ENABLED=true
   ```

3. **Configure CORS properly:**
   ```bash
   export VECTOR_CORS_ENABLED=true
   # Don't use "*" in production - specify allowed origins
   ```

4. **Use TLS/HTTPS:**
   - Put the service behind a reverse proxy (nginx, Caddy)
   - Or configure TLS for the gRPC server

### Rate Limiting

Adjust rate limits based on your workload:

```bash
# Higher limits for production
export VECTOR_RATE_LIMIT_PER_SEC=100
export VECTOR_RATE_LIMIT_BURST=200

# Per-user rate limiting (requires auth)
export VECTOR_AUTH_ENABLED=true
export VECTOR_RATE_LIMIT_PER_USER=true
```

### Monitoring

Monitor these metrics:
- Request rate and latency
- Error rates
- Rate limit rejections
- Memory and CPU usage
- gRPC connection health

## Examples

See the `examples/rest-api/` directory for complete examples in multiple programming languages:

- Python
- JavaScript/Node.js
- Go
- cURL scripts

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ HTTP/JSON
       v
┌─────────────────────────────────────┐
│        REST API Server              │
│  ┌─────────────────────────────┐   │
│  │  Middleware Chain           │   │
│  │  1. Logging                 │   │
│  │  2. CORS                    │   │
│  │  3. Rate Limiting           │   │
│  │  4. Authentication          │   │
│  └─────────────────────────────┘   │
│  ┌─────────────────────────────┐   │
│  │  HTTP Handlers              │   │
│  │  (JSON ← → gRPC conversion) │   │
│  └─────────────────────────────┘   │
└─────────────┬───────────────────────┘
              │ gRPC
              v
┌─────────────────────────────────────┐
│        gRPC Server                  │
│  ┌─────────────────────────────┐   │
│  │  Vector Operations          │   │
│  │  - Insert, Search, Update   │   │
│  │  - Delete, Batch, Stats     │   │
│  └─────────────────────────────┘   │
└─────────────┬───────────────────────┘
              │
              v
┌─────────────────────────────────────┐
│        HNSW Index + Storage         │
└─────────────────────────────────────┘
```

## Comparison: REST vs gRPC

| Feature | REST API | gRPC |
|---------|----------|------|
| Protocol | HTTP/1.1 | HTTP/2 |
| Format | JSON | Protocol Buffers |
| Ease of Use | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| Performance | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Browser Support | ✅ Native | ❌ Requires grpc-web |
| Streaming | Limited | ✅ Full Support |
| Type Safety | Runtime | Compile-time |
| Tools | curl, Postman, etc. | grpcurl, specialized clients |

**Recommendation:**
- Use **REST API** for web applications, prototyping, and ease of integration
- Use **gRPC** for high-performance, service-to-service communication

## Support

For issues, questions, or contributions:
- GitHub Issues: https://github.com/therealutkarshpriyadarshi/vector/issues
- Documentation: https://github.com/therealutkarshpriyadarshi/vector/docs
