package quantization

import (
	"fmt"
	"math"
)

// ScalarQuantizer performs scalar quantization on float32 vectors
// Compresses float32 (4 bytes) to int8 (1 byte) - 4x memory reduction
type ScalarQuantizer struct {
	min    float32
	max    float32
	scale  float32
	offset float32
}

// NewScalarQuantizer creates a new scalar quantizer
func NewScalarQuantizer() *ScalarQuantizer {
	return &ScalarQuantizer{}
}

// Train computes quantization parameters from training data
func (q *ScalarQuantizer) Train(vectors [][]float32) error {
	if len(vectors) == 0 {
		return fmt.Errorf("no training data provided")
	}

	// Find global min and max across all vectors
	q.min = float32(math.MaxFloat32)
	q.max = float32(-math.MaxFloat32)

	for _, vector := range vectors {
		for _, val := range vector {
			if val < q.min {
				q.min = val
			}
			if val > q.max {
				q.max = val
			}
		}
	}

	// Compute scale and offset for quantization
	// Map [min, max] to [-127, 127]
	valueRange := q.max - q.min
	if valueRange == 0 {
		valueRange = 1.0 // Avoid division by zero
	}

	q.scale = 254.0 / valueRange  // 254 = 127 - (-127)
	q.offset = -127.0 - (q.min * q.scale)

	return nil
}

// Quantize converts a float32 vector to int8
func (q *ScalarQuantizer) Quantize(vector []float32) []int8 {
	quantized := make([]int8, len(vector))

	for i, val := range vector {
		// Scale to [-127, 127] and round
		scaled := val*q.scale + q.offset

		// Clamp to valid range
		if scaled < -127 {
			scaled = -127
		} else if scaled > 127 {
			scaled = 127
		}

		quantized[i] = int8(math.Round(float64(scaled)))
	}

	return quantized
}

// Dequantize converts an int8 vector back to float32
func (q *ScalarQuantizer) Dequantize(quantized []int8) []float32 {
	vector := make([]float32, len(quantized))

	for i, val := range quantized {
		// Reverse the quantization
		vector[i] = (float32(val) - q.offset) / q.scale
	}

	return vector
}

// QuantizeBatch quantizes multiple vectors
func (q *ScalarQuantizer) QuantizeBatch(vectors [][]float32) [][]int8 {
	quantized := make([][]int8, len(vectors))

	for i, vector := range vectors {
		quantized[i] = q.Quantize(vector)
	}

	return quantized
}

// DequantizeBatch dequantizes multiple vectors
func (q *ScalarQuantizer) DequantizeBatch(quantized [][]int8) [][]float32 {
	vectors := make([][]float32, len(quantized))

	for i, qvec := range quantized {
		vectors[i] = q.Dequantize(qvec)
	}

	return vectors
}

// GetMemoryReduction returns the theoretical memory reduction factor
func (q *ScalarQuantizer) GetMemoryReduction() float32 {
	// float32 = 4 bytes, int8 = 1 byte
	return 4.0
}

// GetParameters returns the quantization parameters
func (q *ScalarQuantizer) GetParameters() (min, max, scale, offset float32) {
	return q.min, q.max, q.scale, q.offset
}

// SetParameters sets the quantization parameters (for loading from disk)
func (q *ScalarQuantizer) SetParameters(min, max, scale, offset float32) {
	q.min = min
	q.max = max
	q.scale = scale
	q.offset = offset
}

// DistanceInt8 computes distance between quantized vectors directly (faster)
// This is an approximate Euclidean distance on quantized space
func DistanceInt8(a, b []int8) float32 {
	if len(a) != len(b) {
		return float32(math.MaxFloat32)
	}

	var sum int64
	for i := range a {
		diff := int64(a[i]) - int64(b[i])
		sum += diff * diff
	}

	return float32(math.Sqrt(float64(sum)))
}

// DotProductInt8 computes dot product between quantized vectors
func DotProductInt8(a, b []int8) int64 {
	if len(a) != len(b) {
		return 0
	}

	var sum int64
	for i := range a {
		sum += int64(a[i]) * int64(b[i])
	}

	return sum
}
