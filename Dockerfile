# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags '-w -s' \
    -o sonic-change-agent \
    ./cmd/sonic-change-agent

# Final stage
FROM alpine:3.18

# Install ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder stage
COPY --from=builder /workspace/sonic-change-agent .

# Create non-root user
RUN addgroup -g 1001 sonic && \
    adduser -D -u 1001 -G sonic sonic

USER sonic

ENTRYPOINT ["./sonic-change-agent"]