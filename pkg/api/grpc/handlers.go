package grpc

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"time"

	"github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
	"github.com/therealutkarshpriyadarshi/vector/pkg/hnsw"
	"github.com/therealutkarshpriyadarshi/vector/pkg/search"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Insert implements the Insert RPC
func (s *Server) Insert(ctx context.Context, req *proto.InsertRequest) (*proto.InsertResponse, error) {
	start := time.Now()

	// Validate request
	if err := validateInsertRequest(req); err != nil {
		return &proto.InsertResponse{
			Success: false,
			Error:   stringPtr(err.Error()),
		}, status.Error(codes.InvalidArgument, err.Error())
	}

	// Get indexes for namespace
	index, textIndex, _, err := s.getNamespaceIndexes(req.Namespace)
	if err != nil {
		return &proto.InsertResponse{
			Success: false,
			Error:   stringPtr(err.Error()),
		}, status.Error(codes.Internal, err.Error())
	}

	// Convert to float32 vector
	vector := make([]float32, len(req.Vector))
	for i, v := range req.Vector {
		vector[i] = v
	}

	// Insert into HNSW index
	id, err := index.Insert(vector)
	if err != nil {
		return &proto.InsertResponse{
			Success: false,
			Error:   stringPtr(err.Error()),
		}, status.Error(codes.Internal, err.Error())
	}

	// Store metadata
	s.mu.Lock()
	metadataStore := s.metadata[req.Namespace]
	if metadataStore == nil {
		metadataStore = make(map[uint64]map[string]interface{})
		s.metadata[req.Namespace] = metadataStore
	}
	metaMap := make(map[string]interface{})
	for k, v := range req.Metadata {
		metaMap[k] = v
	}
	metadataStore[id] = metaMap
	s.mu.Unlock()

	// Insert into text index if text is provided
	if req.Text != nil && *req.Text != "" {
		doc := &search.Document{
			ID:       id,
			Text:     *req.Text,
			Metadata: metaMap,
		}

		if err := textIndex.Index(doc); err != nil {
			log.Printf("Warning: failed to index text for vector %d: %v", id, err)
		}
	}

	log.Printf("Inserted vector %d in namespace %s (took %v)", id, req.Namespace, time.Since(start))

	return &proto.InsertResponse{
		Id:      strconv.FormatUint(id, 10),
		Success: true,
	}, nil
}

// Search implements the Search RPC
func (s *Server) Search(ctx context.Context, req *proto.SearchRequest) (*proto.SearchResponse, error) {
	start := time.Now()

	// Validate request
	if err := validateSearchRequest(req); err != nil {
		return &proto.SearchResponse{
			Error: stringPtr(err.Error()),
		}, status.Error(codes.InvalidArgument, err.Error())
	}

	// Get indexes for namespace
	index, _, _, err := s.getNamespaceIndexes(req.Namespace)
	if err != nil {
		return &proto.SearchResponse{
			Error: stringPtr(err.Error()),
		}, status.Error(codes.Internal, err.Error())
	}

	// Convert to float32 vector
	queryVector := make([]float32, len(req.QueryVector))
	for i, v := range req.QueryVector {
		queryVector[i] = v
	}

	// Use efSearch from request or default
	efSearch := int(req.EfSearch)
	if efSearch == 0 {
		efSearch = s.config.HNSW.DefaultEfSearch
	}

	// Perform search
	searchResult, err := index.Search(queryVector, int(req.K), efSearch)
	if err != nil {
		return &proto.SearchResponse{
			Error: stringPtr(err.Error()),
		}, status.Error(codes.Internal, err.Error())
	}

	results := searchResult.Results

	// Convert filter if provided and apply
	if req.Filter != nil {
		filter, err := protoFilterToFilter(req.Filter)
		if err != nil {
			return &proto.SearchResponse{
				Error: stringPtr(err.Error()),
			}, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid filter: %v", err))
		}

		// Apply filter
		results = s.applyFilterToResults(req.Namespace, results, filter)
	}

	// Convert results to proto
	protoResults := make([]*proto.SearchResult, 0, len(results))
	for _, r := range results {
		protoResults = append(protoResults, s.resultToProto(req.Namespace, r))
	}

	searchTime := time.Since(start)
	log.Printf("Search in namespace %s returned %d results (took %v)", req.Namespace, len(protoResults), searchTime)

	return &proto.SearchResponse{
		Results:      protoResults,
		TotalResults: int32(len(protoResults)),
		SearchTimeMs: float32(searchTime.Milliseconds()),
	}, nil
}

