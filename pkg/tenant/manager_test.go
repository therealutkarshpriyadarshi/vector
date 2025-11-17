package tenant

import (
	"testing"
	"time"
)

func TestManager_CreateTenant(t *testing.T) {
	manager := NewManager()

	quota := Quota{
		MaxVectors:      10000,
		MaxStorageBytes: 1024 * 1024 * 100, // 100MB
		MaxDimensions:   768,
		RateLimitQPS:    100,
	}

	tenant, err := manager.CreateTenant("test-namespace", quota)
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	if tenant.Namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got '%s'", tenant.Namespace)
	}

	if tenant.Quota.MaxVectors != 10000 {
		t.Errorf("Expected MaxVectors 10000, got %d", tenant.Quota.MaxVectors)
	}

	if !tenant.IsActive {
		t.Error("Expected tenant to be active")
	}
}

func TestManager_CreateDuplicateTenant(t *testing.T) {
	manager := NewManager()
	quota := DefaultQuota()

	_, err := manager.CreateTenant("test", quota)
	if err != nil {
		t.Fatalf("First CreateTenant failed: %v", err)
	}

	_, err = manager.CreateTenant("test", quota)
	if err == nil {
		t.Error("Expected error when creating duplicate tenant")
	}
}

func TestManager_GetTenant(t *testing.T) {
	manager := NewManager()
	quota := DefaultQuota()

	created, err := manager.CreateTenant("test", quota)
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	retrieved, err := manager.GetTenant("test")
	if err != nil {
		t.Fatalf("GetTenant failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("Expected ID '%s', got '%s'", created.ID, retrieved.ID)
	}
}

func TestManager_GetNonexistentTenant(t *testing.T) {
	manager := NewManager()

	_, err := manager.GetTenant("nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent tenant")
	}
}

func TestManager_DeleteTenant(t *testing.T) {
	manager := NewManager()
	quota := DefaultQuota()

	_, err := manager.CreateTenant("test", quota)
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	err = manager.DeleteTenant("test")
	if err != nil {
		t.Fatalf("DeleteTenant failed: %v", err)
	}

	_, err = manager.GetTenant("test")
	if err == nil {
		t.Error("Expected error when getting deleted tenant")
	}
}

func TestManager_ListTenants(t *testing.T) {
	manager := NewManager()
	quota := DefaultQuota()

	_, _ = manager.CreateTenant("tenant1", quota)
	_, _ = manager.CreateTenant("tenant2", quota)
	_, _ = manager.CreateTenant("tenant3", quota)

	tenants := manager.ListTenants()
	if len(tenants) != 3 {
		t.Errorf("Expected 3 tenants, got %d", len(tenants))
	}
}

func TestManager_UpdateQuota(t *testing.T) {
	manager := NewManager()
	quota := DefaultQuota()

	_, err := manager.CreateTenant("test", quota)
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	newQuota := Quota{
		MaxVectors:      50000,
		MaxStorageBytes: 1024 * 1024 * 500,
		MaxDimensions:   1024,
		RateLimitQPS:    500,
	}

	err = manager.UpdateQuota("test", newQuota)
	if err != nil {
		t.Fatalf("UpdateQuota failed: %v", err)
	}

	tenant, _ := manager.GetTenant("test")
	if tenant.Quota.MaxVectors != 50000 {
		t.Errorf("Expected MaxVectors 50000, got %d", tenant.Quota.MaxVectors)
	}
}

func TestTenant_CheckVectorQuota(t *testing.T) {
	tenant := &Tenant{
		Quota: Quota{MaxVectors: 100},
		Usage: Usage{VectorCount: 90},
	}

	// Should pass
	err := tenant.CheckVectorQuota(5)
	if err != nil {
		t.Errorf("CheckVectorQuota should pass: %v", err)
	}

	// Should fail
	err = tenant.CheckVectorQuota(20)
	if err == nil {
		t.Error("Expected CheckVectorQuota to fail when exceeding quota")
	}
}

func TestTenant_CheckStorageQuota(t *testing.T) {
	tenant := &Tenant{
		Quota: Quota{MaxStorageBytes: 1000},
		Usage: Usage{StorageBytes: 800},
	}

	// Should pass
	err := tenant.CheckStorageQuota(100)
	if err != nil {
		t.Errorf("CheckStorageQuota should pass: %v", err)
	}

	// Should fail
	err = tenant.CheckStorageQuota(300)
	if err == nil {
		t.Error("Expected CheckStorageQuota to fail when exceeding quota")
	}
}

func TestTenant_CheckDimensionQuota(t *testing.T) {
	tenant := &Tenant{
		Quota: Quota{MaxDimensions: 768},
	}

	// Should pass
	err := tenant.CheckDimensionQuota(768)
	if err != nil {
		t.Errorf("CheckDimensionQuota should pass: %v", err)
	}

	// Should fail
	err = tenant.CheckDimensionQuota(1024)
	if err == nil {
		t.Error("Expected CheckDimensionQuota to fail when exceeding quota")
	}
}

