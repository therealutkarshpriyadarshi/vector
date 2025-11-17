package quantization

import (
	"math"
	"math/rand"
	"testing"
)

func TestScalarQuantizer_Train(t *testing.T) {
	q := NewScalarQuantizer()

	// Create training data
	vectors := [][]float32{
		{0.0, 0.5, 1.0},
		{0.2, 0.6, 0.8},
		{0.1, 0.4, 0.9},
	}

	err := q.Train(vectors)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	if q.min >= q.max {
		t.Errorf("Invalid min/max: min=%f, max=%f", q.min, q.max)
	}
}

func TestScalarQuantizer_Quantize(t *testing.T) {
	q := NewScalarQuantizer()

	// Train on sample data
	vectors := [][]float32{
		{0.0, 0.5, 1.0},
		{0.2, 0.6, 0.8},
	}
	q.Train(vectors)

	// Quantize a vector
	quantized := q.Quantize([]float32{0.1, 0.55, 0.9})

	if len(quantized) != 3 {
		t.Errorf("Expected length 3, got %d", len(quantized))
	}

	// Check values are in valid range
	for i, val := range quantized {
		if val < -127 || val > 127 {
			t.Errorf("Value %d out of range: %d", i, val)
		}
	}
}

func TestScalarQuantizer_Dequantize(t *testing.T) {
	q := NewScalarQuantizer()

	// Train
	vectors := [][]float32{
		{0.0, 1.0},
		{0.5, 0.5},
	}
	q.Train(vectors)

	// Quantize and dequantize
	original := []float32{0.3, 0.7}
	quantized := q.Quantize(original)
	dequantized := q.Dequantize(quantized)

	// Check reconstruction error
	for i := range original {
		error := math.Abs(float64(original[i] - dequantized[i]))
		if error > 0.1 { // Allow 10% error
			t.Errorf("Large reconstruction error at %d: original=%f, dequantized=%f, error=%f",
				i, original[i], dequantized[i], error)
		}
	}
}

func TestScalarQuantizer_RoundTrip(t *testing.T) {
	q := NewScalarQuantizer()

	// Generate random training data
	vectors := make([][]float32, 100)
	for i := 0; i < 100; i++ {
		vectors[i] = make([]float32, 768)
		for j := 0; j < 768; j++ {
			vectors[i][j] = rand.Float32()
		}
	}

	q.Train(vectors)

	// Test round-trip
	testVector := make([]float32, 768)
	for j := 0; j < 768; j++ {
		testVector[j] = rand.Float32()
	}

	quantized := q.Quantize(testVector)
	dequantized := q.Dequantize(quantized)

	// Compute average error
	var totalError float64
	for i := range testVector {
		error := math.Abs(float64(testVector[i] - dequantized[i]))
		totalError += error
	}
	avgError := totalError / float64(len(testVector))

	if avgError > 0.05 { // 5% average error threshold
		t.Errorf("Average reconstruction error too high: %f", avgError)
	}
}

func TestScalarQuantizer_QuantizeBatch(t *testing.T) {
	q := NewScalarQuantizer()

	vectors := [][]float32{
		{0.0, 0.5, 1.0},
		{0.2, 0.6, 0.8},
		{0.1, 0.4, 0.9},
	}

	q.Train(vectors)

	quantized := q.QuantizeBatch(vectors)

	if len(quantized) != 3 {
		t.Errorf("Expected 3 quantized vectors, got %d", len(quantized))
	}

	for i, qvec := range quantized {
		if len(qvec) != len(vectors[i]) {
			t.Errorf("Vector %d: expected length %d, got %d", i, len(vectors[i]), len(qvec))
		}
	}
}

func TestScalarQuantizer_DequantizeBatch(t *testing.T) {
	q := NewScalarQuantizer()

	vectors := [][]float32{
		{0.0, 0.5, 1.0},
		{0.2, 0.6, 0.8},
	}

	q.Train(vectors)

	quantized := q.QuantizeBatch(vectors)
	dequantized := q.DequantizeBatch(quantized)

	if len(dequantized) != len(vectors) {
		t.Errorf("Expected %d dequantized vectors, got %d", len(vectors), len(dequantized))
	}
}

func TestScalarQuantizer_GetMemoryReduction(t *testing.T) {
	q := NewScalarQuantizer()

	reduction := q.GetMemoryReduction()

	if reduction != 4.0 {
		t.Errorf("Expected 4x memory reduction, got %f", reduction)
	}
}

func TestScalarQuantizer_Parameters(t *testing.T) {
	q := NewScalarQuantizer()

	// Set parameters
	q.SetParameters(0.0, 1.0, 254.0, -127.0)

	min, max, scale, offset := q.GetParameters()

	if min != 0.0 || max != 1.0 || scale != 254.0 || offset != -127.0 {
		t.Errorf("Parameters mismatch: min=%f, max=%f, scale=%f, offset=%f",
			min, max, scale, offset)
	}
}