// HybridSearch implements the HybridSearch RPC
func (s *Server) HybridSearch(ctx context.Context, req *proto.HybridSearchRequest) (*proto.SearchResponse, error) {
	start := time.Now()

	// Validate request
	if err := validateHybridSearchRequest(req); err != nil {
		return &proto.SearchResponse{
			Error: stringPtr(err.Error()),
		}, status.Error(codes.InvalidArgument, err.Error())
	}

	// Get indexes for namespace
	_, _, hybridSearch, err := s.getNamespaceIndexes(req.Namespace)
	if err != nil {
		return &proto.SearchResponse{
			Error: stringPtr(err.Error()),
		}, status.Error(codes.Internal, err.Error())
	}

	// Convert to float32 vector
	queryVector := make([]float32, len(req.QueryVector))
	for i, v := range req.QueryVector {
		queryVector[i] = v
	}

	// Use efSearch from request or default
	efSearch := int(req.EfSearch)
	if efSearch == 0 {
		efSearch = s.config.HNSW.DefaultEfSearch
	}

	// Perform hybrid search
	results := hybridSearch.Search(queryVector, req.QueryText, int(req.K), efSearch)

	// Apply filter if provided
	if req.Filter != nil {
		filter, err := protoFilterToFilter(req.Filter)
		if err != nil {
			return &proto.SearchResponse{
				Error: stringPtr(err.Error()),
			}, status.Error(codes.InvalidArgument, fmt.Sprintf("invalid filter: %v", err))
		}
		results = applyFilterToHybridResults(results, filter)
	}

	// Convert results to proto
	protoResults := make([]*proto.SearchResult, 0, len(results))
	for _, r := range results {
		protoResults = append(protoResults, s.hybridResultToProto(req.Namespace, r))
	}

	searchTime := time.Since(start)
	log.Printf("Hybrid search in namespace %s returned %d results (took %v)", req.Namespace, len(protoResults), searchTime)

	return &proto.SearchResponse{
		Results:      protoResults,
		TotalResults: int32(len(protoResults)),
		SearchTimeMs: float32(searchTime.Milliseconds()),
	}, nil
}

