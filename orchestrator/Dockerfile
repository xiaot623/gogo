# Build stage
FROM golang:1.25-alpine AS builder

# Install CGO dependencies for sqlite3
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with CGO enabled for sqlite3
RUN CGO_ENABLED=1 GOOS=linux go build -o orchestrator .

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies for sqlite3
RUN apk add --no-cache libc6-compat

# Copy binary from builder
COPY --from=builder /app/orchestrator .

# Copy policies directory
COPY --from=builder /app/policies ./policies

# Expose ports
EXPOSE 8080 8081

# Run the binary
CMD ["./orchestrator"]
