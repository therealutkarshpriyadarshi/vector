package diskann

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// DiskGraph represents the disk-resident portion of the DiskANN index
// Uses an SSTable-like structure for efficient random access
type DiskGraph struct {
	dataPath     string           // Path to disk storage
	R            int              // Number of neighbors per node

	// File handles
	nodeFile     *os.File         // Stores node metadata and neighbors
	vectorFile   *os.File         // Stores compressed vectors (PQ codes)

	// Index for fast lookups
	nodeIndex    map[uint64]int64 // Maps node ID to file offset

	// Concurrency control
	mu           sync.RWMutex     // Protects file operations

	// Statistics
	nodeCount    int64            // Number of nodes stored
}

// DiskNode represents a node stored on disk
type DiskNode struct {
	ID           uint64   // Node ID
	Neighbors    []uint64 // Neighbor IDs
	PQCode       []byte   // Product quantization code
	VectorOffset int64    // Offset in vector file (for full vectors if stored)
}

// NewDiskGraph creates a new disk-based graph storage
func NewDiskGraph(dataPath string, R int) (*DiskGraph, error) {
	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create file paths
	nodePath := filepath.Join(dataPath, "nodes.dat")
	vectorPath := filepath.Join(dataPath, "vectors.dat")

	// Open or create node file
	nodeFile, err := os.OpenFile(nodePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open node file: %w", err)
	}

	// Open or create vector file
	vectorFile, err := os.OpenFile(vectorPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		nodeFile.Close()
		return nil, fmt.Errorf("failed to open vector file: %w", err)
	}

	dg := &DiskGraph{
		dataPath:   dataPath,
		R:          R,
		nodeFile:   nodeFile,
		vectorFile: vectorFile,
		nodeIndex:  make(map[uint64]int64),
		nodeCount:  0,
	}

	// Load existing index if files are not empty
	if err := dg.loadIndex(); err != nil {
		nodeFile.Close()
		vectorFile.Close()
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	return dg, nil
}

// WriteNode writes a node to disk
func (dg *DiskGraph) WriteNode(node *DiskNode) error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	// Get current position in file
	offset, err := dg.nodeFile.Seek(0, io.SeekEnd)
	if err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	// Write node ID (8 bytes)
	if err := binary.Write(dg.nodeFile, binary.LittleEndian, node.ID); err != nil {
		return fmt.Errorf("failed to write node ID: %w", err)
	}

	// Write number of neighbors (4 bytes)
	numNeighbors := uint32(len(node.Neighbors))
	if err := binary.Write(dg.nodeFile, binary.LittleEndian, numNeighbors); err != nil {
		return fmt.Errorf("failed to write neighbor count: %w", err)
	}

	// Write neighbors (8 bytes each)
	for _, neighborID := range node.Neighbors {
		if err := binary.Write(dg.nodeFile, binary.LittleEndian, neighborID); err != nil {
			return fmt.Errorf("failed to write neighbor: %w", err)
		}
	}

	// Write PQ code length (4 bytes)
	pqCodeLen := uint32(len(node.PQCode))
	if err := binary.Write(dg.nodeFile, binary.LittleEndian, pqCodeLen); err != nil {
		return fmt.Errorf("failed to write PQ code length: %w", err)
	}

	// Write PQ code
	if len(node.PQCode) > 0 {
		if _, err := dg.nodeFile.Write(node.PQCode); err != nil {
			return fmt.Errorf("failed to write PQ code: %w", err)
		}
	}

	// Write vector offset (8 bytes)
	if err := binary.Write(dg.nodeFile, binary.LittleEndian, node.VectorOffset); err != nil {
		return fmt.Errorf("failed to write vector offset: %w", err)
	}

	// Update index
	dg.nodeIndex[node.ID] = offset
	dg.nodeCount++

	// Flush to disk
	if err := dg.nodeFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync: %w", err)
	}

	return nil
}

