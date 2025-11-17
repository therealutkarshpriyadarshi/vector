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

// ProductQuantizer performs product quantization (more advanced)
// Divides vector into subvectors and quantizes each separately
type ProductQuantizer struct {
	numSubvectors int
	bitsPerCode   int
	codebooks     [][][]float32 // codebooks[subvector][code] = centroid
	subvectorDim  int
}

// NewProductQuantizer creates a new product quantizer
func NewProductQuantizer(numSubvectors, bitsPerCode int) *ProductQuantizer {
	return &ProductQuantizer{
		numSubvectors: numSubvectors,
		bitsPerCode:   bitsPerCode,
		codebooks:     make([][][]float32, numSubvectors),
	}
}

// Train trains the product quantizer using k-means on subvectors
func (pq *ProductQuantizer) Train(vectors [][]float32, iterations int) error {
	if len(vectors) == 0 {
		return fmt.Errorf("no training data provided")
	}

	dimensions := len(vectors[0])
	if dimensions%pq.numSubvectors != 0 {
		return fmt.Errorf("dimensions (%d) must be divisible by numSubvectors (%d)",
			dimensions, pq.numSubvectors)
	}

	pq.subvectorDim = dimensions / pq.numSubvectors
	numCodes := 1 << pq.bitsPerCode // 2^bitsPerCode

	// Train a codebook for each subvector
	for sv := 0; sv < pq.numSubvectors; sv++ {
		startDim := sv * pq.subvectorDim
		endDim := (sv + 1) * pq.subvectorDim

		// Extract subvectors
		subvectors := make([][]float32, len(vectors))
		for i, vec := range vectors {
			subvectors[i] = vec[startDim:endDim]
		}

		// Run k-means clustering
		centroids, err := kMeans(subvectors, numCodes, iterations)
		if err != nil {
			return fmt.Errorf("k-means failed for subvector %d: %w", sv, err)
		}

		pq.codebooks[sv] = centroids
	}

	return nil
}

// Encode encodes a vector into product quantization codes
func (pq *ProductQuantizer) Encode(vector []float32) []uint8 {
	codes := make([]uint8, pq.numSubvectors)

	for sv := 0; sv < pq.numSubvectors; sv++ {
		startDim := sv * pq.subvectorDim
		endDim := (sv + 1) * pq.subvectorDim
		subvector := vector[startDim:endDim]

		// Find closest centroid
		minDist := float32(math.MaxFloat32)
		minCode := 0

		for code, centroid := range pq.codebooks[sv] {
			dist := euclideanDistance(subvector, centroid)
			if dist < minDist {
				minDist = dist
				minCode = code
			}
		}

		codes[sv] = uint8(minCode)
	}

	return codes
}

// Decode decodes product quantization codes back to a vector
func (pq *ProductQuantizer) Decode(codes []uint8) []float32 {
	vector := make([]float32, pq.numSubvectors*pq.subvectorDim)

	for sv := 0; sv < pq.numSubvectors; sv++ {
		code := codes[sv]
		centroid := pq.codebooks[sv][code]

		startDim := sv * pq.subvectorDim
		copy(vector[startDim:startDim+pq.subvectorDim], centroid)
	}

	return vector
}

// GetCompressionRatio returns the compression ratio
func (pq *ProductQuantizer) GetCompressionRatio(originalDim int) float32 {
	// Original: originalDim * 4 bytes (float32)
	// Compressed: numSubvectors * 1 byte (uint8 code)
	originalBytes := float32(originalDim * 4)
	compressedBytes := float32(pq.numSubvectors)
	return originalBytes / compressedBytes
}

// Helper functions

func euclideanDistance(a, b []float32) float32 {
	var sum float32
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return float32(math.Sqrt(float64(sum)))
}

// Simple k-means implementation for product quantization
func kMeans(vectors [][]float32, k, iterations int) ([][]float32, error) {
	if len(vectors) < k {
		return nil, fmt.Errorf("not enough vectors (%d) for %d clusters", len(vectors), k)
	}

	dim := len(vectors[0])
	centroids := make([][]float32, k)

	// Initialize centroids randomly (using first k vectors)
	for i := 0; i < k; i++ {
		centroids[i] = make([]float32, dim)
		copy(centroids[i], vectors[i%len(vectors)])
	}

	// Iterate
	for iter := 0; iter < iterations; iter++ {
		// Assign vectors to clusters
		clusters := make([][][]float32, k)
		for _, vec := range vectors {
			minDist := float32(math.MaxFloat32)
			minCluster := 0

			for c, centroid := range centroids {
				dist := euclideanDistance(vec, centroid)
				if dist < minDist {
					minDist = dist
					minCluster = c
				}
			}

			clusters[minCluster] = append(clusters[minCluster], vec)
		}

		// Update centroids
		for c := range centroids {
			if len(clusters[c]) == 0 {
				continue // Keep old centroid if cluster is empty
			}

			// Compute mean
			for d := 0; d < dim; d++ {
				var sum float32
				for _, vec := range clusters[c] {
					sum += vec[d]
				}
				centroids[c][d] = sum / float32(len(clusters[c]))
			}
		}
	}

	return centroids, nil
}
