# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=1 go build -o langdag ./cmd/langdag

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Copy binary from builder
COPY --from=builder /app/langdag /usr/local/bin/langdag

# Create data directory
RUN mkdir -p /data

# Set environment variables
ENV LANGDAG_STORAGE_PATH=/data/langdag.db

# Expose default port
EXPOSE 8080

# Default command
ENTRYPOINT ["langdag"]
CMD ["serve", "--host", "0.0.0.0", "--port", "8080"]