func TestDistanceInt8(t *testing.T) {
	a := []int8{10, 20, 30}
	b := []int8{12, 22, 32}

	dist := DistanceInt8(a, b)

	// Distance should be sqrt(2^2 + 2^2 + 2^2) = sqrt(12) ≈ 3.46
	expected := float32(math.Sqrt(12))
	if math.Abs(float64(dist-expected)) > 0.01 {
		t.Errorf("Expected distance %f, got %f", expected, dist)
	}
}

func TestDistanceInt8_DifferentLengths(t *testing.T) {
	a := []int8{10, 20, 30}
	b := []int8{12, 22}

	dist := DistanceInt8(a, b)

	if dist != float32(math.MaxFloat32) {
		t.Errorf("Expected MaxFloat32 for different lengths, got %f", dist)
	}
}

func TestDotProductInt8(t *testing.T) {
	a := []int8{1, 2, 3}
	b := []int8{4, 5, 6}

	dot := DotProductInt8(a, b)

	// 1*4 + 2*5 + 3*6 = 4 + 10 + 18 = 32
	expected := int64(32)
	if dot != expected {
		t.Errorf("Expected dot product %d, got %d", expected, dot)
	}
}

func TestProductQuantizer_Train(t *testing.T) {
	pq := NewProductQuantizer(4, 8) // 4 subvectors, 8 bits per code

	// Create training data (768 dimensions)
	vectors := make([][]float32, 100)
	for i := 0; i < 100; i++ {
		vectors[i] = make([]float32, 768)
		for j := 0; j < 768; j++ {
			vectors[i][j] = rand.Float32()
		}
	}

	err := pq.Train(vectors, 10)
	if err != nil {
		t.Fatalf("Train failed: %v", err)
	}

	if len(pq.codebooks) != 4 {
		t.Errorf("Expected 4 codebooks, got %d", len(pq.codebooks))
	}

	// Each codebook should have 2^8 = 256 centroids
	for i, codebook := range pq.codebooks {
		if len(codebook) != 256 {
			t.Errorf("Codebook %d: expected 256 centroids, got %d", i, len(codebook))
		}
	}
}

func TestProductQuantizer_Encode(t *testing.T) {
	pq := NewProductQuantizer(4, 8)

	// Train
	vectors := make([][]float32, 100)
	for i := 0; i < 100; i++ {
		vectors[i] = make([]float32, 768)
		for j := 0; j < 768; j++ {
			vectors[i][j] = rand.Float32()
		}
	}
	pq.Train(vectors, 5)

	// Encode a vector
	testVector := make([]float32, 768)
	for j := 0; j < 768; j++ {
		testVector[j] = rand.Float32()
	}

	codes := pq.Encode(testVector)

	if len(codes) != 4 {
		t.Errorf("Expected 4 codes, got %d", len(codes))
	}
}

func TestProductQuantizer_Decode(t *testing.T) {
	pq := NewProductQuantizer(4, 8)

	// Train
	vectors := make([][]float32, 100)
	for i := 0; i < 100; i++ {
		vectors[i] = make([]float32, 768)
		for j := 0; j < 768; j++ {
			vectors[i][j] = rand.Float32()
		}
	}
	pq.Train(vectors, 5)

	// Encode and decode
	testVector := make([]float32, 768)
	for j := 0; j < 768; j++ {
		testVector[j] = rand.Float32()
	}

	codes := pq.Encode(testVector)
	decoded := pq.Decode(codes)

	if len(decoded) != 768 {
		t.Errorf("Expected 768 dimensions, got %d", len(decoded))
	}
}

func TestProductQuantizer_CompressionRatio(t *testing.T) {
	pq := NewProductQuantizer(4, 8)

	ratio := pq.GetCompressionRatio(768)

	// 768 * 4 bytes / 4 codes = 768
	expected := float32(768.0)
	if math.Abs(float64(ratio-expected)) > 0.01 {
		t.Errorf("Expected compression ratio %f, got %f", expected, ratio)
	}
}

func BenchmarkScalarQuantize(b *testing.B) {
	q := NewScalarQuantizer()

	// Train
	vectors := make([][]float32, 1000)
	for i := 0; i < 1000; i++ {
		vectors[i] = make([]float32, 768)
		for j := 0; j < 768; j++ {
			vectors[i][j] = rand.Float32()
		}
	}
	q.Train(vectors)

	testVector := make([]float32, 768)
	for j := 0; j < 768; j++ {
		testVector[j] = rand.Float32()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Quantize(testVector)
	}
}

func BenchmarkScalarDequantize(b *testing.B) {
	q := NewScalarQuantizer()
	q.SetParameters(0.0, 1.0, 254.0, -127.0)

	quantized := make([]int8, 768)
	for j := 0; j < 768; j++ {
		quantized[j] = int8(rand.Intn(255) - 127)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Dequantize(quantized)
	}
}

func BenchmarkDistanceInt8(b *testing.B) {
	a := make([]int8, 768)
	b2 := make([]int8, 768)

	for j := 0; j < 768; j++ {
		a[j] = int8(rand.Intn(255) - 127)
		b2[j] = int8(rand.Intn(255) - 127)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DistanceInt8(a, b2)
	}
}
