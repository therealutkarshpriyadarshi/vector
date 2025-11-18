package diskann

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// Build constructs the DiskANN index from the accumulated vectors
// This is a multi-phase process:
// 1. Train Product Quantization codebook
// 2. Build initial graph using greedy search
// 3. Prune graph to maintain degree constraints
// 4. Select nodes for memory-resident graph
// 5. Write remaining nodes to disk
func (idx *Index) Build() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.isBuilt {
		return fmt.Errorf("index already built")
	}

	if len(idx.buildVectors) == 0 {
		return fmt.Errorf("no vectors to build index")
	}

	numVectors := len(idx.buildVectors)
	fmt.Printf("Building DiskANN index with %d vectors...\n", numVectors)

	// Phase 1: Train Product Quantization
	fmt.Printf("Phase 1: Training product quantization codebook...\n")
	if err := idx.pqCodebook.Train(idx.buildVectors); err != nil {
		return fmt.Errorf("failed to train PQ: %w", err)
	}

	// Phase 2: Encode all vectors with PQ
	fmt.Printf("Phase 2: Encoding vectors with PQ...\n")
	pqCodes := make([][]byte, numVectors)
	for i, vec := range idx.buildVectors {
		code := idx.pqCodebook.Encode(vec)
		pqCodes[i] = code
	}

	// Phase 3: Build graph structure
	fmt.Printf("Phase 3: Building graph structure...\n")
	if err := idx.buildGraph(pqCodes); err != nil {
		return fmt.Errorf("failed to build graph: %w", err)
	}

	// Phase 4: Select nodes for memory graph (medoid-based selection)
	fmt.Printf("Phase 4: Selecting nodes for memory-resident graph...\n")
	if err := idx.selectMemoryNodes(); err != nil {
		return fmt.Errorf("failed to select memory nodes: %w", err)
	}

	// Phase 5: Write remaining nodes to disk
	fmt.Printf("Phase 5: Writing nodes to disk...\n")
	if err := idx.writeToDisk(pqCodes); err != nil {
		return fmt.Errorf("failed to write to disk: %w", err)
	}

	// Mark as built and cleanup build data
	idx.isBuilt = true
	idx.size = int64(numVectors)
	idx.buildVectors = nil // Free memory
	idx.buildIDs = nil
	idx.buildMetadata = nil

	fmt.Printf("DiskANN index built successfully!\n")
	fmt.Printf("  Total nodes: %d\n", numVectors)
	fmt.Printf("  Memory nodes: %d\n", idx.memoryGraph.Size())
	fmt.Printf("  Disk nodes: %d\n", idx.diskGraph.Size())

	return nil
}

// buildGraph builds the graph structure using greedy search
func (idx *Index) buildGraph(pqCodes [][]byte) error {
	numVectors := len(idx.buildVectors)

	// Initialize nodes
	for i := 0; i < numVectors; i++ {
		node := NewNode(idx.buildIDs[i], idx.buildVectors[i], idx.buildMetadata[i])
		node.PQCode = pqCodes[i]
		idx.nodes[idx.buildIDs[i]] = node
	}

	// Find medoid as entry point (approximate center of dataset)
	medoidID := idx.findMedoid()
	idx.memoryGraph.SetEntryPoint(medoidID)

	// Build graph using greedy insertion
	for i := 0; i < numVectors; i++ {
		if i%1000 == 0 {
			fmt.Printf("  Building graph: %d/%d nodes\n", i, numVectors)
		}

		id := idx.buildIDs[i]
		vec := idx.buildVectors[i]

		// Skip medoid (already entry point)
		if id == medoidID {
			continue
		}

		// Find candidate neighbors using greedy search
		candidates := idx.greedySearch(vec, idx.L, medoidID)

		// Select R best neighbors
		neighbors := idx.selectNeighbors(candidates, idx.R)

		// Add bidirectional edges
		idx.nodes[id].SetNeighbors(neighbors)

		// Add reverse edges (with pruning to maintain degree constraint)
		for _, neighborID := range neighbors {
			idx.addReverseEdge(neighborID, id)
		}
	}

	return nil
}

// findMedoid finds the approximate medoid (center) of the dataset
func (idx *Index) findMedoid() uint64 {
	if len(idx.buildVectors) == 0 {
		return 0
	}

	// Sample random points for efficiency
	sampleSize := min(1000, len(idx.buildVectors))
	samples := make([]int, sampleSize)
	for i := 0; i < sampleSize; i++ {
		samples[i] = rand.Intn(len(idx.buildVectors))
	}

	// Find point with minimum average distance to samples
	bestIdx := 0
	bestAvgDist := float32(math.Inf(1))

	for i := 0; i < len(idx.buildVectors); i++ {
		totalDist := float32(0)
		for _, sampleIdx := range samples {
			dist := idx.distanceFunc(idx.buildVectors[i], idx.buildVectors[sampleIdx])
			totalDist += dist
		}
		avgDist := totalDist / float32(sampleSize)

		if avgDist < bestAvgDist {
			bestAvgDist = avgDist
			bestIdx = i
		}
	}

	return idx.buildIDs[bestIdx]
}

