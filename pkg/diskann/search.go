package diskann

import (
	"container/heap"
	"fmt"
	"sort"
)

// SearchResult represents a single search result
type SearchResult struct {
	ID       uint64                 // Vector ID
	Distance float32                // Distance to query
	Metadata map[string]interface{} // User-defined metadata
}

// Search performs approximate nearest neighbor search using beam search
// This is the key algorithm of DiskANN:
// 1. Start search in memory-resident graph (fast)
// 2. Beam search on disk-resident graph (parallel I/O)
// 3. Re-rank with full precision vectors
func (idx *Index) Search(query []float32, k int) ([]SearchResult, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if !idx.isBuilt {
		return nil, fmt.Errorf("index not built yet - call Build() first")
	}

	if len(query) != idx.dimension {
		return nil, fmt.Errorf("query dimension mismatch: expected %d, got %d", idx.dimension, len(query))
	}

	if k <= 0 {
		return nil, fmt.Errorf("k must be positive")
	}

	// Phase 1: Search in memory graph to find entry points for disk search
	memoryEntryPoint := idx.memoryGraph.GetEntryPoint()
	memoryCandidates := idx.searchMemoryGraph(query, idx.L, memoryEntryPoint)

	// Phase 2: Beam search on disk graph starting from memory candidates
	diskCandidates := idx.beamSearchDisk(query, memoryCandidates, idx.L*2)

	// Phase 3: Re-rank top candidates
	topCandidates := idx.rerank(query, diskCandidates, k*2)

	// Phase 4: Convert to results
	results := make([]SearchResult, 0, k)
	for i := 0; i < min(k, len(topCandidates)); i++ {
		candidate := topCandidates[i]
		node, exists := idx.nodes[candidate.ID]
		if !exists {
			continue
		}

		results = append(results, SearchResult{
			ID:       candidate.ID,
			Distance: candidate.Distance,
			Metadata: node.Metadata,
		})
	}

	return results, nil
}

// searchMemoryGraph searches the in-memory graph using greedy search
func (idx *Index) searchMemoryGraph(query []float32, L int, entryID uint64) []Candidate {
	visited := make(map[uint64]bool)
	candidates := NewMinHeap()

	// Check if entry point is in memory
	entryNode, exists := idx.memoryGraph.GetNode(entryID)
	if !exists {
		// Fallback to first node in memory
		allNodes := idx.memoryGraph.GetAllNodes()
		if len(allNodes) == 0 {
			return []Candidate{}
		}
		entryID = allNodes[0]
		entryNode, _ = idx.memoryGraph.GetNode(entryID)
	}

	// Calculate distance to entry point
	entryDist := idx.distanceFunc(query, entryNode.Vector)
	heap.Push(candidates, Candidate{ID: entryID, Distance: entryDist})
	visited[entryID] = true

	// Result heap (max heap for top-k)
	results := NewMaxHeap()
	heap.Push(results, Candidate{ID: entryID, Distance: entryDist})

	// Greedy search in memory graph
	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(Candidate)

		// If this distance is worse than our Lth best, we're done
		if results.Len() >= L && current.Distance > (*results)[0].Distance {
			break
		}

		// Get neighbors from memory graph
		node, exists := idx.memoryGraph.GetNode(current.ID)
		if !exists {
			continue
		}

		// Explore neighbors
		for _, neighborID := range node.Neighbors {
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			// Calculate distance
			var dist float32
			if neighborNode, exists := idx.memoryGraph.GetNode(neighborID); exists {
				// Use full precision if in memory
				dist = idx.distanceFunc(query, neighborNode.Vector)
			} else {
				// Skip if not in memory (will be explored in disk phase)
				continue
			}

			// Add to candidates
			heap.Push(candidates, Candidate{ID: neighborID, Distance: dist})

			// Update results
			if results.Len() < L {
				heap.Push(results, Candidate{ID: neighborID, Distance: dist})
			} else if dist < (*results)[0].Distance {
				heap.Pop(results)
				heap.Push(results, Candidate{ID: neighborID, Distance: dist})
			}
		}
	}

	// Convert results to slice
	resultSlice := make([]Candidate, results.Len())
	for i := len(resultSlice) - 1; i >= 0; i-- {
		resultSlice[i] = heap.Pop(results).(Candidate)
	}

	return resultSlice
}

