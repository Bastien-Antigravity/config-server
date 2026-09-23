---
microservice: config-server
type: overview
status: active
tags:
- '#service/config-server'
- '#domain/configuration'
- '#domain/networking'
- '#zone/3-fleet'
- '#type/overview'
- '#state/active'
- '#ai/ignore'
---
# Features & Behavior: config-server

`config-server` is the authoritative configuration synchronization daemon and live discovery hub for the Bastien-Antigravity ecosystem.

## 🎯 Core Capabilities

### 1. Real-Time Dynamic Configuration Distribution
- **Protobuf Wire Protocol**: Communicates with fleet clients via SafeSocket using the `ConfigMsg` schema from `distributed-config`.
- **Non-Blocking Push Broadcasts**: When updates occur (`PUT_SYNC`, REST `/api/v1/config/set`, gRPC, or Telegram), the server pushes delta updates (`BROADCAST_SYNC`) directly to all connected client mailboxes.
- **Client Synchronization**: Clients fetch initial states via `GET_SYNC` and can request full baseline resets via `FULL_REFRESH`.

### 2. Copy-On-Write In-Memory Store
- **Thread-Safe Architecture**: Uses `sync.RWMutex` with a Copy-On-Write (COW) pattern to provide zero-allocation, instant read access (`store.Get()`).
- **Atomic Modifications**: Mutations create an isolated deep copy (`UpdateAtomic()`). If modification succeeds, the internal pointer is swapped instantaneously; on error, the sandbox is discarded without side-effects.

### 3. Dynamic Service Registry & Auto-Discovery
- **Handshake Verification**: Authenticates connecting clients via SafeSocket `tcp-hello` handshakes, extracting advertised service names and listen addresses.
- **Dynamic Fleet Map**: Dispatches `BROADCAST_REGISTRY` containing all active service addresses (`name=address`) to allow microservices to discover peers without static DNS or proxy overhead.
- **Collision Resilience**: Automatically disambiguates duplicate service connections from the same host, ensuring multiple replica workers receive independent broadcast streams.

### 4. Debounced Atomic Persistence
- **Fault-Tolerant Disk Writes**: Flushes dirty configuration state to JSON (default: `config_store.json`) on a 5-second ticker.
- **Write-to-Temp & Rename**: Uses `os.CreateTemp`, explicit `Sync()`, and atomic `os.Rename` to guarantee immunity from partial writes during sudden power loss or process crashes.

### 5. Multi-Channel Administration
- **REST & OpenMFE**: Hosts JSON REST API on port `3308` and serves an encapsulated Shadow DOM micro-frontend (`/static/js/mfe-loader.js`).
- **gRPC Shadow Port**: Exposes `ConfigControlService` on port `3307` for typed RPC control and health checks.
- **Tele-Remote Integration**: Synchronizes dynamic interactive Telegram menus with `tele-remote` for mobile inspection and tweaking.