// Delete implements the Delete RPC
func (s *Server) Delete(ctx context.Context, req *proto.DeleteRequest) (*proto.DeleteResponse, error) {
	// Validate request
	if req.Namespace == "" {
		return &proto.DeleteResponse{
			Success: false,
			Error:   stringPtr("namespace is required"),
		}, status.Error(codes.InvalidArgument, "namespace is required")
	}

	// Get indexes for namespace
	index, textIndex, _, err := s.getNamespaceIndexes(req.Namespace)
	if err != nil {
		return &proto.DeleteResponse{
			Success: false,
			Error:   stringPtr(err.Error()),
		}, status.Error(codes.Internal, err.Error())
	}

	var deletedCount int32

	// Handle deletion by ID or filter
	switch selector := req.Selector.(type) {
	case *proto.DeleteRequest_Id:
		// Delete by ID
		id, err := strconv.ParseUint(selector.Id, 10, 64)
		if err != nil {
			return &proto.DeleteResponse{
				Success: false,
				Error:   stringPtr("invalid ID format"),
			}, status.Error(codes.InvalidArgument, "invalid ID format")
		}

		if err := index.Delete(id); err != nil {
			return &proto.DeleteResponse{
				Success: false,
				Error:   stringPtr(err.Error()),
			}, status.Error(codes.Internal, err.Error())
		}

		// Delete from text index
		textIndex.Remove(id)

		// Delete metadata
		s.mu.Lock()
		if metadataStore, ok := s.metadata[req.Namespace]; ok {
			delete(metadataStore, id)
		}
		s.mu.Unlock()

		deletedCount = 1

	case *proto.DeleteRequest_Filter:
		// Delete by filter not implemented yet
		return &proto.DeleteResponse{
			Success: false,
			Error:   stringPtr("delete by filter not yet implemented"),
		}, status.Error(codes.Unimplemented, "delete by filter not yet implemented")

	default:
		return &proto.DeleteResponse{
			Success: false,
			Error:   stringPtr("either id or filter must be specified"),
		}, status.Error(codes.InvalidArgument, "either id or filter must be specified")
	}

	log.Printf("Deleted %d vectors in namespace %s", deletedCount, req.Namespace)

	return &proto.DeleteResponse{
		DeletedCount: deletedCount,
		Success:      true,
	}, nil
}

// Update implements the Update RPC
func (s *Server) Update(ctx context.Context, req *proto.UpdateRequest) (*proto.UpdateResponse, error) {
	// Validate request
	if req.Namespace == "" || req.Id == "" {
		return &proto.UpdateResponse{
			Success: false,
			Error:   stringPtr("namespace and id are required"),
		}, status.Error(codes.InvalidArgument, "namespace and id are required")
	}

	// Get indexes for namespace
	index, textIndex, _, err := s.getNamespaceIndexes(req.Namespace)
	if err != nil {
		return &proto.UpdateResponse{
			Success: false,
			Error:   stringPtr(err.Error()),
		}, status.Error(codes.Internal, err.Error())
	}

	id, err := strconv.ParseUint(req.Id, 10, 64)
	if err != nil {
		return &proto.UpdateResponse{
			Success: false,
			Error:   stringPtr("invalid ID format"),
		}, status.Error(codes.InvalidArgument, "invalid ID format")
	}

	// Update vector if provided
	if len(req.Vector) > 0 {
		vector := make([]float32, len(req.Vector))
		for i, v := range req.Vector {
			vector[i] = v
		}

		if err := index.Update(id, vector); err != nil {
			return &proto.UpdateResponse{
				Success: false,
				Error:   stringPtr(err.Error()),
			}, status.Error(codes.Internal, err.Error())
		}
	}

	// Update metadata if provided
	if len(req.Metadata) > 0 {
		s.mu.Lock()
		if metadataStore, ok := s.metadata[req.Namespace]; ok {
			metaMap := make(map[string]interface{})
			for k, v := range req.Metadata {
				metaMap[k] = v
			}
			metadataStore[id] = metaMap
		}
		s.mu.Unlock()
	}

	// Update text index if text provided
	if req.Text != nil && *req.Text != "" {
		// Remove old document
		textIndex.Remove(id)

		// Get updated metadata
		s.mu.RLock()
		var metadata map[string]interface{}
		if metadataStore, ok := s.metadata[req.Namespace]; ok {
			if meta, ok := metadataStore[id]; ok {
				metadata = meta
			}
		}
		s.mu.RUnlock()

		// Index new document
		doc := &search.Document{
			ID:       id,
			Text:     *req.Text,
			Metadata: metadata,
		}

		if err := textIndex.Index(doc); err != nil {
			log.Printf("Warning: failed to update text for vector %s: %v", req.Id, err)
		}
	}

	log.Printf("Updated vector %s in namespace %s", req.Id, req.Namespace)

	return &proto.UpdateResponse{
		Success: true,
	}, nil
}