func TestTenant_CheckRateLimit(t *testing.T) {
	tenant := &Tenant{
		Quota: Quota{RateLimitQPS: 5},
		Usage: Usage{
			QueryCount:    0,
			LastQueryTime: time.Now(),
		},
	}

	// First 5 should pass
	for i := 0; i < 5; i++ {
		err := tenant.CheckRateLimit()
		if err != nil {
			t.Errorf("Query %d should pass: %v", i+1, err)
		}
	}

	// 6th should fail
	err := tenant.CheckRateLimit()
	if err == nil {
		t.Error("Expected CheckRateLimit to fail after exceeding limit")
	}

	// Wait 1 second and try again
	time.Sleep(1100 * time.Millisecond)
	err = tenant.CheckRateLimit()
	if err != nil {
		t.Errorf("CheckRateLimit should pass after reset: %v", err)
	}
}

func TestTenant_IncrementDecrementVectorCount(t *testing.T) {
	tenant := &Tenant{
		Usage: Usage{VectorCount: 100},
	}

	tenant.IncrementVectorCount(50)
	if tenant.Usage.VectorCount != 150 {
		t.Errorf("Expected count 150, got %d", tenant.Usage.VectorCount)
	}

	tenant.DecrementVectorCount(30)
	if tenant.Usage.VectorCount != 120 {
		t.Errorf("Expected count 120, got %d", tenant.Usage.VectorCount)
	}

	// Test underflow protection
	tenant.DecrementVectorCount(200)
	if tenant.Usage.VectorCount != 0 {
		t.Errorf("Expected count 0, got %d", tenant.Usage.VectorCount)
	}
}

func TestTenant_GetUsagePercentage(t *testing.T) {
	tenant := &Tenant{
		Quota: Quota{
			MaxVectors:      1000,
			MaxStorageBytes: 10000,
		},
		Usage: Usage{
			VectorCount:  500,
			StorageBytes: 2500,
		},
	}

	percentages := tenant.GetUsagePercentage()

	if percentages["vectors"] != 50.0 {
		t.Errorf("Expected vectors 50%%, got %.2f%%", percentages["vectors"])
	}

	if percentages["storage"] != 25.0 {
		t.Errorf("Expected storage 25%%, got %.2f%%", percentages["storage"])
	}
}

func TestTenant_IsOverQuota(t *testing.T) {
	tenant := &Tenant{
		Quota: Quota{
			MaxVectors:      100,
			MaxStorageBytes: 1000,
		},
		Usage: Usage{
			VectorCount:  90,
			StorageBytes: 900,
		},
	}

	if tenant.IsOverQuota() {
		t.Error("Expected tenant to not be over quota")
	}

	tenant.Usage.VectorCount = 110
	if !tenant.IsOverQuota() {
		t.Error("Expected tenant to be over quota")
	}
}

func TestTenant_Metadata(t *testing.T) {
	tenant := &Tenant{
		Metadata: make(map[string]interface{}),
	}

	tenant.SetMetadata("owner", "test-user")
	tenant.SetMetadata("plan", "premium")

	owner, exists := tenant.GetMetadata("owner")
	if !exists {
		t.Error("Expected metadata 'owner' to exist")
	}
	if owner != "test-user" {
		t.Errorf("Expected owner 'test-user', got '%v'", owner)
	}

	_, exists = tenant.GetMetadata("nonexistent")
	if exists {
		t.Error("Expected metadata 'nonexistent' to not exist")
	}
}

func TestDefaultQuota(t *testing.T) {
	quota := DefaultQuota()

	if quota.MaxVectors <= 0 {
		t.Error("Expected positive MaxVectors in default quota")
	}

	if quota.MaxStorageBytes <= 0 {
		t.Error("Expected positive MaxStorageBytes in default quota")
	}
}

func TestUnlimitedQuota(t *testing.T) {
	quota := UnlimitedQuota()

	if quota.MaxVectors != -1 {
		t.Error("Expected unlimited MaxVectors (-1)")
	}

	if quota.MaxStorageBytes != -1 {
		t.Error("Expected unlimited MaxStorageBytes (-1)")
	}
}

func TestTenant_ConcurrentAccess(t *testing.T) {
	tenant := &Tenant{
		Quota: Quota{MaxVectors: 100000},
		Usage: Usage{VectorCount: 0},
	}

	// Concurrent increments
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			tenant.IncrementVectorCount(1)
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	if tenant.Usage.VectorCount != 100 {
		t.Errorf("Expected count 100, got %d (race condition)", tenant.Usage.VectorCount)
	}
}
