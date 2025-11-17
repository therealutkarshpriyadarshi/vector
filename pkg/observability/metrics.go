package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"time"
)

// Metrics holds all Prometheus metrics for the vector database
type Metrics struct {
	// Request metrics
	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	RequestErrors    *prometheus.CounterVec

	// Vector operation metrics
	VectorsInserted  prometheus.Counter
	VectorsDeleted   prometheus.Counter
	VectorsUpdated   prometheus.Counter
	VectorsSearched  prometheus.Counter

	// Index metrics
	IndexSize        *prometheus.GaugeVec
	IndexMemoryBytes *prometheus.GaugeVec
	IndexMaxLayer    *prometheus.GaugeVec

	// Search metrics
	SearchLatency    prometheus.Histogram
	SearchRecall     prometheus.Histogram
	SearchResultSize prometheus.Histogram

	// Cache metrics
	CacheHits   prometheus.Counter
	CacheMisses prometheus.Counter
	CacheSize   prometheus.Gauge

	// Batch operation metrics
	BatchInsertTotal    prometheus.Counter
	BatchInsertDuration prometheus.Histogram
	BatchDeleteTotal    prometheus.Counter
	BatchDeleteDuration prometheus.Histogram

	// Tenant metrics
	TenantsTotal     prometheus.Gauge
	TenantQuotaUsage *prometheus.GaugeVec

	// System metrics
	GoroutinesCount prometheus.Gauge
	MemoryUsage     prometheus.Gauge
	CPUUsage        prometheus.Gauge
}

// NewMetrics creates and registers all Prometheus metrics
func NewMetrics() *Metrics {
	m := &Metrics{
		// Request metrics
		RequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vectordb_requests_total",
				Help: "Total number of requests by method and status",
			},
			[]string{"method", "status"},
		),
		RequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "vectordb_request_duration_seconds",
				Help:    "Request duration in seconds",
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method"},
		),
		RequestErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vectordb_request_errors_total",
				Help: "Total number of request errors by method and error type",
			},
			[]string{"method", "error_type"},
		),

		// Vector operation metrics
		VectorsInserted: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "vectordb_vectors_inserted_total",
				Help: "Total number of vectors inserted",
			},
		),
		VectorsDeleted: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "vectordb_vectors_deleted_total",
				Help: "Total number of vectors deleted",
			},
		),
		VectorsUpdated: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "vectordb_vectors_updated_total",
				Help: "Total number of vectors updated",
			},
		),
		VectorsSearched: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "vectordb_vectors_searched_total",
				Help: "Total number of search operations",
			},
		),

		// Index metrics
		IndexSize: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vectordb_index_size",
				Help: "Number of vectors in index by namespace",
			},
			[]string{"namespace"},
		),
		IndexMemoryBytes: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vectordb_index_memory_bytes",
				Help: "Memory usage of index in bytes by namespace",
			},
			[]string{"namespace"},
		),
		IndexMaxLayer: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vectordb_index_max_layer",
				Help: "Maximum layer in HNSW graph by namespace",
			},
			[]string{"namespace"},
		),

		// Search metrics
		SearchLatency: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "vectordb_search_latency_seconds",
				Help:    "Search latency in seconds",
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
			},
		),
		SearchRecall: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "vectordb_search_recall",
				Help:    "Search recall percentage (0-1)",
				Buckets: []float64{.8, .85, .9, .92, .94, .95, .96, .97, .98, .99, 1.0},
			},
		),
		SearchResultSize: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "vectordb_search_result_size",
				Help:    "Number of results returned by search",
				Buckets: []float64{1, 5, 10, 20, 50, 100, 200, 500, 1000},
			},
		),

		// Cache metrics
		CacheHits: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "vectordb_cache_hits_total",
				Help: "Total number of cache hits",
			},
		),
		CacheMisses: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "vectordb_cache_misses_total",
				Help: "Total number of cache misses",
			},
		),
		CacheSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "vectordb_cache_size",
				Help: "Current number of entries in cache",
			},
		),

		// Batch operation metrics
		BatchInsertTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "vectordb_batch_insert_total",
				Help: "Total number of batch insert operations",
			},
		),
		BatchInsertDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "vectordb_batch_insert_duration_seconds",
				Help:    "Batch insert duration in seconds",
				Buckets: []float64{.1, .5, 1, 2.5, 5, 10, 30, 60, 120},
			},
		),
		BatchDeleteTotal: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "vectordb_batch_delete_total",
				Help: "Total number of batch delete operations",
			},
		),
		BatchDeleteDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "vectordb_batch_delete_duration_seconds",
				Help:    "Batch delete duration in seconds",
				Buckets: []float64{.1, .5, 1, 2.5, 5, 10, 30, 60},
			},
		),

		// Tenant metrics
		TenantsTotal: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "vectordb_tenants_total",
				Help: "Total number of active tenants",
			},
		),
		TenantQuotaUsage: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vectordb_tenant_quota_usage",
				Help: "Tenant quota usage percentage by namespace and resource",
			},
			[]string{"namespace", "resource"},
		),

		// System metrics
		GoroutinesCount: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "vectordb_goroutines",
				Help: "Current number of goroutines",
			},
		),
		MemoryUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "vectordb_memory_bytes",
				Help: "Memory usage in bytes",
			},
		),
		CPUUsage: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "vectordb_cpu_usage",
				Help: "CPU usage percentage",
			},
		),
	}

	return m
}