// BatchInsert implements the BatchInsert streaming RPC
func (s *Server) BatchInsert(stream proto.VectorDB_BatchInsertServer) error {
	start := time.Now()
	var insertedCount, failedCount int32
	var insertedIDs []string
	var errors []string

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// End of stream
			break
		}
		if err != nil {
			return status.Error(codes.Internal, fmt.Sprintf("stream error: %v", err))
		}

		// Insert each vector
		resp, err := s.Insert(stream.Context(), req)
		if err != nil || !resp.Success {
			failedCount++
			errMsg := "unknown error"
			if resp.Error != nil {
				errMsg = *resp.Error
			} else if err != nil {
				errMsg = err.Error()
			}
			errors = append(errors, errMsg)
		} else {
			insertedCount++
			insertedIDs = append(insertedIDs, resp.Id)
		}
	}

	totalTime := time.Since(start)
	log.Printf("Batch insert completed: %d succeeded, %d failed (took %v)",
		insertedCount, failedCount, totalTime)

	return stream.SendAndClose(&proto.BatchInsertResponse{
		InsertedCount: insertedCount,
		FailedCount:   failedCount,
		InsertedIds:   insertedIDs,
		Errors:        errors,
		TotalTimeMs:   float32(totalTime.Milliseconds()),
	})
}

// GetStats implements the GetStats RPC
func (s *Server) GetStats(ctx context.Context, req *proto.StatsRequest) (*proto.StatsResponse, error) {
	stats := s.Stats()

	resp := &proto.StatsResponse{
		TotalVectors:     0,
		TotalNamespaces:  int64(stats["namespaces"].(int)),
		MemoryUsageBytes: 0, // TODO: implement memory tracking
		NamespaceStats:   make(map[string]*proto.NamespaceStats),
	}

	// Collect namespace stats
	nsStats := stats["namespace_stats"].(map[string]map[string]interface{})
	for ns, nsStat := range nsStats {
		vectorCount := int64(nsStat["vector_count"].(int))
		resp.TotalVectors += vectorCount

		dimensions := s.config.HNSW.Dimensions
		if dimensions == 0 {
			dimensions = 768 // default
		}

		resp.NamespaceStats[ns] = &proto.NamespaceStats{
			VectorCount: vectorCount,
			MemoryBytes: 0, // TODO: implement memory tracking
			Dimensions:  int32(dimensions),
		}
	}

	return resp, nil
}

// HealthCheck implements the HealthCheck RPC
func (s *Server) HealthCheck(ctx context.Context, req *proto.HealthCheckRequest) (*proto.HealthCheckResponse, error) {
	status := "healthy"
	details := make(map[string]string)

	// Check if server is shutting down
	s.shutdownMu.Lock()
	isShutdown := s.isShutdown
	s.shutdownMu.Unlock()

	if isShutdown {
		status = "unhealthy"
		details["reason"] = "server is shutting down"
	}

	// Add namespace count
	s.mu.RLock()
	namespaceCount := len(s.indexes)
	s.mu.RUnlock()

	details["namespaces"] = strconv.Itoa(namespaceCount)
	details["cache_enabled"] = strconv.FormatBool(s.config.Cache.Enabled)

	return &proto.HealthCheckResponse{
		Status:        status,
		Version:       "1.0.0", // TODO: read from build info
		UptimeSeconds: int64(s.Uptime().Seconds()),
		Details:       details,
	}, nil
}

// Helper methods

func (s *Server) applyFilterToResults(namespace string, results []hnsw.Result, filter search.Filter) []hnsw.Result {
	if filter == nil {
		return results
	}

	filtered := make([]hnsw.Result, 0, len(results))
	s.mu.RLock()
	metadataStore := s.metadata[namespace]
	s.mu.RUnlock()

	for _, r := range results {
		if metadata, ok := metadataStore[r.ID]; ok {
			if filter.Match(metadata) {
				filtered = append(filtered, r)
			}
		}
	}
	return filtered
}

