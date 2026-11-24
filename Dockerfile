# === BUILD STAGE ===
FROM golang:1.25-alpine AS builder

LABEL org.opencontainers.image.source="https://github.com/Bastien-Antigravity/config-server"

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev ca-certificates tzdata

WORKDIR /workspace

# Clone shared library modules for go.work resolution in builder stage
RUN git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/microservice-toolbox.git /workspace/microservice-toolbox && \
    git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/distributed-config.git /workspace/distributed-config && \
    git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/safe-socket.git /workspace/safe-socket && \
    git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/universal-logger.git /workspace/universal-logger && \
    git clone --depth 1 -b develop https://github.com/Bastien-Antigravity/flexible-logger.git /workspace/flexible-logger

# Copy config-server source
WORKDIR /workspace/config-server
COPY . .

# Initialize workspace go.work for cloned sibling modules and build binary
RUN cd /workspace && \
    go work init ./config-server ./microservice-toolbox ./distributed-config ./safe-socket ./universal-logger ./flexible-logger && \
    cd /workspace/config-server && \
    VERSION=$(cat VERSION.txt) && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X 'github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/version.BuildVersion=${VERSION}'" -o /config-server-bin ./cmd/config-server

# === RUNTIME STAGE ===
FROM alpine:3.20

LABEL org.opencontainers.image.source="https://github.com/Bastien-Antigravity/config-server"

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /config-server

# Copy binary and VERSION.txt from builder stage
COPY --from=builder /config-server-bin /config-server/config-server
COPY --from=builder /workspace/config-server/VERSION.txt /config-server/VERSION.txt

# Set the entrypoint
ENTRYPOINT ["/config-server/config-server"]