// greedySearch performs greedy search to find candidates
func (idx *Index) greedySearch(query []float32, L int, entryID uint64) []Candidate {
	visited := make(map[uint64]bool)
	candidates := make([]Candidate, 0, L)

	// Start from entry point
	entryNode := idx.nodes[entryID]
	entryDist := idx.distanceFunc(query, entryNode.Vector)

	candidates = append(candidates, Candidate{ID: entryID, Distance: entryDist})
	visited[entryID] = true

	// Greedy search
	for len(candidates) < L {
		// Find best unvisited neighbor
		bestDist := float32(math.Inf(1))
		var bestID uint64
		found := false

		for _, candidate := range candidates {
			node := idx.nodes[candidate.ID]
			for _, neighborID := range node.Neighbors {
				if visited[neighborID] {
					continue
				}

				neighborNode := idx.nodes[neighborID]
				dist := idx.distanceFunc(query, neighborNode.Vector)

				if dist < bestDist {
					bestDist = dist
					bestID = neighborID
					found = true
				}
			}
		}

		if !found {
			break
		}

		candidates = append(candidates, Candidate{ID: bestID, Distance: bestDist})
		visited[bestID] = true
	}

	// Sort by distance
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Distance < candidates[j].Distance
	})

	return candidates
}

// selectNeighbors selects the best R neighbors using RNG heuristic
func (idx *Index) selectNeighbors(candidates []Candidate, R int) []uint64 {
	if len(candidates) <= R {
		neighbors := make([]uint64, len(candidates))
		for i, c := range candidates {
			neighbors[i] = c.ID
		}
		return neighbors
	}

	// Use RNG (Relative Neighborhood Graph) heuristic
	// Select diverse neighbors to avoid clustering
	selected := make([]uint64, 0, R)

	for _, candidate := range candidates {
		if len(selected) >= R {
			break
		}

		// Check if this candidate is "useful" (not occluded by existing neighbors)
		useful := true
		candidateVec := idx.nodes[candidate.ID].Vector

		for _, selectedID := range selected {
			selectedVec := idx.nodes[selectedID].Vector

			// If distance to candidate is greater than distance to existing neighbor,
			// the candidate might be occluded
			distToCandidate := idx.distanceFunc(candidateVec, selectedVec)
			if distToCandidate < candidate.Distance * float32(idx.alpha) {
				useful = false
				break
			}
		}

		if useful {
			selected = append(selected, candidate.ID)
		}
	}

	return selected
}

// addReverseEdge adds a reverse edge with degree constraint
func (idx *Index) addReverseEdge(fromID, toID uint64) {
	node := idx.nodes[fromID]

	// Check if already connected
	for _, neighborID := range node.Neighbors {
		if neighborID == toID {
			return
		}
	}

	// Add edge
	node.Neighbors = append(node.Neighbors, toID)

	// Prune if exceeds degree constraint
	if len(node.Neighbors) > idx.R {
		idx.pruneNeighbors(fromID)
	}
}

// pruneNeighbors prunes neighbors to maintain degree constraint
func (idx *Index) pruneNeighbors(nodeID uint64) {
	node := idx.nodes[nodeID]
	if len(node.Neighbors) <= idx.R {
		return
	}

	// Calculate distances to all neighbors
	candidates := make([]Candidate, len(node.Neighbors))
	for i, neighborID := range node.Neighbors {
		neighborVec := idx.nodes[neighborID].Vector
		dist := idx.distanceFunc(node.Vector, neighborVec)
		candidates[i] = Candidate{ID: neighborID, Distance: dist}
	}

	// Select best R neighbors
	pruned := idx.selectNeighbors(candidates, idx.R)
	node.SetNeighbors(pruned)
}

// selectMemoryNodes selects nodes to keep in memory
func (idx *Index) selectMemoryNodes() error {
	// Strategy: Keep nodes closest to medoid (representative sample)
	medoidID := idx.memoryGraph.GetEntryPoint()
	medoidVec := idx.nodes[medoidID].Vector

	// Calculate distance to medoid for all nodes
	type nodeDistance struct {
		id   uint64
		dist float32
	}

	distances := make([]nodeDistance, 0, len(idx.nodes))
	for id, node := range idx.nodes {
		dist := idx.distanceFunc(node.Vector, medoidVec)
		distances = append(distances, nodeDistance{id: id, dist: dist})
	}

	// Sort by distance
	sort.Slice(distances, func(i, j int) bool {
		return distances[i].dist < distances[j].dist
	})

	// Select closest nodes for memory graph
	maxMemoryNodes := idx.memoryGraph.maxSize
	numMemoryNodes := min(maxMemoryNodes, len(distances))

	for i := 0; i < numMemoryNodes; i++ {
		id := distances[i].id
		node := idx.nodes[id]
		idx.memoryGraph.AddNode(id, node.Vector, node.Neighbors)
	}

	return nil
}

// writeToDisk writes nodes to disk storage
func (idx *Index) writeToDisk(pqCodes [][]byte) error {
	for i, id := range idx.buildIDs {
		node := idx.nodes[id]

		// Create disk node
		diskNode := &DiskNode{
			ID:           id,
			Neighbors:    node.Neighbors,
			PQCode:       pqCodes[i],
			VectorOffset: 0, // Not storing full vectors on disk for now
		}

		// Write to disk
		if err := idx.diskGraph.WriteNode(diskNode); err != nil {
			return fmt.Errorf("failed to write node %d: %w", id, err)
		}
	}

	return nil
}

// Candidate represents a search candidate
type Candidate struct {
	ID       uint64
	Distance float32
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
