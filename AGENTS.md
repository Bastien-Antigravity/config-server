# AGENTS.md: config-server

## Service Mission & Architecture Role
`config-server` is the central configuration repository and real-time distributor for the Bastien-Antigravity ecosystem. It serves configuration profiles (e.g. `native.yaml`, `docker.yaml`), persists dynamic configuration state atomically, and pushes live config changes to connected client microservices over TCP, gRPC, and REST.

- **Exposed Capability**: `config_server`
  - **TCP Sync Protocol**: Port `3306` (`appConfig.GetListenAddr("config_server")`) — framed TCP distribution via `safe-socket`
  - **gRPC Shadow Port**: Port `3307` (`appConfig.GetGRPCAddr("config_server")`) — management and remote sync
  - **REST & OpenMFE**: Port `3308` (`appConfig.GetRESTAddr("config_server")`) — HTTP REST endpoints and OpenMFE micro-frontend host
- **Downstream Integrations**: `web-interface` (`127.0.0.1:5000` via dynamic MFE registration), `tele-remote` (Telegram control bot)
- **Shared Libraries**: `microservice-toolbox`, `universal-logger`, `distributed-config`, `safe-socket`
- **Configuration Link**: `standalone.yaml -> ../docker-deployment/modes/local/config/native.yaml`

## Key Build & Test Commands
```bash
# Build binary
go build -o bin/config-server ./cmd/config-server

# Run unit tests
go test -v ./...

# Run service locally
./bin/config-server

# Run with custom persistence store path
./bin/config-server --store /path/to/custom_store.json
```

## Internal Architecture & Subsystems
1. **Core & Store (`src/core`, `src/store`)**:
   - In-memory store using Copy-On-Write (COW) mutex semantics to guarantee read consistency and atomic updates.
   - Debounced file persistence worker flushing state to `config_store.json` (or path passed via `--store` flag).
2. **Network Layer (`src/server`)**:
   - Built on `safe-socket` (`tcp-hello` profile) with handshake verification (`Name`/`Group`) and 10-minute idle timeouts.
   - Per-client non-blocking mailbox buffers with dedicated write goroutines to prevent slow consumers from hanging the broadcast loop.
3. **Control Interfaces (`src/grpc_control`, `src/rest`)**:
   - `grpc_control`: Exposes `ConfigService` for gRPC sync and update streams.
   - `rest`: Serves REST routes (`/api/v1/config`, `/api/v1/health`) and hosts OpenMFE web assets (`/static/js/mfe-loader.js`).
4. **OpenMFE Dynamic Registration**:
   - Auto-registers with `web-interface` at `http://<webAddr>/api/v1/register` with exponential backoff on startup.
5. **Tele-Remote Integration (`src/telegram`)**:
   - Binds to `tele-remote` for live menu controls and configuration tweak callbacks.

## AI Development & Integration Guidelines
1. **Bootstrap Ritual**: Initialized using `toolbox_bootstrap.BootstrapService("config-server", "store")`. Never bypass the toolbox bootstrapper.
2. **Dynamic Port Resolution**: Always use the specific toolbox accessors:
   - `GetListenAddr("config_server")` for port `3306`
   - `GetGRPCAddr("config_server")` for port `3307`
   - `GetRESTAddr("config_server")` for port `3308`
   - NEVER hardcode host IP or port numbers.
3. **Encrypted Secrets**: Sensitive values stored in configurations must remain encrypted as `ENC(...)` tokens until runtime decryption via `appConfig.DecryptSecret()`.
4. **Header Ritual**: All Go source files MUST begin with the standard Triple-Block header (`ESSENTIAL PROCESS`, `DATA FLOW`, `KEY PARAMETERS`).
5. **Section Dividers**: Use `// -----------------------------------------------------------------------------` between exported methods and major sections.
6. **Graceful Shutdown**: All long-running workers (TCP server, gRPC service, persistence worker) must be registered with `toolbox_lifecycle.Manager` to flush state cleanly on `SIGINT`/`SIGTERM`.
