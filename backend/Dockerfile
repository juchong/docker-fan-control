# Build stage - use Debian for glibc (required by go-nvml)
FROM golang:1.22 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy all source files first (needed for go mod tidy to resolve dependencies)
COPY . .

# Generate go.sum and download dependencies
RUN go mod tidy && go mod download

# Build
RUN CGO_ENABLED=1 GOOS=linux go build -o /fan-control ./cmd/server

# Runtime stage - use Debian for glibc compatibility
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ipmitool \
    lm-sensors \
    smartmontools \
    ca-certificates \
    tzdata \
    wget \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user (but we'll run as root for IPMI access)
RUN useradd -m -u 1000 fancontrol

WORKDIR /app

# Copy binary
COPY --from=builder /fan-control /app/fan-control

# Create data directory
RUN mkdir -p /app/data && chown -R fancontrol:fancontrol /app/data

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run as root for IPMI access
CMD ["/app/fan-control"]
