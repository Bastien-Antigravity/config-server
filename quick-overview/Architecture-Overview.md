---
microservice: config-server
type: overview
status: active
tags:
- '#service/config-server'
- '#domain/configuration'
- '#domain/architecture'
- '#zone/3-fleet'
- '#type/overview'
- '#state/active'
- '#ai/ignore'
---
# Architecture Overview: config-server

`config-server` decouples configuration state storage and real-time distribution from physical transport protocols through a layered, decoupled design.

## 🏗️ System Architecture

```
                    ┌────────────────────────┐
                    │  Fleet Microservices   │
                    └───────────┬────────────┘
                                │ SafeSocket TCP (3306)
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│  src/server (Connection & Network Daemon)                        │
│   ├── Socket Listener (safe-socket tcp-hello profile)            │
│   ├── Dual-Loop Handler (Reader + Writer goroutines)             │
│   └── Per-Client Non-Blocking Mailboxes (buffer: 3 messages)     │
└───────────────────────────────┬──────────────────────────────────┘
                                │
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│  src/core (Unified Business Logic)                               │
│   ├── ProcessRequest (Protobuf ConfigMsg dispatcher)             │
│   └── ConfigController Interface                                 │
└───────┬───────────────────────┬──────────────────────────┬───────┘
        │                       │                          │
        ▼                       ▼                          ▼
┌──────────────┐       ┌─────────────────┐        ┌─────────────────┐
│ src/store    │       │ src/grpc_control│        │ src/rest        │
│ ├── In-Memory│       │ └── Port 3307   │        │ ├── Port 3308   │
│ │   COW Store│       │     gRPC Shadow │        │ └── OpenMFE     │
│ └── Atomic   │       │     Management  │        │     Shadow DOM  │
│     JSON Save│       └─────────────────┘        └─────────────────┘
└──────────────┘
```

## 🧩 Subsystem Breakdown

### 1. SafeSocket Dual-Loop Connection Model
- **Handshake Authentication**: Every TCP connection on port `3306` must immediately exchange identity metadata (`Name`, `Address`).
- **Dedicated Mailbox Pattern**: Each client receives an isolated channel (`chan []byte, buffer: 3`). Broadcasts use non-blocking `select`: if a consumer is unresponsive, messages are dropped to prevent fleet-wide stalling.
- **`sync.Once` Coordinated Teardown**: Either side of the connection (reader error or writer socket failure) triggers a single-execution teardown that cleanly closes the socket, stops goroutines, and unregisters the client from `s.listeners` without channel panics.

### 2. Management Interfaces (gRPC, REST, OpenMFE)
- **gRPC Shadow Port (`3307`)**: Follows the Shadow Port Protocol (`Base Port + 1`). Serves `ConfigControlService` for typed administrative automation and gRPC health checks.
- **REST & OpenMFE Host (`3308`)**: Serves JSON endpoints under `/api/v1/config/` and `/api/v1/status`. Hosts `/static/js/mfe-loader.js`, dynamically mounting an isolated Web Component on `web-interface`.
- **Tele-Remote Integration**: Binds `MenuManager` to the `tele-remote` daemon over gRPC, mapping configuration sections into an interactive Telegram button tree.

### 3. Store & Persistence Layer
- **Copy-On-Write (COW)**: Implements atomic pointer swaps for state updates. Read operations (`Get`) return immutable memory views without holding write locks.
- **Debounced Persistence Worker**: A dedicated background worker evaluates the atomic `dirty` flag every 5 seconds. On shutdown, `toolbox_lifecycle.Manager` executes a synchronous final save before process exit.
