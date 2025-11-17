package search

import (
	"testing"
)

func TestComparisonFilter_Equals(t *testing.T) {
	filter := Eq("category", "tech")

	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     bool
	}{
		{
			name:     "match",
			metadata: map[string]interface{}{"category": "tech"},
			want:     true,
		},
		{
			name:     "no match",
			metadata: map[string]interface{}{"category": "sports"},
			want:     false,
		},
		{
			name:     "field missing",
			metadata: map[string]interface{}{"type": "article"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filter.Match(tt.metadata); got != tt.want {
				t.Errorf("Eq().Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComparisonFilter_NotEquals(t *testing.T) {
	filter := Ne("status", "deleted")

	metadata1 := map[string]interface{}{"status": "active"}
	metadata2 := map[string]interface{}{"status": "deleted"}

	if !filter.Match(metadata1) {
		t.Error("Ne() should match 'active'")
	}
	if filter.Match(metadata2) {
		t.Error("Ne() should not match 'deleted'")
	}
}

func TestComparisonFilter_Numeric(t *testing.T) {
	tests := []struct {
		name     string
		filter   Filter
		value    int
		wantPass bool
	}{
		{"gt-pass", Gt("score", 50), 60, true},
		{"gt-fail", Gt("score", 50), 40, false},
		{"lt-pass", Lt("score", 50), 40, true},
		{"lt-fail", Lt("score", 50), 60, false},
		{"gte-pass-greater", Gte("score", 50), 60, true},
		{"gte-pass-equal", Gte("score", 50), 50, true},
		{"gte-fail", Gte("score", 50), 40, false},
		{"lte-pass-less", Lte("score", 50), 40, true},
		{"lte-pass-equal", Lte("score", 50), 50, true},
		{"lte-fail", Lte("score", 50), 60, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]interface{}{"score": tt.value}
			if got := tt.filter.Match(metadata); got != tt.wantPass {
				t.Errorf("%s: Match() = %v, want %v", tt.name, got, tt.wantPass)
			}
		})
	}
}

func TestRangeFilter(t *testing.T) {
	filter := Range("year", 2020, 2024)

	tests := []struct {
		name  string
		year  int
		want  bool
	}{
		{"within range", 2022, true},
		{"lower bound", 2020, true},
		{"upper bound", 2024, true},
		{"below range", 2019, false},
		{"above range", 2025, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]interface{}{"year": tt.year}
			if got := filter.Match(metadata); got != tt.want {
				t.Errorf("Range().Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInListFilter(t *testing.T) {
	filter := In("category", "tech", "science", "engineering")

	tests := []struct {
		name     string
		category string
		want     bool
	}{
		{"in list - first", "tech", true},
		{"in list - middle", "science", true},
		{"in list - last", "engineering", true},
		{"not in list", "sports", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]interface{}{"category": tt.category}
			if got := filter.Match(metadata); got != tt.want {
				t.Errorf("In().Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNotInListFilter(t *testing.T) {
	filter := NotIn("status", "deleted", "archived")

	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"not in list", "active", true},
		{"in list - deleted", "deleted", false},
		{"in list - archived", "archived", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]interface{}{"status": tt.status}
			if got := filter.Match(metadata); got != tt.want {
				t.Errorf("NotIn().Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGeoRadiusFilter(t *testing.T) {
	// San Francisco coordinates
	sfLat, sfLon := 37.7749, -122.4194

	// Create filter for 10km radius around SF
	filter := GeoRadius("location", sfLat, sfLon, 10)

	tests := []struct {
		name string
		lat  float64
		lon  float64
		want bool
	}{
		{
			name: "same location",
			lat:  sfLat,
			lon:  sfLon,
			want: true,
		},
		{
			name: "nearby (5km)",
			lat:  37.8, // Slightly north
			lon:  -122.4,
			want: true,
		},
		{
			name: "far away (100km+)",
			lat:  38.5, // Much further north
			lon:  -122.0,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]interface{}{
				"location": map[string]interface{}{
					"lat": tt.lat,
					"lon": tt.lon,
				},
			}
			if got := filter.Match(metadata); got != tt.want {
				t.Errorf("GeoRadius().Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGeoRadiusFilter_GeoPoint(t *testing.T) {
	filter := GeoRadius("location", 37.7749, -122.4194, 10)

	metadata := map[string]interface{}{
		"location": GeoPoint{Lat: 37.7749, Lon: -122.4194},
	}

	if !filter.Match(metadata) {
		t.Error("GeoRadius() should match exact GeoPoint")
	}
}

func TestCompositeFilter_And(t *testing.T) {
	filter := And(
		Eq("category", "tech"),
		Gt("year", 2020),
		Lt("year", 2025),
	)

	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     bool
	}{
		{
			name: "all conditions match",
			metadata: map[string]interface{}{
				"category": "tech",
				"year":     2023,
			},
			want: true,
		},
		{
			name: "category mismatch",
			metadata: map[string]interface{}{
				"category": "sports",
				"year":     2023,
			},
			want: false,
		},
		{
			name: "year too low",
			metadata: map[string]interface{}{
				"category": "tech",
				"year":     2019,
			},
			want: false,
		},
		{
			name: "year too high",
			metadata: map[string]interface{}{
				"category": "tech",
				"year":     2026,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filter.Match(tt.metadata); got != tt.want {
				t.Errorf("And().Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompositeFilter_Or(t *testing.T) {
	filter := Or(
		Eq("category", "tech"),
		Eq("category", "science"),
		Gt("priority", 5),
	)

	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     bool
	}{
		{
			name: "first condition matches",
			metadata: map[string]interface{}{
				"category": "tech",
				"priority": 3,
			},
			want: true,
		},
		{
			name: "second condition matches",
			metadata: map[string]interface{}{
				"category": "science",
				"priority": 2,
			},
			want: true,
		},
		{
			name: "third condition matches",
			metadata: map[string]interface{}{
				"category": "sports",
				"priority": 10,
			},
			want: true,
		},
		{
			name: "no conditions match",
			metadata: map[string]interface{}{
				"category": "sports",
				"priority": 2,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filter.Match(tt.metadata); got != tt.want {
				t.Errorf("Or().Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompositeFilter_Not(t *testing.T) {
	filter := Not(Eq("status", "deleted"))

	metadata1 := map[string]interface{}{"status": "active"}
	metadata2 := map[string]interface{}{"status": "deleted"}

	if !filter.Match(metadata1) {
		t.Error("Not() should match 'active'")
	}
	if filter.Match(metadata2) {
		t.Error("Not() should not match 'deleted'")
	}
}

func TestExistsFilter(t *testing.T) {
	existsFilter := Exists("optional_field")
	notExistsFilter := NotExists("optional_field")

	metadata1 := map[string]interface{}{"optional_field": "value"}
	metadata2 := map[string]interface{}{"other_field": "value"}

	// Exists tests
	if !existsFilter.Match(metadata1) {
		t.Error("Exists() should match when field exists")
	}
	if existsFilter.Match(metadata2) {
		t.Error("Exists() should not match when field missing")
	}

	// NotExists tests
	if notExistsFilter.Match(metadata1) {
		t.Error("NotExists() should not match when field exists")
	}
	if !notExistsFilter.Match(metadata2) {
		t.Error("NotExists() should match when field missing")
	}
}

func TestComplexCompositeFilter(t *testing.T) {
	// (category = "tech" OR category = "science") AND year >= 2020 AND NOT status = "deleted"
	filter := And(
		Or(
			Eq("category", "tech"),
			Eq("category", "science"),
		),
		Gte("year", 2020),
		Not(Eq("status", "deleted")),
	)

	tests := []struct {
		name     string
		metadata map[string]interface{}
		want     bool
	}{
		{
			name: "all conditions pass",
			metadata: map[string]interface{}{
				"category": "tech",
				"year":     2023,
				"status":   "active",
			},
			want: true,
		},
		{
			name: "category science, year 2020",
			metadata: map[string]interface{}{
				"category": "science",
				"year":     2020,
				"status":   "active",
			},
			want: true,
		},
		{
			name: "wrong category",
			metadata: map[string]interface{}{
				"category": "sports",
				"year":     2023,
				"status":   "active",
			},
			want: false,
		},
		{
			name: "year too old",
			metadata: map[string]interface{}{
				"category": "tech",
				"year":     2019,
				"status":   "active",
			},
			want: false,
		},
		{
			name: "status deleted",
			metadata: map[string]interface{}{
				"category": "tech",
				"year":     2023,
				"status":   "deleted",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filter.Match(tt.metadata); got != tt.want {
				t.Errorf("Complex filter Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHaversineDistance(t *testing.T) {
	// Test known distances
	sf := GeoPoint{Lat: 37.7749, Lon: -122.4194}  // San Francisco
	la := GeoPoint{Lat: 34.0522, Lon: -118.2437}  // Los Angeles
	ny := GeoPoint{Lat: 40.7128, Lon: -74.0060}   // New York

	// SF to SF should be 0
	dist := haversineDistance(sf, sf)
	if dist > 1 { // Allow for floating point error
		t.Errorf("Distance SF to SF = %f meters, want ~0", dist)
	}

	// SF to LA should be ~560 km
	dist = haversineDistance(sf, la)
	expectedMin, expectedMax := 540000.0, 580000.0 // 540-580 km
	if dist < expectedMin || dist > expectedMax {
		t.Errorf("Distance SF to LA = %f meters, want between %f and %f",
			dist, expectedMin, expectedMax)
	}

	// SF to NY should be ~4100 km
	dist = haversineDistance(sf, ny)
	expectedMin, expectedMax = 4000000.0, 4200000.0 // 4000-4200 km
	if dist < expectedMin || dist > expectedMax {
		t.Errorf("Distance SF to NY = %f meters, want between %f and %f",
			dist, expectedMin, expectedMax)
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  float64
	}{
		{"int", 42, 42.0},
		{"int64", int64(42), 42.0},
		{"float32", float32(42.5), 42.5},
		{"float64", 42.5, 42.5},
		{"uint", uint(42), 42.0},
		{"unknown", "string", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toFloat64(tt.value); got != tt.want {
				t.Errorf("toFloat64(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func BenchmarkComparisonFilter(b *testing.B) {
	filter := Eq("category", "tech")
	metadata := map[string]interface{}{"category": "tech"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Match(metadata)
	}
}

func BenchmarkCompositeFilter_And(b *testing.B) {
	filter := And(
		Eq("category", "tech"),
		Gt("year", 2020),
		Lt("score", 100),
	)
	metadata := map[string]interface{}{
		"category": "tech",
		"year":     2023,
		"score":    85,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Match(metadata)
	}
}

func BenchmarkGeoRadiusFilter(b *testing.B) {
	filter := GeoRadius("location", 37.7749, -122.4194, 10)
	metadata := map[string]interface{}{
		"location": map[string]interface{}{
			"lat": 37.8,
			"lon": -122.4,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.Match(metadata)
	}
}
