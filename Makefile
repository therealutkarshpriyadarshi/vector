.PHONY: help init build test bench run proto clean lint

# Default target
help:
	@echo "Vector Database - Development Commands"
	@echo ""
	@echo "Setup:"
	@echo "  make init          Initialize project and install dependencies"
	@echo ""
	@echo "Development:"
	@echo "  make build         Build the server binary"
	@echo "  make run           Run the server"
	@echo "  make test          Run unit tests"
	@echo "  make bench         Run benchmarks"
	@echo "  make proto         Generate protobuf code"
	@echo ""
	@echo "Quality:"
	@echo "  make lint          Run linter"
	@echo "  make fmt           Format code"
	@echo "  make vet           Run go vet"
	@echo ""
	@echo "Testing:"
	@echo "  make integration   Run integration tests"
	@echo "  make recall-test   Test HNSW recall accuracy"
	@echo ""
	@echo "Cleanup:"
	@echo "  make clean         Remove build artifacts"

# Initialize project
init:
	@echo "Initializing Go module..."
	go mod tidy
	@echo "Installing dependencies..."
	go get github.com/dgraph-io/badger/v4
	go get github.com/blevesearch/bleve/v2
	go get google.golang.org/grpc
	go get google.golang.org/protobuf/cmd/protoc-gen-go
	go get google.golang.org/grpc/cmd/protoc-gen-go-grpc
	go get github.com/stretchr/testify/assert
	@echo "Creating directory structure..."
	mkdir -p cmd/server cmd/cli
	mkdir -p pkg/hnsw pkg/storage pkg/search pkg/api/grpc/proto pkg/tenant pkg/config
	mkdir -p internal/cache internal/simd internal/quantization
	mkdir -p test/integration test/benchmark test/testdata
	mkdir -p examples/basic examples/rag
	mkdir -p docs scripts
	@echo "✅ Project initialized!"

# Build server
build:
	@echo "Building vector database server..."
	go build -o bin/vector-server ./cmd/server
	@echo "✅ Built bin/vector-server"

# Build CLI
build-cli:
	@echo "Building CLI tool..."
	go build -o bin/vector-cli ./cmd/cli
	@echo "✅ Built bin/vector-cli"

# Run server
run:
	@echo "Starting vector database server..."
	go run ./cmd/server

# Run tests
test:
	@echo "Running unit tests..."
	go test ./pkg/... -v -cover

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test ./pkg/hnsw -bench=. -benchmem
	go test ./test/benchmark -bench=. -benchmem

# Run integration tests
integration:
	@echo "Running integration tests..."
	go test ./test/integration/... -v

# Test HNSW recall accuracy
recall-test:
	@echo "Testing HNSW recall vs brute force..."
	go test ./test/benchmark -run TestRecall -v

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		pkg/api/grpc/proto/*.proto
	@echo "✅ Protobuf code generated"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "✅ Code formatted"

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...
	@echo "✅ Vet passed"

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...
	@echo "✅ Lint passed"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -rf test/testdata/*.db
	@echo "✅ Cleaned"

# Download test datasets
download-testdata:
	@echo "Downloading test datasets..."
	@mkdir -p test/testdata
	@echo "⚠️  Add your dataset download commands here"

# Run load tests
load-test:
	@echo "Running load tests..."
	@echo "⚠️  Implement with hey or k6"

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t vector-db:latest .

# Docker run
docker-run:
	@echo "Running Docker container..."
	docker run -p 9000:9000 vector-db:latest

# Show coverage
coverage:
	@echo "Generating coverage report..."
	go test ./pkg/... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

# Profile CPU
profile-cpu:
	@echo "Running CPU profiler..."
	go test ./pkg/hnsw -cpuprofile=cpu.prof -bench=.
	go tool pprof -http=:8080 cpu.prof

# Profile memory
profile-mem:
	@echo "Running memory profiler..."
	go test ./pkg/hnsw -memprofile=mem.prof -bench=.
	go tool pprof -http=:8080 mem.prof

# Quick check before commit
check: fmt vet test
	@echo "✅ Pre-commit checks passed!"
