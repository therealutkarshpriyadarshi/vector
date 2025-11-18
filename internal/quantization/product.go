package quantization

import (
	"encoding/binary"
	"fmt"
	"math"
)

// ProductQuantizer performs product quantization for high compression ratios
// Achieves 8-32x compression with minimal recall loss
//
// Product Quantization divides vectors into m subvectors and quantizes each
// independently using k-means clustering. This enables:
// - High compression ratios (32-256x for common configurations)
// - Asymmetric distance computation for fast search
// - Flexible trade-off between compression and accuracy
type ProductQuantizer struct {
	numSubvectors int           // Number of subvectors (m)
	bitsPerCode   int           // Bits per code (typically 6-8)
	codebooks     [][][]float32 // codebooks[subvector][code] = centroid
	subvectorDim  int           // Dimensions per subvector
	config        *QuantizationConfig
}

// NewProductQuantizer creates a new product quantizer
//
// Parameters:
//   - numSubvectors: Number of subvectors (m). Typical values: 8, 16, 32
//     Higher values = better accuracy but slower encoding
//   - bitsPerCode: Bits per code (k). Typical values: 6, 7, 8
//     Higher values = better accuracy but larger codebooks
//
// Example configurations:
//   - 8 subvectors, 8 bits: 8 bytes per vector (96x compression for 768-dim)
//   - 16 subvectors, 6 bits: 16 bytes per vector (192x compression for 768-dim)
//   - 32 subvectors, 8 bits: 32 bytes per vector (96x compression for 768-dim)
func NewProductQuantizer(numSubvectors, bitsPerCode int) *ProductQuantizer {
	return &ProductQuantizer{
		numSubvectors: numSubvectors,
		bitsPerCode:   bitsPerCode,
		codebooks:     make([][][]float32, numSubvectors),
		config:        DefaultConfig(),
	}
}

// NewProductQuantizerWithConfig creates a PQ with custom configuration
func NewProductQuantizerWithConfig(numSubvectors, bitsPerCode int, config *QuantizationConfig) *ProductQuantizer {
	return &ProductQuantizer{
		numSubvectors: numSubvectors,
		bitsPerCode:   bitsPerCode,
		codebooks:     make([][][]float32, numSubvectors),
		config:        config,
	}
}

// Train trains the product quantizer using k-means on subvectors
func (pq *ProductQuantizer) Train(vectors [][]float32) error {
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

	if pq.config.Verbose {
		fmt.Printf("Training Product Quantizer:\n")
		fmt.Printf("  Dimensions: %d\n", dimensions)
		fmt.Printf("  Subvectors: %d (dim=%d each)\n", pq.numSubvectors, pq.subvectorDim)
		fmt.Printf("  Codes per subvector: %d (%d bits)\n", numCodes, pq.bitsPerCode)
		fmt.Printf("  Compression: %.1fx\n", pq.GetCompressionRatio(dimensions))
	}

	// Train a codebook for each subvector using k-means++
	for sv := 0; sv < pq.numSubvectors; sv++ {
		if pq.config.Verbose {
			fmt.Printf("  Training codebook %d/%d...\n", sv+1, pq.numSubvectors)
		}

		startDim := sv * pq.subvectorDim
		endDim := (sv + 1) * pq.subvectorDim

		// Extract subvectors
		subvectors := make([][]float32, len(vectors))
		for i, vec := range vectors {
			subvectors[i] = make([]float32, pq.subvectorDim)
			copy(subvectors[i], vec[startDim:endDim])
		}

		// Run k-means++ clustering
		centroids, err := KMeansPlusPlus(subvectors, numCodes, pq.config)
		if err != nil {
			return fmt.Errorf("k-means failed for subvector %d: %w", sv, err)
		}

		pq.codebooks[sv] = centroids
	}

	if pq.config.Verbose {
		fmt.Printf("Training complete!\n")
	}

	return nil
}

// Encode encodes a vector into product quantization codes
func (pq *ProductQuantizer) Encode(vector []float32) []byte {
	codes := make([]byte, pq.numSubvectors)

	for sv := 0; sv < pq.numSubvectors; sv++ {
		startDim := sv * pq.subvectorDim
		endDim := (sv + 1) * pq.subvectorDim
		subvector := vector[startDim:endDim]

		// Find closest centroid
		minDist := float32(math.MaxFloat32)
		minCode := 0

		for code, centroid := range pq.codebooks[sv] {
			var dist float32
			switch pq.config.DistanceMetric {
			case EuclideanDistance:
				dist = EuclideanDistanceFloat32(subvector, centroid)
			case CosineDistance:
				dist = CosineDistanceFloat32(subvector, centroid)
			case DotProductDistance:
				dist = -DotProductFloat32(subvector, centroid)
			}

			if dist < minDist {
				minDist = dist
				minCode = code
			}
		}

		codes[sv] = byte(minCode)
	}

	return codes
}

