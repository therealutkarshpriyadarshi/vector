# Production Deployment Guide

Complete guide for deploying the Vector Database in production environments.

## Table of Contents

- [Overview](#overview)
- [System Requirements](#system-requirements)
- [Deployment Options](#deployment-options)
  - [Docker](#docker)
  - [Kubernetes](#kubernetes)
  - [Systemd](#systemd)
  - [Bare Metal](#bare-metal)
- [Configuration](#configuration)
- [Security](#security)
- [Monitoring](#monitoring)
- [Backup & Recovery](#backup--recovery)
- [Scaling](#scaling)
- [Troubleshooting](#troubleshooting)

---

## Overview

The Vector Database is designed for production use with:
- **High Performance**: <10ms p95 latency for 1M vectors
- **Reliability**: Write-Ahead Log (WAL) for crash recovery
- **Scalability**: Multi-tenancy with namespace isolation
- **Observability**: Prometheus metrics + structured logging

**Architecture**:
```
Client → gRPC (TLS) → Vector DB Server → BadgerDB (persistence)
                           ↓
                    Prometheus/Grafana (monitoring)
```

---

## System Requirements

### Minimum Requirements

**For 100K vectors (768 dimensions)**:
- CPU: 2 cores
- RAM: 2 GB
- Disk: 5 GB SSD
- Network: 100 Mbps

### Recommended Requirements

**For 1M vectors (768 dimensions)**:
- CPU: 4-8 cores
- RAM: 8-16 GB
- Disk: 50 GB NVMe SSD
- Network: 1 Gbps

### Large Scale Requirements

**For 10M+ vectors**:
- CPU: 16+ cores
- RAM: 64+ GB
- Disk: 500 GB NVMe SSD (RAID 10)
- Network: 10 Gbps

### Memory Sizing

**Formula**: `Memory (GB) = (Vectors × Dimensions × 4 bytes × 1.5) / 1024³`

**Examples**:
- 100K vectors × 768 dims: ~460 MB
- 1M vectors × 768 dims: ~4.6 GB
- 10M vectors × 768 dims: ~46 GB

**Overhead**: 1.5x multiplier accounts for:
- HNSW graph structure
- Metadata storage
- Operating system cache
- Connection buffers

**With Quantization** (4x reduction):
- 1M vectors × 768 dims: ~1.2 GB
- 10M vectors × 768 dims: ~12 GB

### Disk Sizing

**Formula**: `Disk (GB) = Vectors × (Dimensions × 4 + Metadata Size) × 2 / 1024³`

**Examples**:
- 1M vectors: ~10 GB (with WAL)
- 10M vectors: ~100 GB
- 100M vectors: ~1 TB

**Best Practices**:
- Use SSD for low latency (<1ms seek time)
- NVMe preferred for >10M vectors
- RAID 10 for high availability
- Keep 30% free space for compaction

---

## Deployment Options

### Docker

#### Quick Start

```bash
# Pull image
docker pull ghcr.io/therealutkarshpriyadarshi/vector:latest

# Run server
docker run -d \
  --name vector-db \
  -p 50051:50051 \
  -v /data/vector:/data \
  -e VECTOR_DATA_DIR=/data \
  -e VECTOR_HNSW_M=16 \
  -e VECTOR_HNSW_EF_CONSTRUCTION=200 \
  ghcr.io/therealutkarshpriyadarshi/vector:latest
```

#### Docker Compose

```yaml
# docker-compose.yml
version: '3.8'

services:
  vector-db:
    image: ghcr.io/therealutkarshpriyadarshi/vector:latest
    container_name: vector-db
    ports:
      - "50051:50051"
    volumes:
      - vector-data:/data
      - ./config.yaml:/etc/vector/config.yaml
    environment:
      - VECTOR_DATA_DIR=/data
      - VECTOR_HNSW_M=16
      - VECTOR_HNSW_EF_CONSTRUCTION=200
      - VECTOR_CACHE_ENABLED=true
      - VECTOR_CACHE_CAPACITY=10000
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "/app/cli", "health"]
      interval: 30s
      timeout: 10s
      retries: 3
    deploy:
      resources:
        limits:
          cpus: '4'
          memory: 8G
        reservations:
          cpus: '2'
          memory: 4G

  prometheus:
    image: prom/prometheus:latest
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus-data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    ports:
      - "3000:3000"
    volumes:
      - grafana-data:/var/lib/grafana
      - ./grafana/dashboards:/etc/grafana/provisioning/dashboards
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
      - GF_USERS_ALLOW_SIGN_UP=false
    restart: unless-stopped

volumes:
  vector-data:
  prometheus-data:
  grafana-data:
```

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f vector-db

# Stop services
docker-compose down
```

#### Build Custom Image

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /build/server .
COPY --from=builder /build/config.yaml .

EXPOSE 50051
CMD ["./server"]
```

```bash
# Build
docker build -t vector-db:custom .

# Run
docker run -d -p 50051:50051 vector-db:custom
```

---

### Kubernetes

#### Deployment Manifest

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vector-db
  namespace: vector
  labels:
    app: vector-db
spec:
  replicas: 3
  selector:
    matchLabels:
      app: vector-db
  template:
    metadata:
      labels:
        app: vector-db
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      containers:
      - name: vector-db
        image: ghcr.io/therealutkarshpriyadarshi/vector:latest
        imagePullPolicy: Always
        ports:
        - containerPort: 50051
          name: grpc
          protocol: TCP
        - containerPort: 9090
          name: metrics
          protocol: TCP
        env:
        - name: VECTOR_DATA_DIR
          value: "/data"
        - name: VECTOR_HNSW_M
          valueFrom:
            configMapKeyRef:
              name: vector-config
              key: hnsw_m
        - name: VECTOR_HNSW_EF_CONSTRUCTION
          valueFrom:
            configMapKeyRef:
              name: vector-config
              key: hnsw_ef_construction
        - name: VECTOR_CACHE_ENABLED
          value: "true"
        - name: VECTOR_CACHE_CAPACITY
          value: "10000"
        volumeMounts:
        - name: data
          mountPath: /data
        resources:
          requests:
            memory: "4Gi"
            cpu: "2"
          limits:
            memory: "8Gi"
            cpu: "4"
        livenessProbe:
          exec:
            command:
            - /app/cli
            - health
          initialDelaySeconds: 30
          periodSeconds: 30
          timeoutSeconds: 10
          failureThreshold: 3
        readinessProbe:
          exec:
            command:
            - /app/cli
            - health
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: vector-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: vector-db
  namespace: vector
spec:
  type: LoadBalancer
  selector:
    app: vector-db
  ports:
  - name: grpc
    port: 50051
    targetPort: 50051
    protocol: TCP
  - name: metrics
    port: 9090
    targetPort: 9090
    protocol: TCP
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: vector-pvc
  namespace: vector
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 100Gi
  storageClassName: fast-ssd
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: vector-config
  namespace: vector
data:
  hnsw_m: "16"
  hnsw_ef_construction: "200"
  dimensions: "768"
```

#### Deploy to Kubernetes

```bash
# Create namespace
kubectl create namespace vector

# Apply manifests
kubectl apply -f deployment.yaml

# Check status
kubectl get pods -n vector
kubectl get svc -n vector

# View logs
kubectl logs -f deployment/vector-db -n vector

# Scale deployment
kubectl scale deployment/vector-db --replicas=5 -n vector

# Update configuration
kubectl edit configmap vector-config -n vector
kubectl rollout restart deployment/vector-db -n vector
```

#### Horizontal Pod Autoscaler

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: vector-db-hpa
  namespace: vector
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: vector-db
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

```bash
kubectl apply -f hpa.yaml
```

---

### Systemd

#### Service File

```ini
# /etc/systemd/system/vector-db.service
[Unit]
Description=Vector Database Server
After=network.target
Documentation=https://github.com/therealutkarshpriyadarshi/vector

[Service]
Type=simple
User=vector
Group=vector
WorkingDirectory=/opt/vector
ExecStart=/opt/vector/bin/server
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s
LimitNOFILE=65536

# Security
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/vector/data

# Environment
Environment="VECTOR_DATA_DIR=/opt/vector/data"
Environment="VECTOR_PORT=50051"
Environment="VECTOR_HNSW_M=16"
Environment="VECTOR_HNSW_EF_CONSTRUCTION=200"

[Install]
WantedBy=multi-user.target
```

#### Setup

```bash
# Create user
sudo useradd -r -s /bin/false vector

# Create directories
sudo mkdir -p /opt/vector/{bin,data,config}
sudo chown -R vector:vector /opt/vector

# Copy binary
sudo cp bin/server /opt/vector/bin/
sudo chmod +x /opt/vector/bin/server

# Enable service
sudo systemctl daemon-reload
sudo systemctl enable vector-db
sudo systemctl start vector-db

# Check status
sudo systemctl status vector-db
sudo journalctl -u vector-db -f
```

---

### Bare Metal

#### Installation

```bash
# Download binary
wget https://github.com/therealutkarshpriyadarshi/vector/releases/latest/download/vector-linux-amd64.tar.gz
tar -xzf vector-linux-amd64.tar.gz
cd vector

# Install
sudo cp bin/server /usr/local/bin/vector-server
sudo cp bin/cli /usr/local/bin/vector-cli
sudo mkdir -p /etc/vector /var/lib/vector
sudo cp config.yaml /etc/vector/

# Create user
sudo useradd -r -s /bin/false vector
sudo chown -R vector:vector /var/lib/vector
```

#### Configuration

```yaml
# /etc/vector/config.yaml
server:
  host: "0.0.0.0"
  port: 50051
  max_connections: 1000
  request_timeout: 30s
  shutdown_timeout: 10s
  enable_tls: false

hnsw:
  m: 16
  ef_construction: 200
  default_ef_search: 50
  dimensions: 768

cache:
  enabled: true
  capacity: 10000
  ttl: 5m

database:
  data_dir: "/var/lib/vector"
  enable_wal: true
  sync_writes: false
  max_namespaces: 100
```

#### Run

```bash
# Start server
sudo -u vector vector-server --config /etc/vector/config.yaml

# Or with environment variables
VECTOR_DATA_DIR=/var/lib/vector \
VECTOR_PORT=50051 \
VECTOR_HNSW_M=16 \
vector-server
```

---

## Configuration

### Environment Variables

All configuration can be set via environment variables:

**Server**:
- `VECTOR_HOST`: Server host (default: "0.0.0.0")
- `VECTOR_PORT`: Server port (default: 50051)
- `VECTOR_MAX_CONNECTIONS`: Max concurrent connections (default: 1000)
- `VECTOR_REQUEST_TIMEOUT`: Request timeout (default: "30s")
- `VECTOR_ENABLE_TLS`: Enable TLS (default: false)
- `VECTOR_TLS_CERT`: TLS certificate file path
- `VECTOR_TLS_KEY`: TLS key file path

**HNSW**:
- `VECTOR_HNSW_M`: Connections per layer (default: 16)
- `VECTOR_HNSW_EF_CONSTRUCTION`: Construction accuracy (default: 200)
- `VECTOR_DIMENSIONS`: Vector dimensions (default: 768)

**Cache**:
- `VECTOR_CACHE_ENABLED`: Enable query cache (default: true)
- `VECTOR_CACHE_CAPACITY`: Max cache entries (default: 1000)
- `VECTOR_CACHE_TTL`: Cache TTL (default: "5m")

**Database**:
- `VECTOR_DATA_DIR`: Data directory (default: "./data")
- `VECTOR_ENABLE_WAL`: Enable WAL (default: true)
- `VECTOR_SYNC_WRITES`: Sync writes to disk (default: false)

### Configuration File

```yaml
# config.yaml
server:
  host: "0.0.0.0"
  port: 50051
  max_connections: 1000
  request_timeout: 30s
  shutdown_timeout: 10s
  enable_tls: true
  cert_file: "/etc/vector/certs/server.crt"
  key_file: "/etc/vector/certs/server.key"

hnsw:
  m: 16                    # Higher = better recall, more memory
  ef_construction: 200     # Higher = better index quality
  default_ef_search: 50    # Higher = better recall, slower
  dimensions: 768

cache:
  enabled: true
  capacity: 10000          # Number of queries to cache
  ttl: 5m                  # Cache entry lifetime

database:
  data_dir: "/var/lib/vector"
  enable_wal: true         # Write-ahead log for durability
  sync_writes: false       # Sync every write (slower but safer)
  max_namespaces: 100
```

### Tuning Guide

**For High Throughput**:
```yaml
hnsw:
  m: 12                    # Lower M
  ef_construction: 100     # Lower construction
  default_ef_search: 30    # Lower search

cache:
  enabled: true
  capacity: 50000          # Large cache

database:
  sync_writes: false       # Async writes
```

**For High Accuracy**:
```yaml
hnsw:
  m: 32                    # Higher M
  ef_construction: 400     # Higher construction
  default_ef_search: 150   # Higher search

cache:
  enabled: true
  capacity: 10000

database:
  sync_writes: false
```

**For Durability**:
```yaml
database:
  enable_wal: true
  sync_writes: true        # Sync every write
```

---

## Security

### TLS Configuration

#### Generate Certificates

```bash
# Generate CA
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 365 -key ca.key -out ca.crt

# Generate server certificate
openssl genrsa -out server.key 4096
openssl req -new -key server.key -out server.csr
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -set_serial 01 -out server.crt

# Copy to server
sudo mkdir -p /etc/vector/certs
sudo cp server.{crt,key} /etc/vector/certs/
sudo chmod 600 /etc/vector/certs/server.key
```

#### Enable TLS

```bash
# Via environment
export VECTOR_ENABLE_TLS=true
export VECTOR_TLS_CERT=/etc/vector/certs/server.crt
export VECTOR_TLS_KEY=/etc/vector/certs/server.key

# Via config.yaml
server:
  enable_tls: true
  cert_file: "/etc/vector/certs/server.crt"
  key_file: "/etc/vector/certs/server.key"
```

### Network Security

#### Firewall Rules

```bash
# Allow gRPC port
sudo ufw allow 50051/tcp

# Allow metrics port (internal only)
sudo ufw allow from 10.0.0.0/8 to any port 9090

# Enable firewall
sudo ufw enable
```

#### Network Policies (Kubernetes)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: vector-db-policy
  namespace: vector
spec:
  podSelector:
    matchLabels:
      app: vector-db
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: application
    ports:
    - protocol: TCP
      port: 50051
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: monitoring
    ports:
    - protocol: TCP
      port: 9090
```

### Best Practices

1. **Always use TLS in production**
2. **Restrict network access** to trusted sources
3. **Use namespace isolation** for multi-tenancy
4. **Enable audit logging** for compliance
5. **Rotate credentials** regularly
6. **Monitor for anomalies** (unusual query patterns)
7. **Implement rate limiting** at load balancer level

---

## Monitoring

### Prometheus Metrics

The server exposes metrics at `http://localhost:9090/metrics`:

**Server Metrics**:
- `vector_server_uptime_seconds`: Server uptime
- `vector_server_requests_total`: Total requests (by method)
- `vector_server_request_duration_seconds`: Request latency histogram
- `vector_server_errors_total`: Total errors (by type)
- `vector_server_connections_active`: Active connections

**Index Metrics**:
- `vector_index_size`: Number of vectors (by namespace)
- `vector_index_dimensions`: Vector dimensions (by namespace)
- `vector_index_memory_bytes`: Memory usage (by namespace)

**Cache Metrics**:
- `vector_cache_hits_total`: Cache hits
- `vector_cache_misses_total`: Cache misses
- `vector_cache_hit_rate`: Cache hit rate
- `vector_cache_size`: Current cache size

**Search Metrics**:
- `vector_search_latency_seconds`: Search latency (p50, p95, p99)
- `vector_search_recall`: Search recall quality
- `vector_insert_latency_seconds`: Insert latency

### Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'vector-db'
    static_configs:
      - targets: ['localhost:9090']
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'vector_.*'
        action: keep
```

### Grafana Dashboard

See `grafana/dashboards/vector-db.json` for a pre-built dashboard with:
- Request rate and latency
- Error rates
- Cache hit rates
- Memory usage
- Index size and growth
- Search recall over time

### Health Checks

```bash
# CLI health check
vector-cli health

# gRPC health check
grpcurl -plaintext localhost:50051 vector.VectorDB/HealthCheck

# HTTP health endpoint (if enabled)
curl http://localhost:8080/health
```

---

## Backup & Recovery

### Backup Strategy

#### Full Backup

```bash
# Stop server
sudo systemctl stop vector-db

# Backup data directory
tar -czf vector-backup-$(date +%Y%m%d).tar.gz /var/lib/vector/

# Restart server
sudo systemctl start vector-db

# Upload to S3
aws s3 cp vector-backup-*.tar.gz s3://backups/vector/
```

#### Hot Backup (No Downtime)

```bash
# BadgerDB supports hot backups
vector-cli backup --output /backups/vector-$(date +%Y%m%d)

# Verify backup
vector-cli verify-backup --path /backups/vector-$(date +%Y%m%d)
```

#### Automated Backups (Cron)

```bash
# /etc/cron.d/vector-backup
0 2 * * * vector /usr/local/bin/vector-backup.sh
```

```bash
#!/bin/bash
# /usr/local/bin/vector-backup.sh

BACKUP_DIR="/backups/vector"
RETENTION_DAYS=7

# Create backup
vector-cli backup --output "$BACKUP_DIR/vector-$(date +%Y%m%d-%H%M%S)"

# Delete old backups
find "$BACKUP_DIR" -name "vector-*" -mtime +$RETENTION_DAYS -delete

# Upload to S3
aws s3 sync "$BACKUP_DIR" s3://backups/vector/
```

### Restore

```bash
# Stop server
sudo systemctl stop vector-db

# Restore data
tar -xzf vector-backup-20250115.tar.gz -C /

# Or restore from hot backup
vector-cli restore --input /backups/vector-20250115 --output /var/lib/vector

# Start server
sudo systemctl start vector-db

# Verify data
vector-cli stats
```

### Disaster Recovery

**RTO (Recovery Time Objective)**: < 5 minutes
**RPO (Recovery Point Objective)**: < 1 hour

**DR Checklist**:
1. Automated backups every hour
2. Off-site backup storage (S3, GCS)
3. Tested restore procedure (monthly)
4. Multi-region replication (future)
5. Documented runbook

---

## Scaling

### Vertical Scaling

**When to scale up**:
- Memory usage > 80%
- CPU usage > 70%
- Search latency increasing
- Cache hit rate decreasing

**Upgrade path**:
- 1M vectors: 8 GB RAM, 4 CPU
- 5M vectors: 32 GB RAM, 8 CPU
- 10M vectors: 64 GB RAM, 16 CPU
- 50M vectors: 256 GB RAM, 32 CPU

### Horizontal Scaling

**Read Replicas** (future):
```yaml
# Multiple read replicas for search queries
┌─────────┐
│ Primary │ (writes)
└────┬────┘
     │
     ├─── Replica 1 (reads)
     ├─── Replica 2 (reads)
     └─── Replica 3 (reads)
```

**Sharding by Namespace** (current):
```yaml
# Distribute namespaces across multiple servers
Server 1: namespace_a, namespace_b
Server 2: namespace_c, namespace_d
Server 3: namespace_e, namespace_f
```

**Load Balancing**:
```yaml
# nginx.conf
upstream vector_backends {
    least_conn;
    server vector1:50051 max_fails=3 fail_timeout=30s;
    server vector2:50051 max_fails=3 fail_timeout=30s;
    server vector3:50051 max_fails=3 fail_timeout=30s;
}

server {
    listen 50051 http2;
    location / {
        grpc_pass grpc://vector_backends;
    }
}
```

---

## Troubleshooting

See [Troubleshooting Guide](troubleshooting.md) for detailed solutions.

**Common Issues**:
1. High memory usage → Enable quantization
2. Slow searches → Tune ef_search, add cache
3. Data corruption → Restore from backup, enable WAL
4. Connection timeouts → Increase max_connections
5. Low recall → Increase ef_construction, M

---

## Production Checklist

Before going live:

- [ ] TLS enabled and tested
- [ ] Monitoring configured (Prometheus + Grafana)
- [ ] Automated backups scheduled
- [ ] Restore procedure tested
- [ ] Health checks configured
- [ ] Resource limits set
- [ ] Firewall rules applied
- [ ] Load testing completed
- [ ] Disaster recovery plan documented
- [ ] On-call runbook created
- [ ] Performance baselines established
- [ ] Logging aggregation configured

---

## Next Steps

- [API Reference](api.md) - Complete API documentation
- [Algorithms](algorithms.md) - HNSW and NSG deep dive
- [Benchmarks](benchmarks.md) - Performance testing guide
- [Troubleshooting](troubleshooting.md) - Common issues and solutions

---

**Version**: 1.1.0
**Last Updated**: 2025-01-15
