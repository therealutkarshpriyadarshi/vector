# Multi-stage Dockerfile for Vector Database
# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make protoc protobuf-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' \
    -o vector-server ./cmd/server

# Build the CLI binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' \
    -o vector-cli ./cmd/cli

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 vectordb && \
    adduser -D -u 1000 -G vectordb vectordb

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/vector-server /app/
COPY --from=builder /build/vector-cli /app/

# Create data directory
RUN mkdir -p /app/data && chown -R vectordb:vectordb /app

# Switch to non-root user
USER vectordb

# Expose gRPC port
EXPOSE 50051

# Expose Prometheus metrics port
EXPOSE 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/vector-cli", "health", "-server", "localhost:50051"] || exit 1

# Run the server
CMD ["/app/vector-server"]
