FROM golang:1.24-alpine@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /shim ./cmd/shim

# Runtime stage
FROM alpine:3.19@sha256:6baf43584bcb78f2e5847d1de515f23499913ac9f12bdf834811a3145eb11ca1

# Add ca-certificates for HTTPS calls to AIDR API
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /shim /shim

# Run as non-root user
RUN adduser -D -g '' appuser
USER appuser

# Expose gRPC and health check ports
EXPOSE 8080 8081

CMD ["/shim"]
