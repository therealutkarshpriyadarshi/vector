package tenant

import (
	"fmt"
	"sync"
	"time"
)

// Quota represents resource limits for a tenant
type Quota struct {
	MaxVectors      int64 // Maximum number of vectors
	MaxStorageBytes int64 // Maximum storage in bytes
	MaxDimensions   int   // Maximum vector dimensions
	RateLimitQPS    int   // Queries per second limit
}

// Usage tracks current resource usage for a tenant
type Usage struct {
	VectorCount   int64
	StorageBytes  int64
	Dimensions    int
	LastQueryTime time.Time
	QueryCount    int64
	mu            sync.RWMutex
}

// Tenant represents a namespace with metadata and quotas
type Tenant struct {
	ID          string
	Name        string
	Namespace   string
	Quota       Quota
	Usage       Usage
	CreatedAt   time.Time
	UpdatedAt   time.Time
	IsActive    bool
	Metadata    map[string]interface{}
	mu          sync.RWMutex
}

// Manager handles tenant lifecycle and resource enforcement
type Manager struct {
	tenants map[string]*Tenant
	mu      sync.RWMutex
}

// NewManager creates a new tenant manager
func NewManager() *Manager {
	return &Manager{
		tenants: make(map[string]*Tenant),
	}
}

// CreateTenant creates a new tenant with specified quota
func (m *Manager) CreateTenant(namespace string, quota Quota) (*Tenant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tenants[namespace]; exists {
		return nil, fmt.Errorf("tenant with namespace '%s' already exists", namespace)
	}

	tenant := &Tenant{
		ID:        generateTenantID(namespace),
		Name:      namespace,
		Namespace: namespace,
		Quota:     quota,
		Usage: Usage{
			VectorCount:  0,
			StorageBytes: 0,
			Dimensions:   0,
			QueryCount:   0,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		IsActive:  true,
		Metadata:  make(map[string]interface{}),
	}

	m.tenants[namespace] = tenant
	return tenant, nil
}

// GetTenant retrieves a tenant by namespace
func (m *Manager) GetTenant(namespace string) (*Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenant, exists := m.tenants[namespace]
	if !exists {
		return nil, fmt.Errorf("tenant with namespace '%s' not found", namespace)
	}

	return tenant, nil
}

// DeleteTenant removes a tenant
func (m *Manager) DeleteTenant(namespace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tenants[namespace]; !exists {
		return fmt.Errorf("tenant with namespace '%s' not found", namespace)
	}

	delete(m.tenants, namespace)
	return nil
}

// ListTenants returns all tenants
func (m *Manager) ListTenants() []*Tenant {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tenants := make([]*Tenant, 0, len(m.tenants))
	for _, tenant := range m.tenants {
		tenants = append(tenants, tenant)
	}

	return tenants
}

// UpdateQuota updates the quota for a tenant
func (m *Manager) UpdateQuota(namespace string, quota Quota) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tenant, exists := m.tenants[namespace]
	if !exists {
		return fmt.Errorf("tenant with namespace '%s' not found", namespace)
	}

	tenant.mu.Lock()
	defer tenant.mu.Unlock()

	tenant.Quota = quota
	tenant.UpdatedAt = time.Now()

	return nil
}

// CheckVectorQuota checks if adding vectors would exceed quota
func (t *Tenant) CheckVectorQuota(count int64) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.Quota.MaxVectors > 0 && t.Usage.VectorCount+count > t.Quota.MaxVectors {
		return fmt.Errorf("vector quota exceeded: current=%d, requested=%d, max=%d",
			t.Usage.VectorCount, count, t.Quota.MaxVectors)
	}

	return nil
}

// CheckStorageQuota checks if adding storage would exceed quota
func (t *Tenant) CheckStorageQuota(bytes int64) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.Quota.MaxStorageBytes > 0 && t.Usage.StorageBytes+bytes > t.Quota.MaxStorageBytes {
		return fmt.Errorf("storage quota exceeded: current=%d, requested=%d, max=%d",
			t.Usage.StorageBytes, bytes, t.Quota.MaxStorageBytes)
	}

	return nil
}