// beamSearchDisk performs beam search on the disk-resident graph
func (idx *Index) beamSearchDisk(query []float32, entryPoints []Candidate, L int) []Candidate {
	visited := make(map[uint64]bool)
	candidates := NewMinHeap()

	// Initialize with entry points
	for _, ep := range entryPoints {
		if !visited[ep.ID] {
			heap.Push(candidates, ep)
			visited[ep.ID] = true
		}
	}

	// Result heap (max heap for top-k)
	results := NewMaxHeap()
	for _, ep := range entryPoints {
		if results.Len() < L {
			heap.Push(results, ep)
		}
	}

	// Beam search parameters
	beamSize := idx.beamWidth
	batch := make([]uint64, 0, beamSize)

	for candidates.Len() > 0 {
		// Pop up to beamWidth candidates for parallel exploration
		batch = batch[:0]
		for i := 0; i < beamSize && candidates.Len() > 0; i++ {
			current := heap.Pop(candidates).(Candidate)

			// If this distance is worse than our Lth best, skip
			if results.Len() >= L && current.Distance > (*results)[0].Distance {
				continue
			}

			batch = append(batch, current.ID)
		}

		if len(batch) == 0 {
			break
		}

		// Batch read nodes from disk (parallel I/O)
		diskNodes, err := idx.diskGraph.BatchReadNodes(batch)
		if err != nil {
			// On error, try individual reads
			for _, nodeID := range batch {
				diskNode, err := idx.diskGraph.ReadNode(nodeID)
				if err != nil {
					continue
				}
				diskNodes = append(diskNodes, diskNode)
			}
		}

		// Process each node's neighbors
		for _, diskNode := range diskNodes {
			if diskNode == nil {
				continue
			}

			// Explore neighbors
			for _, neighborID := range diskNode.Neighbors {
				if visited[neighborID] {
					continue
				}
				visited[neighborID] = true

				// Calculate distance using PQ codes for efficiency
				var dist float32
				if neighborDiskNode, err := idx.diskGraph.ReadNode(neighborID); err == nil {
					dist = idx.computePQDistance(query, neighborDiskNode.PQCode)
				} else {
					continue
				}

				// Add to candidates
				heap.Push(candidates, Candidate{ID: neighborID, Distance: dist})

				// Update results
				if results.Len() < L {
					heap.Push(results, Candidate{ID: neighborID, Distance: dist})
				} else if dist < (*results)[0].Distance {
					heap.Pop(results)
					heap.Push(results, Candidate{ID: neighborID, Distance: dist})
				}
			}
		}
	}

	// Convert results to slice
	resultSlice := make([]Candidate, results.Len())
	for i := len(resultSlice) - 1; i >= 0; i-- {
		resultSlice[i] = heap.Pop(results).(Candidate)
	}

	return resultSlice
}

// rerank re-ranks candidates using full precision vectors
func (idx *Index) rerank(query []float32, candidates []Candidate, k int) []Candidate {
	// Recalculate distances with full precision
	reranked := make([]Candidate, 0, len(candidates))

	for _, candidate := range candidates {
		node, exists := idx.nodes[candidate.ID]
		if !exists {
			continue
		}

		// Use full precision vector for accurate distance
		dist := idx.distanceFunc(query, node.Vector)
		reranked = append(reranked, Candidate{
			ID:       candidate.ID,
			Distance: dist,
		})
	}

	// Sort by distance
	sort.Slice(reranked, func(i, j int) bool {
		return reranked[i].Distance < reranked[j].Distance
	})

	// Return top k
	if len(reranked) > k {
		reranked = reranked[:k]
	}

	return reranked
}

// computePQDistance computes distance using PQ codes
func (idx *Index) computePQDistance(query []float32, pqCode []byte) float32 {
	// Get codebooks from PQ
	codebooks := idx.pqCodebook.GetCodebooks()
	subvectorDim := idx.dimension / len(pqCode)

	return PQDistance(query, pqCode, codebooks, subvectorDim)
}

// MinHeap implements heap.Interface for min-heap of candidates
type MinHeap []Candidate

func NewMinHeap() *MinHeap {
	h := &MinHeap{}
	heap.Init(h)
	return h
}

func (h MinHeap) Len() int           { return len(h) }
func (h MinHeap) Less(i, j int) bool { return h[i].Distance < h[j].Distance }
func (h MinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MinHeap) Push(x interface{}) {
	*h = append(*h, x.(Candidate))
}

func (h *MinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

// MaxHeap implements heap.Interface for max-heap of candidates
type MaxHeap []Candidate

func NewMaxHeap() *MaxHeap {
	h := &MaxHeap{}
	heap.Init(h)
	return h
}

func (h MaxHeap) Len() int           { return len(h) }
func (h MaxHeap) Less(i, j int) bool { return h[i].Distance > h[j].Distance }
func (h MaxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *MaxHeap) Push(x interface{}) {
	*h = append(*h, x.(Candidate))
}

func (h *MaxHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