// RecordRequest records a request with duration and status
func (m *Metrics) RecordRequest(method, status string, duration time.Duration) {
	m.RequestsTotal.WithLabelValues(method, status).Inc()
	m.RequestDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// RecordError records an error
func (m *Metrics) RecordError(method, errorType string) {
	m.RequestErrors.WithLabelValues(method, errorType).Inc()
}

// RecordInsert records a vector insertion
func (m *Metrics) RecordInsert(namespace string, count int) {
	m.VectorsInserted.Add(float64(count))
	// Update index size (this should be called after successful insert)
}

// RecordDelete records a vector deletion
func (m *Metrics) RecordDelete(namespace string, count int) {
	m.VectorsDeleted.Add(float64(count))
}

// RecordUpdate records a vector update
func (m *Metrics) RecordUpdate(namespace string, count int) {
	m.VectorsUpdated.Add(float64(count))
}

// RecordSearch records a search operation
func (m *Metrics) RecordSearch(duration time.Duration, resultSize int) {
	m.VectorsSearched.Inc()
	m.SearchLatency.Observe(duration.Seconds())
	m.SearchResultSize.Observe(float64(resultSize))
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit() {
	m.CacheHits.Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss() {
	m.CacheMisses.Inc()
}

// UpdateIndexSize updates the index size metric
func (m *Metrics) UpdateIndexSize(namespace string, size int) {
	m.IndexSize.WithLabelValues(namespace).Set(float64(size))
}

// UpdateIndexMemory updates the index memory metric
func (m *Metrics) UpdateIndexMemory(namespace string, bytes int64) {
	m.IndexMemoryBytes.WithLabelValues(namespace).Set(float64(bytes))
}

// UpdateIndexMaxLayer updates the max layer metric
func (m *Metrics) UpdateIndexMaxLayer(namespace string, maxLayer int) {
	m.IndexMaxLayer.WithLabelValues(namespace).Set(float64(maxLayer))
}

// RecordBatchInsert records a batch insert operation
func (m *Metrics) RecordBatchInsert(duration time.Duration, count int) {
	m.BatchInsertTotal.Inc()
	m.BatchInsertDuration.Observe(duration.Seconds())
	m.VectorsInserted.Add(float64(count))
}

// RecordBatchDelete records a batch delete operation
func (m *Metrics) RecordBatchDelete(duration time.Duration, count int) {
	m.BatchDeleteTotal.Inc()
	m.BatchDeleteDuration.Observe(duration.Seconds())
	m.VectorsDeleted.Add(float64(count))
}

// UpdateTenantCount updates the total tenant count
func (m *Metrics) UpdateTenantCount(count int) {
	m.TenantsTotal.Set(float64(count))
}

// UpdateTenantQuota updates tenant quota usage
func (m *Metrics) UpdateTenantQuota(namespace, resource string, usage float64) {
	m.TenantQuotaUsage.WithLabelValues(namespace, resource).Set(usage)
}

// UpdateGoroutineCount updates goroutine count
func (m *Metrics) UpdateGoroutineCount(count int) {
	m.GoroutinesCount.Set(float64(count))
}

// UpdateMemoryUsage updates memory usage
func (m *Metrics) UpdateMemoryUsage(bytes uint64) {
	m.MemoryUsage.Set(float64(bytes))
}

// UpdateCPUUsage updates CPU usage
func (m *Metrics) UpdateCPUUsage(percentage float64) {
	m.CPUUsage.Set(percentage)
}

// UpdateCacheSize updates cache size
func (m *Metrics) UpdateCacheSize(size int) {
	m.CacheSize.Set(float64(size))
}

// GetCacheHitRate returns the cache hit rate
func (m *Metrics) GetCacheHitRate() float64 {
	// This would need to query the actual counter values
	// For now, return 0 as placeholder
	return 0.0
}