// ReadNode reads a node from disk
func (dg *DiskGraph) ReadNode(id uint64) (*DiskNode, error) {
	dg.mu.RLock()
	defer dg.mu.RUnlock()

	// Get offset from index
	offset, exists := dg.nodeIndex[id]
	if !exists {
		return nil, fmt.Errorf("node %d not found", id)
	}

	// Seek to position
	if _, err := dg.nodeFile.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	node := &DiskNode{}

	// Read node ID
	if err := binary.Read(dg.nodeFile, binary.LittleEndian, &node.ID); err != nil {
		return nil, fmt.Errorf("failed to read node ID: %w", err)
	}

	// Read number of neighbors
	var numNeighbors uint32
	if err := binary.Read(dg.nodeFile, binary.LittleEndian, &numNeighbors); err != nil {
		return nil, fmt.Errorf("failed to read neighbor count: %w", err)
	}

	// Read neighbors
	node.Neighbors = make([]uint64, numNeighbors)
	for i := uint32(0); i < numNeighbors; i++ {
		if err := binary.Read(dg.nodeFile, binary.LittleEndian, &node.Neighbors[i]); err != nil {
			return nil, fmt.Errorf("failed to read neighbor: %w", err)
		}
	}

	// Read PQ code length
	var pqCodeLen uint32
	if err := binary.Read(dg.nodeFile, binary.LittleEndian, &pqCodeLen); err != nil {
		return nil, fmt.Errorf("failed to read PQ code length: %w", err)
	}

	// Read PQ code
	if pqCodeLen > 0 {
		node.PQCode = make([]byte, pqCodeLen)
		if _, err := io.ReadFull(dg.nodeFile, node.PQCode); err != nil {
			return nil, fmt.Errorf("failed to read PQ code: %w", err)
		}
	}

	// Read vector offset
	if err := binary.Read(dg.nodeFile, binary.LittleEndian, &node.VectorOffset); err != nil {
		return nil, fmt.Errorf("failed to read vector offset: %w", err)
	}

	return node, nil
}

// BatchReadNodes reads multiple nodes in parallel for efficiency
func (dg *DiskGraph) BatchReadNodes(ids []uint64) ([]*DiskNode, error) {
	nodes := make([]*DiskNode, len(ids))
	errChan := make(chan error, len(ids))

	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(idx int, nodeID uint64) {
			defer wg.Done()
			node, err := dg.ReadNode(nodeID)
			if err != nil {
				errChan <- err
				return
			}
			nodes[idx] = node
		}(i, id)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return nil, err
	}

	return nodes, nil
}

// loadIndex loads the index from disk
func (dg *DiskGraph) loadIndex() error {
	// Get file size
	stat, err := dg.nodeFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if stat.Size() == 0 {
		// Empty file, nothing to load
		return nil
	}

	// Seek to beginning
	if _, err := dg.nodeFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	offset := int64(0)
	for {
		// Record current offset
		currentOffset := offset

		// Read node ID
		var nodeID uint64
		if err := binary.Read(dg.nodeFile, binary.LittleEndian, &nodeID); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read node ID: %w", err)
		}
		offset += 8

		// Read number of neighbors
		var numNeighbors uint32
		if err := binary.Read(dg.nodeFile, binary.LittleEndian, &numNeighbors); err != nil {
			return fmt.Errorf("failed to read neighbor count: %w", err)
		}
		offset += 4

		// Skip neighbors
		offset += int64(numNeighbors) * 8
		if _, err := dg.nodeFile.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek: %w", err)
		}

		// Read PQ code length
		var pqCodeLen uint32
		if err := binary.Read(dg.nodeFile, binary.LittleEndian, &pqCodeLen); err != nil {
			return fmt.Errorf("failed to read PQ code length: %w", err)
		}
		offset += 4

		// Skip PQ code
		offset += int64(pqCodeLen)
		if _, err := dg.nodeFile.Seek(offset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek: %w", err)
		}

		// Read vector offset
		var vectorOffset int64
		if err := binary.Read(dg.nodeFile, binary.LittleEndian, &vectorOffset); err != nil {
			return fmt.Errorf("failed to read vector offset: %w", err)
		}
		offset += 8

		// Update index
		dg.nodeIndex[nodeID] = currentOffset
		dg.nodeCount++
	}

	return nil
}

// Close closes the disk graph files
func (dg *DiskGraph) Close() error {
	dg.mu.Lock()
	defer dg.mu.Unlock()

	var errs []error

	if dg.nodeFile != nil {
		if err := dg.nodeFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if dg.vectorFile != nil {
		if err := dg.vectorFile.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing disk graph: %v", errs)
	}

	return nil
}

// Size returns the number of nodes stored on disk
func (dg *DiskGraph) Size() int64 {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	return dg.nodeCount
}

// Contains checks if a node exists on disk
func (dg *DiskGraph) Contains(id uint64) bool {
	dg.mu.RLock()
	defer dg.mu.RUnlock()
	_, exists := dg.nodeIndex[id]
	return exists
}