// Decode decodes product quantization codes back to a vector
func (pq *ProductQuantizer) Decode(codes []byte) []float32 {
	if len(codes) != pq.numSubvectors {
		return nil
	}

	vector := make([]float32, pq.numSubvectors*pq.subvectorDim)

	for sv := 0; sv < pq.numSubvectors; sv++ {
		code := codes[sv]
		if int(code) >= len(pq.codebooks[sv]) {
			continue // Invalid code
		}

		centroid := pq.codebooks[sv][code]
		startDim := sv * pq.subvectorDim
		copy(vector[startDim:startDim+pq.subvectorDim], centroid)
	}

	return vector
}

// ComputeDistanceTable precomputes distance table for asymmetric distance computation
// This is the key optimization for fast search with Product Quantization
//
// For a query vector, we precompute the distance from the query subvector to
// all centroids in each codebook. Then distance to any encoded vector is just
// a table lookup and summation.
//
// Returns: [][]float32 where distTable[subvector][code] = distance
func (pq *ProductQuantizer) ComputeDistanceTable(query []float32) interface{} {
	distTable := make([][]float32, pq.numSubvectors)

	for sv := 0; sv < pq.numSubvectors; sv++ {
		startDim := sv * pq.subvectorDim
		endDim := (sv + 1) * pq.subvectorDim
		querySubvector := query[startDim:endDim]

		numCodes := len(pq.codebooks[sv])
		distTable[sv] = make([]float32, numCodes)

		// Precompute distance to all centroids
		for code, centroid := range pq.codebooks[sv] {
			var dist float32
			switch pq.config.DistanceMetric {
			case EuclideanDistance:
				// For L2, we compute squared distance for efficiency
				for d := 0; d < pq.subvectorDim; d++ {
					diff := querySubvector[d] - centroid[d]
					dist += diff * diff
				}
			case CosineDistance:
				dist = CosineDistanceFloat32(querySubvector, centroid)
			case DotProductDistance:
				dist = -DotProductFloat32(querySubvector, centroid)
			}

			distTable[sv][code] = dist
		}
	}

	return distTable
}

// AsymmetricDistance computes distance between query and encoded vector
// using precomputed distance table. This is MUCH faster than decoding
// the vector and computing distance in the original space.
//
// Time complexity: O(m) where m = numSubvectors
// vs O(d) for symmetric distance where d = dimensions
//
// This is the key to fast search with Product Quantization!
func (pq *ProductQuantizer) AsymmetricDistance(distTableInterface interface{}, codes []byte) float32 {
	distTable := distTableInterface.([][]float32)

	if len(codes) != pq.numSubvectors {
		return float32(math.MaxFloat32)
	}

	var totalDist float32
	for sv := 0; sv < pq.numSubvectors; sv++ {
		code := codes[sv]
		if int(code) >= len(distTable[sv]) {
			return float32(math.MaxFloat32)
		}
		totalDist += distTable[sv][code]
	}

	// For L2 distance, take square root of sum of squared distances
	if pq.config.DistanceMetric == EuclideanDistance {
		return float32(math.Sqrt(float64(totalDist)))
	}

	return totalDist
}

// SymmetricDistance computes distance between two encoded vectors
// This is slower than asymmetric distance but useful for some applications
func (pq *ProductQuantizer) SymmetricDistance(codes1, codes2 []byte) float32 {
	if len(codes1) != pq.numSubvectors || len(codes2) != pq.numSubvectors {
		return float32(math.MaxFloat32)
	}

	var totalDist float32
	for sv := 0; sv < pq.numSubvectors; sv++ {
		code1 := codes1[sv]
		code2 := codes2[sv]

		if int(code1) >= len(pq.codebooks[sv]) || int(code2) >= len(pq.codebooks[sv]) {
			return float32(math.MaxFloat32)
		}

		centroid1 := pq.codebooks[sv][code1]
		centroid2 := pq.codebooks[sv][code2]

		var dist float32
		switch pq.config.DistanceMetric {
		case EuclideanDistance:
			dist = EuclideanDistanceFloat32(centroid1, centroid2)
			dist = dist * dist // Square for summation
		case CosineDistance:
			dist = CosineDistanceFloat32(centroid1, centroid2)
		case DotProductDistance:
			dist = -DotProductFloat32(centroid1, centroid2)
		}

		totalDist += dist
	}

	if pq.config.DistanceMetric == EuclideanDistance {
		return float32(math.Sqrt(float64(totalDist)))
	}

	return totalDist
}