// CheckDimensionQuota checks if dimensions match quota
func (t *Tenant) CheckDimensionQuota(dimensions int) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.Quota.MaxDimensions > 0 && dimensions > t.Quota.MaxDimensions {
		return fmt.Errorf("dimension quota exceeded: requested=%d, max=%d",
			dimensions, t.Quota.MaxDimensions)
	}

	return nil
}

// CheckRateLimit checks if query rate limit is exceeded
func (t *Tenant) CheckRateLimit() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Quota.RateLimitQPS <= 0 {
		// No rate limit
		return nil
	}

	now := time.Now()
	if now.Sub(t.Usage.LastQueryTime) < time.Second {
		// Within the same second
		if t.Usage.QueryCount >= int64(t.Quota.RateLimitQPS) {
			return fmt.Errorf("rate limit exceeded: %d queries per second (max: %d)",
				t.Usage.QueryCount, t.Quota.RateLimitQPS)
		}
	} else {
		// Reset counter for new second
		t.Usage.QueryCount = 0
		t.Usage.LastQueryTime = now
	}

	t.Usage.QueryCount++
	return nil
}

// IncrementVectorCount increments the vector count
func (t *Tenant) IncrementVectorCount(count int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Usage.VectorCount += count
	t.UpdatedAt = time.Now()
}

// DecrementVectorCount decrements the vector count
func (t *Tenant) DecrementVectorCount(count int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Usage.VectorCount -= count
	if t.Usage.VectorCount < 0 {
		t.Usage.VectorCount = 0
	}
	t.UpdatedAt = time.Now()
}

// UpdateStorageBytes updates the storage usage
func (t *Tenant) UpdateStorageBytes(bytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Usage.StorageBytes = bytes
	t.UpdatedAt = time.Now()
}

// SetDimensions sets the vector dimensions
func (t *Tenant) SetDimensions(dimensions int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Usage.Dimensions = dimensions
}

// GetUsagePercentage returns usage as percentage of quota
func (t *Tenant) GetUsagePercentage() map[string]float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	percentages := make(map[string]float64)

	if t.Quota.MaxVectors > 0 {
		percentages["vectors"] = float64(t.Usage.VectorCount) / float64(t.Quota.MaxVectors) * 100
	}

	if t.Quota.MaxStorageBytes > 0 {
		percentages["storage"] = float64(t.Usage.StorageBytes) / float64(t.Quota.MaxStorageBytes) * 100
	}

	return percentages
}

// IsOverQuota checks if any quota is exceeded
func (t *Tenant) IsOverQuota() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.Quota.MaxVectors > 0 && t.Usage.VectorCount > t.Quota.MaxVectors {
		return true
	}

	if t.Quota.MaxStorageBytes > 0 && t.Usage.StorageBytes > t.Quota.MaxStorageBytes {
		return true
	}

	return false
}

// SetActive sets the tenant active status
func (t *Tenant) SetActive(active bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.IsActive = active
	t.UpdatedAt = time.Now()
}

// GetMetadata retrieves tenant metadata
func (t *Tenant) GetMetadata(key string) (interface{}, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	value, exists := t.Metadata[key]
	return value, exists
}

// SetMetadata sets tenant metadata
func (t *Tenant) SetMetadata(key string, value interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Metadata[key] = value
	t.UpdatedAt = time.Now()
}

// generateTenantID generates a unique tenant ID
func generateTenantID(namespace string) string {
	return fmt.Sprintf("tenant_%s_%d", namespace, time.Now().UnixNano())
}

// DefaultQuota returns a default quota configuration
func DefaultQuota() Quota {
	return Quota{
		MaxVectors:      1000000, // 1M vectors
		MaxStorageBytes: 10 * 1024 * 1024 * 1024, // 10GB
		MaxDimensions:   2048,
		RateLimitQPS:    1000, // 1000 queries per second
	}
}

// UnlimitedQuota returns an unlimited quota configuration
func UnlimitedQuota() Quota {
	return Quota{
		MaxVectors:      -1, // Unlimited
		MaxStorageBytes: -1,
		MaxDimensions:   -1,
		RateLimitQPS:    -1,
	}
}
