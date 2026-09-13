# === BUILD STAGE ===
FROM golang:1.25-alpine AS builder

LABEL org.opencontainers.image.source="https://github.com/Bastien-Antigravity/config-server"

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev ca-certificates tzdata

WORKDIR /workspace

# Clone shared library modules for replace directives in builder stage
RUN git clone --depth 1 https://github.com/Bastien-Antigravity/microservice-toolbox.git /workspace/microservice-toolbox && \
    git clone --depth 1 https://github.com/Bastien-Antigravity/distributed-config.git /workspace/distributed-config && \
    git clone --depth 1 https://github.com/Bastien-Antigravity/safe-socket.git /workspace/safe-socket && \
    git clone --depth 1 https://github.com/Bastien-Antigravity/universal-logger.git /workspace/universal-logger && \
    git clone --depth 1 https://github.com/Bastien-Antigravity/flexible-logger.git /workspace/flexible-logger

# Copy config-server source
WORKDIR /workspace/config-server
COPY . .

# Ensure dependencies are tidy and build binary
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /config-server-bin ./cmd/config-server

# === RUNTIME STAGE ===
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /config-server

# Copy binary from builder stage
COPY --from=builder /config-server-bin /config-server/config-server

# Set the entrypoint
ENTRYPOINT ["/config-server/config-server"]

