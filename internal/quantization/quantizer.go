package quantization

// Quantizer defines the common interface for all quantization methods
type Quantizer interface {
	// Train learns quantization parameters from training data
	Train(vectors [][]float32) error

	// Encode compresses a vector into a compact representation
	Encode(vector []float32) []byte

	// Decode decompresses a compact representation back to a vector
	Decode(code []byte) []float32

	// GetCompressionRatio returns the theoretical compression ratio
	GetCompressionRatio(originalDim int) float32
}

// AsymmetricQuantizer extends Quantizer for asymmetric distance computation
// This is key for high-performance approximate nearest neighbor search
type AsymmetricQuantizer interface {
	Quantizer

	// ComputeDistanceTable precomputes distance table for a query vector
	// This enables fast asymmetric distance computation during search
	ComputeDistanceTable(query []float32) interface{}

	// AsymmetricDistance computes distance between query and encoded vector
	// Uses precomputed distance table for efficiency
	AsymmetricDistance(distTable interface{}, code []byte) float32
}

// DistanceMetric defines supported distance metrics
type DistanceMetric int

const (
	// EuclideanDistance is L2 distance
	EuclideanDistance DistanceMetric = iota

	// CosineDistance is 1 - cosine similarity
	CosineDistance

	// DotProductDistance is negative dot product (for maximum inner product search)
	DotProductDistance
)

// QuantizationConfig holds configuration for quantization training
type QuantizationConfig struct {
	// NumIterations for k-means clustering
	NumIterations int

	// DistanceMetric to use
	DistanceMetric DistanceMetric

	// Verbose logging during training
	Verbose bool

	// RandomSeed for reproducible training
	RandomSeed int64
}

// DefaultConfig returns default quantization configuration
func DefaultConfig() *QuantizationConfig {
	return &QuantizationConfig{
		NumIterations:  25,
		DistanceMetric: EuclideanDistance,
		Verbose:        false,
		RandomSeed:     42,
	}
}
