package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	pb "github.com/therealutkarshpriyadarshi/vector/pkg/api/grpc/proto"
)

// Handler wraps the gRPC client and provides HTTP handlers
type Handler struct {
	client pb.VectorDBClient
}

// NewHandler creates a new REST API handler
func NewHandler(client pb.VectorDBClient) *Handler {
	return &Handler{
		client: client,
	}
}

// HealthCheck handles GET /v1/health
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := h.client.HealthCheck(r.Context(), &pb.HealthCheckRequest{})
	if err != nil {
		writeError(w, fmt.Sprintf("Health check failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp, http.StatusOK)
}

// GetStats handles GET /v1/stats and GET /v1/stats/{namespace}
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract namespace from URL path if present
	path := strings.TrimPrefix(r.URL.Path, "/v1/stats")
	namespace := strings.TrimPrefix(path, "/")

	req := &pb.StatsRequest{}
	if namespace != "" {
		req.Namespace = &namespace
	}

	resp, err := h.client.GetStats(r.Context(), req)
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp, http.StatusOK)
}

// Insert handles POST /v1/vectors
func (h *Handler) Insert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req pb.InsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := h.client.Insert(r.Context(), &req)
	if err != nil {
		writeError(w, fmt.Sprintf("Insert failed: %v", err), http.StatusInternalServerError)
		return
	}

	if !resp.Success {
		status := http.StatusInternalServerError
		if resp.Error != nil {
			writeError(w, *resp.Error, status)
		} else {
			writeError(w, "Insert failed", status)
		}
		return
	}

	writeJSON(w, resp, http.StatusCreated)
}

// Search handles POST /v1/vectors/search
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req pb.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := h.client.Search(r.Context(), &req)
	if err != nil {
		writeError(w, fmt.Sprintf("Search failed: %v", err), http.StatusInternalServerError)
		return
	}

	if resp.Error != nil && *resp.Error != "" {
		writeError(w, *resp.Error, http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp, http.StatusOK)
}

// HybridSearch handles POST /v1/vectors/hybrid-search
func (h *Handler) HybridSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req pb.HybridSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := h.client.HybridSearch(r.Context(), &req)
	if err != nil {
		writeError(w, fmt.Sprintf("Hybrid search failed: %v", err), http.StatusInternalServerError)
		return
	}

	if resp.Error != nil && *resp.Error != "" {
		writeError(w, *resp.Error, http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp, http.StatusOK)
}

// Delete handles DELETE /v1/vectors/{namespace}/{id} and POST /v1/vectors/delete
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	var req pb.DeleteRequest

	if r.Method == http.MethodDelete {
		// Parse namespace and id from URL path
		// URL format: /v1/vectors/{namespace}/{id}
		path := strings.TrimPrefix(r.URL.Path, "/v1/vectors/")
		parts := strings.SplitN(path, "/", 2)

		if len(parts) != 2 {
			writeError(w, "Invalid URL format, expected /v1/vectors/{namespace}/{id}", http.StatusBadRequest)
			return
		}

		req.Namespace = parts[0]
		req.Selector = &pb.DeleteRequest_Id{Id: parts[1]}

	} else if r.Method == http.MethodPost {
		// Parse request body
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp, err := h.client.Delete(r.Context(), &req)
	if err != nil {
		writeError(w, fmt.Sprintf("Delete failed: %v", err), http.StatusInternalServerError)
		return
	}

	if !resp.Success {
		status := http.StatusInternalServerError
		if resp.Error != nil {
			writeError(w, *resp.Error, status)
		} else {
			writeError(w, "Delete failed", status)
		}
		return
	}

	writeJSON(w, resp, http.StatusOK)
}

// Update handles PUT/PATCH /v1/vectors/{namespace}/{id}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse namespace and id from URL path
	path := strings.TrimPrefix(r.URL.Path, "/v1/vectors/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) != 2 {
		writeError(w, "Invalid URL format, expected /v1/vectors/{namespace}/{id}", http.StatusBadRequest)
		return
	}

	var req pb.UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Override namespace and id from URL
	req.Namespace = parts[0]
	req.Id = parts[1]

	resp, err := h.client.Update(r.Context(), &req)
	if err != nil {
		writeError(w, fmt.Sprintf("Update failed: %v", err), http.StatusInternalServerError)
		return
	}

	if !resp.Success {
		status := http.StatusInternalServerError
		if resp.Error != nil {
			writeError(w, *resp.Error, status)
		} else {
			writeError(w, "Update failed", status)
		}
		return
	}

	writeJSON(w, resp, http.StatusOK)
}

// BatchInsert handles POST /v1/vectors/batch
func (h *Handler) BatchInsert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body as array of InsertRequest
	var requests []pb.InsertRequest
	if err := json.NewDecoder(r.Body).Decode(&requests); err != nil {
		writeError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Create streaming client
	stream, err := h.client.BatchInsert(r.Context())
	if err != nil {
		writeError(w, fmt.Sprintf("Failed to create batch insert stream: %v", err), http.StatusInternalServerError)
		return
	}

	// Send all requests
	for _, req := range requests {
		if err := stream.Send(&req); err != nil {
			writeError(w, fmt.Sprintf("Failed to send batch request: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Close and receive response
	resp, err := stream.CloseAndRecv()
	if err != nil {
		writeError(w, fmt.Sprintf("Batch insert failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, resp, http.StatusCreated)
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
	}
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":  message,
		"status": statusCode,
	})
}

// ServeDocs serves the OpenAPI/Swagger documentation
func ServeDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read and serve the OpenAPI spec file
	content, err := os.ReadFile("docs/api/openapi.yaml")
	if err != nil {
		writeError(w, "OpenAPI spec not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// ServeSwaggerUI serves the Swagger UI HTML page
func ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Serve a simple HTML page with Swagger UI
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Vector DB API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "/docs/openapi.yaml",
                dom_id: '#swagger-ui',
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIBundle.SwaggerUIStandalonePreset
                ],
                layout: "BaseLayout"
            });
        };
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// ParseIntQuery parses an integer query parameter
func ParseIntQuery(r *http.Request, key string, defaultValue int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return parsed
}
