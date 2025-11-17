package hnsw

import (
	"math"
	"testing"
)

const epsilon = 1e-6

func almostEqual(a, b float32) bool {
	return math.Abs(float64(a-b)) < epsilon
}

// TestCosineSimilarity tests the cosine similarity metric
func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0, // distance = 1 - similarity(1) = 0
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 1.0, // distance = 1 - similarity(0) = 1
		},
		{
			name:     "opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: 2.0, // distance = 1 - similarity(-1) = 2
		},
		{
			name:     "similar vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{2, 4, 6},
			expected: 0.0, // scaled versions, perfectly similar
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			if !almostEqual(result, tt.expected) {
				t.Errorf("CosineSimilarity(%v, %v) = %v, expected %v",
					tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestEuclideanDistance tests the Euclidean distance metric
func TestEuclideanDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "unit distance",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "3-4-5 triangle",
			a:        []float32{0, 0},
			b:        []float32{3, 4},
			expected: 5.0,
		},
		{
			name:     "negative coordinates",
			a:        []float32{-1, -1},
			b:        []float32{1, 1},
			expected: float32(math.Sqrt(8)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EuclideanDistance(tt.a, tt.b)
			if !almostEqual(result, tt.expected) {
				t.Errorf("EuclideanDistance(%v, %v) = %v, expected %v",
					tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestDotProduct tests the dot product distance metric
func TestDotProduct(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
		},
		{
			name:     "parallel vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3},
			expected: -14.0, // -(1 + 4 + 9) = -14
		},
		{
			name:     "simple case",
			a:        []float32{1, 2},
			b:        []float32{3, 4},
			expected: -11.0, // -(3 + 8) = -11
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DotProduct(tt.a, tt.b)
			if !almostEqual(result, tt.expected) {
				t.Errorf("DotProduct(%v, %v) = %v, expected %v",
					tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestSquaredEuclideanDistance tests the squared Euclidean distance metric
func TestSquaredEuclideanDistance(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1, 2, 3},
			b:        []float32{1, 2, 3},
			expected: 0.0,
		},
		{
			name:     "unit distance squared",
			a:        []float32{0, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
		},
		{
			name:     "3-4 right triangle",
			a:        []float32{0, 0},
			b:        []float32{3, 4},
			expected: 25.0, // 3^2 + 4^2 = 25
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SquaredEuclideanDistance(tt.a, tt.b)
			if !almostEqual(result, tt.expected) {
				t.Errorf("SquaredEuclideanDistance(%v, %v) = %v, expected %v",
					tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// TestDimensionMismatch tests that distance functions panic on mismatched dimensions
func TestDimensionMismatch(t *testing.T) {
	a := []float32{1, 2, 3}
	b := []float32{1, 2}

	tests := []struct {
		name string
		fn   func()
	}{
		{
			name: "CosineSimilarity",
			fn:   func() { CosineSimilarity(a, b) },
		},
		{
			name: "EuclideanDistance",
			fn:   func() { EuclideanDistance(a, b) },
		},
		{
			name: "DotProduct",
			fn:   func() { DotProduct(a, b) },
		},
		{
			name: "SquaredEuclideanDistance",
			fn:   func() { SquaredEuclideanDistance(a, b) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s should panic on dimension mismatch", tt.name)
				}
			}()
			tt.fn()
		})
	}
}

// Benchmarks for 768-dimensional vectors (common embedding size)
func generateVector768() []float32 {
	vec := make([]float32, 768)
	for i := range vec {
		vec[i] = float32(i) * 0.001
	}
	return vec
}

func BenchmarkCosineSimilarity768(b *testing.B) {
	vec1 := generateVector768()
	vec2 := generateVector768()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CosineSimilarity(vec1, vec2)
	}
}

func BenchmarkEuclideanDistance768(b *testing.B) {
	vec1 := generateVector768()
	vec2 := generateVector768()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EuclideanDistance(vec1, vec2)
	}
}

func BenchmarkDotProduct768(b *testing.B) {
	vec1 := generateVector768()
	vec2 := generateVector768()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DotProduct(vec1, vec2)
	}
}

func BenchmarkSquaredEuclideanDistance768(b *testing.B) {
	vec1 := generateVector768()
	vec2 := generateVector768()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SquaredEuclideanDistance(vec1, vec2)
	}
}

// Benchmarks for different vector dimensions
func BenchmarkCosineSimilarity128(b *testing.B) {
	vec1 := make([]float32, 128)
	vec2 := make([]float32, 128)
	for i := range vec1 {
		vec1[i] = float32(i) * 0.01
		vec2[i] = float32(i) * 0.01
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CosineSimilarity(vec1, vec2)
	}
}

func BenchmarkCosineSimilarity1536(b *testing.B) {
	vec1 := make([]float32, 1536)
	vec2 := make([]float32, 1536)
	for i := range vec1 {
		vec1[i] = float32(i) * 0.001
		vec2[i] = float32(i) * 0.001
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CosineSimilarity(vec1, vec2)
	}
}
