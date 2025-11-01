# Build stage
FROM golang:1.22-alpine AS builder

# Install git and ca-certificates for dependencies and HTTPS
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build client binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o renoter-client ./cmd/client

# Build server binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o renoter-server ./cmd/server

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS connections
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 renoter && \
    adduser -D -u 1000 -G renoter renoter

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/renoter-client .
COPY --from=builder /build/renoter-server .

# Change ownership to non-root user
RUN chown -R renoter:renoter /app

# Switch to non-root user
USER renoter

# Default command (can be overridden)
CMD ["./renoter-server"]