func applyFilterToHybridResults(results []*search.HybridSearchResult, filter search.Filter) []*search.HybridSearchResult {
	if filter == nil {
		return results
	}

	filtered := make([]*search.HybridSearchResult, 0, len(results))
	for _, r := range results {
		if filter.Match(r.Metadata) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func (s *Server) resultToProto(namespace string, r hnsw.Result) *proto.SearchResult {
	// Get metadata
	s.mu.RLock()
	var metadata map[string]interface{}
	if metadataStore, ok := s.metadata[namespace]; ok {
		if meta, ok := metadataStore[r.ID]; ok {
			metadata = meta
		}
	}
	s.mu.RUnlock()

	// Convert metadata to string map
	metadataProto := make(map[string]string)
	for k, v := range metadata {
		metadataProto[k] = fmt.Sprintf("%v", v)
	}

	// Get vector from index
	s.mu.RLock()
	index := s.indexes[namespace]
	s.mu.RUnlock()

	var vector []float32
	if index != nil {
		if node := index.GetNode(r.ID); node != nil {
			vector = node.Vector()
		}
	}

	return &proto.SearchResult{
		Id:       strconv.FormatUint(r.ID, 10),
		Distance: r.Distance,
		Vector:   vector,
		Metadata: metadataProto,
	}
}

func (s *Server) hybridResultToProto(namespace string, r *search.HybridSearchResult) *proto.SearchResult {
	// Convert metadata to string map
	metadataProto := make(map[string]string)
	for k, v := range r.Metadata {
		metadataProto[k] = fmt.Sprintf("%v", v)
	}

	// Get vector from index
	s.mu.RLock()
	index := s.indexes[namespace]
	s.mu.RUnlock()

	var vector []float32
	if index != nil {
		if node := index.GetNode(r.ID); node != nil {
			vector = node.Vector()
		}
	}

	// Get text from text index
	s.mu.RLock()
	textIndex := s.textIndexes[namespace]
	s.mu.RUnlock()

	var text *string
	if textIndex != nil {
		if doc := textIndex.GetDocument(r.ID); doc != nil {
			text = &doc.Text
		}
	}

	return &proto.SearchResult{
		Id:          strconv.FormatUint(r.ID, 10),
		Distance:    r.VectorScore,
		Vector:      vector,
		Metadata:    metadataProto,
		Text:        text,
		VectorScore: &r.VectorScore,
		TextScore:   floatPtr(float32(r.TextScore)),
	}
}

// Validation helpers

func validateInsertRequest(req *proto.InsertRequest) error {
	if req.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if len(req.Vector) == 0 {
		return fmt.Errorf("vector is required")
	}
	return nil
}

func validateSearchRequest(req *proto.SearchRequest) error {
	if req.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if len(req.QueryVector) == 0 {
		return fmt.Errorf("query vector is required")
	}
	if req.K <= 0 {
		return fmt.Errorf("k must be > 0")
	}
	return nil
}

func validateHybridSearchRequest(req *proto.HybridSearchRequest) error {
	if req.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if len(req.QueryVector) == 0 {
		return fmt.Errorf("query vector is required")
	}
	if req.QueryText == "" {
		return fmt.Errorf("query text is required")
	}
	if req.K <= 0 {
		return fmt.Errorf("k must be > 0")
	}
	return nil
}

// Filter conversion helpers

func protoFilterToFilter(pf *proto.Filter) (search.Filter, error) {
	switch ft := pf.FilterType.(type) {
	case *proto.Filter_Comparison:
		return protoComparisonToFilter(ft.Comparison)
	case *proto.Filter_Range:
		return protoRangeToFilter(ft.Range)
	case *proto.Filter_List:
		return protoListToFilter(ft.List)
	case *proto.Filter_GeoRadius:
		return protoGeoRadiusToFilter(ft.GeoRadius)
	case *proto.Filter_Exists:
		return protoExistsToFilter(ft.Exists)
	case *proto.Filter_Composite:
		return protoCompositeToFilter(ft.Composite)
	default:
		return nil, fmt.Errorf("unknown filter type")
	}
}

func protoComparisonToFilter(cf *proto.ComparisonFilter) (search.Filter, error) {
	var value interface{} = cf.Value

	// Try to parse as number
	if f, err := strconv.ParseFloat(cf.Value, 64); err == nil {
		value = f
	}

	switch cf.Operator {
	case "eq":
		return search.Eq(cf.Field, value), nil
	case "ne":
		return search.Ne(cf.Field, value), nil
	case "gt":
		return search.Gt(cf.Field, value), nil
	case "lt":
		return search.Lt(cf.Field, value), nil
	case "gte":
		return search.Gte(cf.Field, value), nil
	case "lte":
		return search.Lte(cf.Field, value), nil
	default:
		return nil, fmt.Errorf("unknown comparison operator: %s", cf.Operator)
	}
}

func protoRangeToFilter(rf *proto.RangeFilter) (search.Filter, error) {
	// Not directly supported, convert to composite AND filter
	var filters []search.Filter

	if rf.Gte != nil {
		if f, err := strconv.ParseFloat(*rf.Gte, 64); err == nil {
			filters = append(filters, search.Gte(rf.Field, f))
		}
	}
	if rf.Lte != nil {
		if f, err := strconv.ParseFloat(*rf.Lte, 64); err == nil {
			filters = append(filters, search.Lte(rf.Field, f))
		}
	}
	if rf.Gt != nil {
		if f, err := strconv.ParseFloat(*rf.Gt, 64); err == nil {
			filters = append(filters, search.Gt(rf.Field, f))
		}
	}
	if rf.Lt != nil {
		if f, err := strconv.ParseFloat(*rf.Lt, 64); err == nil {
			filters = append(filters, search.Lt(rf.Field, f))
		}
	}

	if len(filters) == 0 {
		return nil, fmt.Errorf("range filter has no conditions")
	}
	if len(filters) == 1 {
		return filters[0], nil
	}

	return search.And(filters...), nil
}

func protoListToFilter(lf *proto.ListFilter) (search.Filter, error) {
	values := make([]interface{}, len(lf.Values))
	for i, v := range lf.Values {
		values[i] = v
	}

	switch lf.Operator {
	case "in":
		return search.In(lf.Field, values), nil
	case "not_in":
		return search.NotIn(lf.Field, values), nil
	default:
		return nil, fmt.Errorf("unknown list operator: %s", lf.Operator)
	}
}

func protoGeoRadiusToFilter(gf *proto.GeoRadiusFilter) (search.Filter, error) {
	return search.GeoRadius(gf.Field, gf.Latitude, gf.Longitude, gf.RadiusKm), nil
}

func protoExistsToFilter(ef *proto.ExistsFilter) (search.Filter, error) {
	return search.Exists(ef.Field), nil
}

func protoCompositeToFilter(cf *proto.CompositeFilter) (search.Filter, error) {
	filters := make([]search.Filter, len(cf.Filters))
	for i, pf := range cf.Filters {
		f, err := protoFilterToFilter(pf)
		if err != nil {
			return nil, err
		}
		filters[i] = f
	}

	switch cf.Operator {
	case "and":
		return search.And(filters...), nil
	case "or":
		return search.Or(filters...), nil
	case "not":
		if len(filters) != 1 {
			return nil, fmt.Errorf("NOT filter requires exactly one sub-filter")
		}
		return search.Not(filters[0]), nil
	default:
		return nil, fmt.Errorf("unknown composite operator: %s", cf.Operator)
	}
}

// Utility helpers

func stringPtr(s string) *string {
	return &s
}

func floatPtr(f float32) *float32 {
	return &f
}