// GetCompressionRatio returns the compression ratio
func (pq *ProductQuantizer) GetCompressionRatio(originalDim int) float32 {
	// Original: originalDim * 4 bytes (float32)
	// Compressed: numSubvectors * 1 byte (uint8 code)
	originalBytes := float32(originalDim * 4)
	compressedBytes := float32(pq.numSubvectors)
	return originalBytes / compressedBytes
}

// GetMemoryUsage returns memory usage statistics
func (pq *ProductQuantizer) GetMemoryUsage() (codebookBytes, perVectorBytes int) {
	// Codebook memory
	numCodes := 1 << pq.bitsPerCode
	codebookBytes = pq.numSubvectors * numCodes * pq.subvectorDim * 4 // float32 = 4 bytes

	// Per-vector memory
	perVectorBytes = pq.numSubvectors // 1 byte per subvector

	return codebookBytes, perVectorBytes
}

// Serialize serializes the quantizer for saving to disk
func (pq *ProductQuantizer) Serialize() ([]byte, error) {
	// Format: [numSubvectors(4)] [bitsPerCode(4)] [subvectorDim(4)] [codebooks...]
	numCodes := 1 << pq.bitsPerCode

	// Calculate total size
	headerSize := 12 // 3 * int32
	codebookSize := pq.numSubvectors * numCodes * pq.subvectorDim * 4
	totalSize := headerSize + codebookSize

	data := make([]byte, totalSize)
	offset := 0

	// Write header
	binary.LittleEndian.PutUint32(data[offset:], uint32(pq.numSubvectors))
	offset += 4
	binary.LittleEndian.PutUint32(data[offset:], uint32(pq.bitsPerCode))
	offset += 4
	binary.LittleEndian.PutUint32(data[offset:], uint32(pq.subvectorDim))
	offset += 4

	// Write codebooks
	for sv := 0; sv < pq.numSubvectors; sv++ {
		for code := 0; code < numCodes; code++ {
			for d := 0; d < pq.subvectorDim; d++ {
				bits := math.Float32bits(pq.codebooks[sv][code][d])
				binary.LittleEndian.PutUint32(data[offset:], bits)
				offset += 4
			}
		}
	}

	return data, nil
}

// Deserialize deserializes a quantizer from disk
func (pq *ProductQuantizer) Deserialize(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("data too short")
	}

	offset := 0

	// Read header
	pq.numSubvectors = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	pq.bitsPerCode = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4
	pq.subvectorDim = int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	numCodes := 1 << pq.bitsPerCode

	// Initialize codebooks
	pq.codebooks = make([][][]float32, pq.numSubvectors)
	for sv := 0; sv < pq.numSubvectors; sv++ {
		pq.codebooks[sv] = make([][]float32, numCodes)
		for code := 0; code < numCodes; code++ {
			pq.codebooks[sv][code] = make([]float32, pq.subvectorDim)
			for d := 0; d < pq.subvectorDim; d++ {
				if offset+4 > len(data) {
					return fmt.Errorf("unexpected end of data")
				}
				bits := binary.LittleEndian.Uint32(data[offset:])
				pq.codebooks[sv][code][d] = math.Float32frombits(bits)
				offset += 4
			}
		}
	}

	return nil
}

// GetConfig returns the configuration
func (pq *ProductQuantizer) GetConfig() *QuantizationConfig {
	return pq.config
}

// SetConfig sets the configuration
func (pq *ProductQuantizer) SetConfig(config *QuantizationConfig) {
	pq.config = config
}

// GetNumSubvectors returns the number of subvectors
func (pq *ProductQuantizer) GetNumSubvectors() int {
	return pq.numSubvectors
}

// GetSubvectorDim returns the subvector dimension
func (pq *ProductQuantizer) GetSubvectorDim() int {
	return pq.subvectorDim
}

// GetBitsPerCode returns bits per code
func (pq *ProductQuantizer) GetBitsPerCode() int {
	return pq.bitsPerCode
}

// GetCodebooks returns the codebooks (for DiskANN integration)
func (pq *ProductQuantizer) GetCodebooks() [][][]float32 {
	return pq.codebooks
}
