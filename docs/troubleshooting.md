# Troubleshooting Guide

Common issues and solutions for the Vector Database.

## Table of Contents

- [Quick Diagnosis](#quick-diagnosis)
- [Performance Issues](#performance-issues)
- [Memory Issues](#memory-issues)
- [Search Quality Issues](#search-quality-issues)
- [Connection Issues](#connection-issues)
- [Data Integrity Issues](#data-integrity-issues)
- [Deployment Issues](#deployment-issues)
- [Monitoring and Debugging](#monitoring-and-debugging)

---

## Quick Diagnosis

### Health Check

```bash
# Check server health
vector-cli health

# Or via gRPC
grpcurl -plaintext localhost:50051 vector.VectorDB/HealthCheck
```

**Expected Response**:
```json
{
  "status": "healthy",
  "version": "1.1.0",
  "uptime_seconds": 3600,
  "details": {
    "namespaces": "3",
    "total_vectors": "1000000"
  }
}
```

### Get Statistics

```bash
# View database stats
vector-cli stats

# Or via gRPC
grpcurl -plaintext localhost:50051 vector.VectorDB/GetStats
```

### Check Logs

```bash
# Systemd
sudo journalctl -u vector-db -f

# Docker
docker logs -f vector-db

# Kubernetes
kubectl logs -f deployment/vector-db -n vector
```

---

## Performance Issues

### Issue: Slow Search Queries

**Symptoms**:
- Search latency >100ms
- p99 latency >500ms
- Low QPS (<50)

**Diagnosis**:

```bash
# Check current ef_search setting
vector-cli config get hnsw.ef_search

# Monitor search latency
curl http://localhost:9090/metrics | grep vector_search_latency
```

**Solutions**:

#### 1. Lower ef_search

```bash
# Reduce ef_search for faster searches
export VECTOR_HNSW_EF_SEARCH=30  # Was 50
```

**Trade-off**: Lower recall (~92% vs 95%)

#### 2. Enable Query Cache

```yaml
# config.yaml
cache:
  enabled: true
  capacity: 50000     # Increase capacity
  ttl: 10m            # Longer TTL
```

**Expected**: 2-5x speedup on repeated queries

#### 3. Reduce Dataset Size

```bash
# Use namespace filtering
grpcurl -d '{"namespace":"subset1"}' localhost:50051 vector.VectorDB/Search
```

#### 4. Optimize Distance Function

```go
// Use dot product instead of cosine for normalized vectors
SearchRequest{
    DistanceMetric: "dot_product",  // Faster than cosine
}
```

**Speedup**: ~30% faster

#### 5. Enable Quantization

```bash
# Reduce memory pressure with quantization
vector-cli quantize enable --type product --subvectors 96
```

**Impact**: 4x less memory, slightly faster searches

---

### Issue: Slow Inserts

**Symptoms**:
- Insert latency >100ms
- Batch insert <100 vectors/sec
- High CPU during inserts

**Diagnosis**:

```bash
# Check insert metrics
curl http://localhost:9090/metrics | grep vector_insert_latency

# Monitor CPU usage
top -p $(pgrep vector-server)
```

**Solutions**:

#### 1. Use Batch Insert

```go
// Instead of individual inserts
for _, vec := range vectors {
    client.Insert(ctx, vec)  // Slow
}

// Use batch insert (4.5x faster)
stream, _ := client.BatchInsert(ctx)
for _, vec := range vectors {
    stream.Send(vec)
}
stream.CloseAndRecv()
```

#### 2. Lower ef_construction

```yaml
hnsw:
  ef_construction: 100  # Was 200
```

**Trade-off**: Slightly lower index quality (~93% recall vs 95%)

#### 3. Reduce M

```yaml
hnsw:
  m: 12  # Was 16
```

**Trade-off**: Lower recall, less memory

#### 4. Disable Sync Writes

```yaml
database:
  sync_writes: false  # Async writes (faster)
```

**Warning**: Less durable in case of crash

---

### Issue: High CPU Usage

**Symptoms**:
- CPU usage >80% idle
- Server slow to respond
- Request timeouts

**Diagnosis**:

```bash
# Check CPU usage
top -p $(pgrep vector-server)

# Profile CPU
curl http://localhost:9090/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof
```

**Solutions**:

#### 1. Limit Concurrent Connections

```yaml
server:
  max_connections: 500  # Was 1000
```

#### 2. Add More CPU Cores

```bash
# Kubernetes
kubectl set resources deployment/vector-db --limits=cpu=8 -n vector

# Docker
docker update --cpus="8" vector-db
```

#### 3. Optimize Distance Calculations

```bash
# Enable SIMD optimizations (AVX2)
CGO_ENABLED=1 go build -tags=avx2 ./cmd/server
```

**Speedup**: 4-8x faster distance calculations

#### 4. Use Connection Pooling

```go
// Client-side connection pooling
pool := grpc.NewPool(
    "localhost:50051",
    grpc.WithPoolSize(10),
)
```

---

## Memory Issues

### Issue: High Memory Usage

**Symptoms**:
- Memory usage >80% of available RAM
- OOM (Out of Memory) errors
- Swapping

**Diagnosis**:

```bash
# Check memory usage
free -h

# Check database memory
vector-cli stats --format json | jq '.memory_usage_bytes'

# Profile memory
curl http://localhost:9090/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

**Solutions**:

#### 1. Enable Product Quantization

```go
// Compress vectors (4x memory reduction)
import "github.com/therealutkarshpriyadarshi/vector/internal/quantization"

pq := quantization.NewProductQuantizer(dimensions, 96)
compressedVectors := pq.Compress(vectors)
```

**Impact**: 4x memory reduction, -2% recall

#### 2. Reduce M

```yaml
hnsw:
  m: 8  # Was 16
```

**Memory Savings**: ~50% reduction

#### 3. Reduce Cache Size

```yaml
cache:
  capacity: 1000  # Was 10000
```

#### 4. Add More RAM

```bash
# Kubernetes
kubectl set resources deployment/vector-db --limits=memory=16Gi -n vector
```

#### 5. Split Data Across Namespaces

```go
// Distribute data across multiple namespaces
client.Insert(ctx, &InsertRequest{
    Namespace: "shard_" + (id % 10),  // 10 shards
    Vector:    vector,
})
```

---

### Issue: Memory Leak

**Symptoms**:
- Memory grows over time
- No corresponding increase in data
- Eventually crashes with OOM

**Diagnosis**:

```bash
# Monitor memory growth
watch -n 1 'curl -s http://localhost:9090/metrics | grep go_memstats_alloc_bytes'

# Profile heap allocations
curl http://localhost:9090/debug/pprof/heap > heap.prof
go tool pprof -alloc_space heap.prof
```

**Solutions**:

#### 1. Check for Goroutine Leaks

```bash
# Check goroutine count
curl http://localhost:9090/debug/pprof/goroutine?debug=1

# Should be stable, not growing
```

#### 2. Close gRPC Streams

```go
// Always close streams
stream, err := client.BatchInsert(ctx)
defer stream.CloseSend()  // Important!
```

#### 3. Restart Server (Temporary Fix)

```bash
# Systemd
sudo systemctl restart vector-db

# Kubernetes
kubectl rollout restart deployment/vector-db -n vector
```

#### 4. Report Bug

If memory leak persists, report with:
- Heap profile
- Reproduction steps
- Server logs

---

## Search Quality Issues

### Issue: Low Recall (<90%)

**Symptoms**:
- Search results not accurate
- Missing expected results
- recall@10 <90%

**Diagnosis**:

```bash
# Calculate recall
vector-cli benchmark recall \
  --vectors 100000 \
  --queries 1000 \
  --ground-truth ground_truth.npy
```

**Solutions**:

#### 1. Increase ef_search

```yaml
hnsw:
  ef_search: 100  # Was 50
```

**Expected**: Recall increases to ~97%

#### 2. Increase M

```yaml
hnsw:
  m: 32  # Was 16
```

**Rebuild Index**: Required for changes to take effect

#### 3. Increase ef_construction

```yaml
hnsw:
  ef_construction: 400  # Was 200
```

**Note**: Only affects newly inserted vectors

#### 4. Check Distance Metric

```go
// Ensure consistent metric
// If vectors are normalized, use dot_product or cosine
// If not normalized, use euclidean

SearchRequest{
    DistanceMetric: "cosine",  // Match your embeddings
}
```

#### 5. Rebuild Index

```bash
# If parameters changed, rebuild for best quality
vector-cli rebuild-index --namespace default
```

---

### Issue: Inconsistent Results

**Symptoms**:
- Same query returns different results
- Non-deterministic behavior

**Diagnosis**:

```bash
# Run same query multiple times
for i in {1..10}; do
  vector-cli search --query query.json | jq '.results[0].id'
done
```

**Causes & Solutions**:

#### 1. Concurrent Modifications

**Problem**: Index being modified during search

**Solution**: Use read-write locks or read replicas

```go
// Server already uses RWMutex, but ensure no writes during search
index.mu.RLock()
defer index.mu.RUnlock()
results := index.Search(query, k)
```

#### 2. Tie-Breaking

**Problem**: Equal distances, random order

**Solution**: Add secondary sort by ID

```go
// Sort by distance, then ID for determinism
sort.Slice(results, func(i, j int) bool {
    if results[i].Distance == results[j].Distance {
        return results[i].ID < results[j].ID
    }
    return results[i].Distance < results[j].Distance
})
```

---

## Connection Issues

### Issue: Connection Refused

**Symptoms**:
```
Failed to dial: connection refused
```

**Diagnosis**:

```bash
# Check if server is running
ps aux | grep vector-server

# Check port
netstat -tulpn | grep 50051
```

**Solutions**:

#### 1. Start Server

```bash
# Systemd
sudo systemctl start vector-db

# Docker
docker start vector-db

# Manual
./bin/server
```

#### 2. Check Firewall

```bash
# Open port
sudo ufw allow 50051/tcp
```

#### 3. Check Binding Address

```yaml
server:
  host: "0.0.0.0"  # Not "localhost" if remote
  port: 50051
```

---

### Issue: Connection Timeout

**Symptoms**:
```
context deadline exceeded
```

**Diagnosis**:

```bash
# Check network latency
ping <server-ip>

# Check if server is responsive
grpcurl -plaintext localhost:50051 vector.VectorDB/HealthCheck
```

**Solutions**:

#### 1. Increase Client Timeout

```go
ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)  // Was 30s
defer cancel()

resp, err := client.Search(ctx, req)
```

#### 2. Increase Server Timeout

```yaml
server:
  request_timeout: 60s  # Was 30s
```

#### 3. Check Server Load

```bash
# If server is overloaded, add more resources
top
```

---

### Issue: TLS Handshake Failed

**Symptoms**:
```
transport: authentication handshake failed: x509: certificate signed by unknown authority
```

**Solutions**:

#### 1. Verify Certificate

```bash
# Check certificate
openssl x509 -in server.crt -text -noout

# Verify against CA
openssl verify -CAfile ca.crt server.crt
```

#### 2. Use Correct CA

```go
// Load CA certificate
creds, err := credentials.NewClientTLSFromFile("ca.crt", "")
conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(creds))
```

#### 3. Skip Verification (Development Only)

```go
// WARNING: Insecure, only for testing
conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
```

---

## Data Integrity Issues

### Issue: Data Corruption

**Symptoms**:
- Server crashes on startup
- "corrupted data" errors
- Unexpected search results

**Diagnosis**:

```bash
# Check data directory
ls -lh /var/lib/vector/

# Check logs for errors
sudo journalctl -u vector-db | grep -i corrupt
```

**Solutions**:

#### 1. Restore from Backup

```bash
# Stop server
sudo systemctl stop vector-db

# Restore backup
tar -xzf vector-backup-20250115.tar.gz -C /var/lib/vector/

# Start server
sudo systemctl start vector-db
```

#### 2. Enable WAL (Write-Ahead Log)

```yaml
database:
  enable_wal: true
```

#### 3. Enable Sync Writes

```yaml
database:
  sync_writes: true  # Slower but more durable
```

---

### Issue: Lost Data After Crash

**Symptoms**:
- Server crashed
- Recent inserts missing after restart

**Prevention**:

#### 1. Enable WAL

```yaml
database:
  enable_wal: true
```

#### 2. Regular Backups

```bash
# Cron job for hourly backups
0 * * * * /usr/local/bin/vector-backup.sh
```

#### 3. Use Sync Writes (Critical Data)

```yaml
database:
  sync_writes: true  # Force sync to disk
```

---

## Deployment Issues

### Issue: Pod CrashLoopBackOff (Kubernetes)

**Symptoms**:
```
kubectl get pods -n vector
NAME                         READY   STATUS             RESTARTS
vector-db-5d8f7c8d-xyz       0/1     CrashLoopBackOff   5
```

**Diagnosis**:

```bash
# Check logs
kubectl logs vector-db-5d8f7c8d-xyz -n vector

# Check events
kubectl describe pod vector-db-5d8f7c8d-xyz -n vector
```

**Common Causes**:

#### 1. Insufficient Memory

**Solution**: Increase memory limit

```yaml
resources:
  limits:
    memory: 8Gi  # Was 4Gi
```

#### 2. Missing PersistentVolume

**Solution**: Create PVC

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: vector-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 100Gi
```

#### 3. Port Conflict

**Solution**: Change port or fix conflicting service

---

### Issue: Container Won't Start (Docker)

**Diagnosis**:

```bash
# Check container status
docker ps -a | grep vector

# Check logs
docker logs vector-db
```

**Solutions**:

#### 1. Volume Permissions

```bash
# Fix permissions
sudo chown -R 1000:1000 /data/vector
```

#### 2. Port Already in Use

```bash
# Find process using port
sudo lsof -i :50051

# Kill process or change port
docker run -p 50052:50051 vector-db
```

---

## Monitoring and Debugging

### Enable Debug Logging

```yaml
# config.yaml
logging:
  level: debug  # Was info
```

Or via environment:

```bash
export VECTOR_LOG_LEVEL=debug
```

### Prometheus Metrics

```bash
# View all metrics
curl http://localhost:9090/metrics

# Filter vector metrics
curl http://localhost:9090/metrics | grep ^vector_
```

**Key Metrics**:
- `vector_search_latency_seconds`: Search performance
- `vector_insert_latency_seconds`: Insert performance
- `vector_cache_hit_rate`: Cache effectiveness
- `vector_index_memory_bytes`: Memory usage

### CPU Profiling

```bash
# Capture 30-second CPU profile
curl http://localhost:9090/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze
go tool pprof cpu.prof
(pprof) top10
(pprof) web
```

### Memory Profiling

```bash
# Capture heap profile
curl http://localhost:9090/debug/pprof/heap > heap.prof

# Analyze
go tool pprof heap.prof
(pprof) top10
(pprof) list Insert
```

### Distributed Tracing (Future)

```yaml
tracing:
  enabled: true
  provider: jaeger
  endpoint: "http://jaeger:14268/api/traces"
```

---

## Common Error Messages

### "namespace not found"

**Cause**: Namespace doesn't exist

**Solution**: Create namespace or use "default"

```go
client.Insert(ctx, &InsertRequest{
    Namespace: "default",  // Use existing namespace
})
```

### "vector dimension mismatch"

**Cause**: Vector has wrong dimensions

**Solution**: Ensure consistent dimensions

```go
// Check configured dimensions
config := GetConfig()
fmt.Printf("Expected dimensions: %d\n", config.HNSW.Dimensions)

// Resize or regenerate vectors
```

### "quota exceeded"

**Cause**: Namespace quota limit reached

**Solution**: Increase quota or delete old vectors

```yaml
database:
  max_namespaces: 200  # Was 100
```

### "index build failed"

**Cause**: Insufficient memory or disk space

**Solution**: Free resources or reduce dataset

```bash
# Check disk space
df -h

# Check memory
free -h
```

---

## Getting Help

### Community Support

- **GitHub Issues**: https://github.com/therealutkarshpriyadarshi/vector/issues
- **Discussions**: https://github.com/therealutkarshpriyadarshi/vector/discussions

### Reporting Bugs

Include:
1. **Version**: `vector-cli version`
2. **Environment**: OS, Docker/K8s, resources
3. **Configuration**: `config.yaml`
4. **Logs**: Last 100 lines of logs
5. **Steps to Reproduce**: Minimal example
6. **Metrics**: Relevant Prometheus metrics

### Performance Issues

Include:
1. **Dataset Size**: Number of vectors, dimensions
2. **Hardware**: CPU, RAM, disk
3. **Configuration**: HNSW parameters
4. **Metrics**: Latency, throughput, memory
5. **Benchmark Results**: Output of `./bin/benchmark`

---

## Preventive Measures

### Production Checklist

- [ ] Enable WAL for durability
- [ ] Configure automated backups (hourly)
- [ ] Set up monitoring (Prometheus + Grafana)
- [ ] Configure alerting (high latency, errors)
- [ ] Test restore procedure
- [ ] Document runbook
- [ ] Load test before launch
- [ ] Set resource limits
- [ ] Enable TLS
- [ ] Regular health checks

### Regular Maintenance

**Weekly**:
- Review metrics and logs
- Check disk space
- Verify backups

**Monthly**:
- Update to latest version
- Test disaster recovery
- Review and tune parameters
- Clean up old data

**Quarterly**:
- Load testing
- Security audit
- Capacity planning

---

## Next Steps

- [API Reference](api.md) - Complete API documentation
- [Deployment Guide](deployment.md) - Production deployment
- [Algorithms](algorithms.md) - HNSW and NSG deep dive
- [Benchmarks](benchmarks.md) - Performance testing

---

**Version**: 1.1.0
**Last Updated**: 2025-01-15
