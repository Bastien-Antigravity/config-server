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
- **Protobuf Wire Protocol**: Communicates with fleet clients via SafeSocket using the `ConfigMsg` schema from `distributed-config` (`GET_SYNC`, `PUT_SYNC`, `DELETE_SYNC`, `FULL_REFRESH`).
- **Reliable Push Broadcasts**: When updates occur, the server pushes delta updates (`BROADCAST_SYNC`) or full state refreshes (`FULL_REFRESH`) carrying monotonic version numbers. Mailbox overflows flag `needsRefresh` to gently elevate the next message to `FULL_REFRESH` without disconnecting the socket.
- **Client Synchronization & Gap Detection**: Clients fetch initial states via `GET_SYNC` and detect sequence gaps from `ConfigMsg.Version`, triggering automatic `FULL_REFRESH` recovery.

### 2. Copy-On-Write In-Memory Store
- **Lock-Free Read Architecture**: Uses `atomic.Pointer[ConfigMap]` with a Copy-On-Write (COW) pattern to provide 100% lock-free, zero-allocation instant read access (`store.Get()`, verified by `BenchmarkStore_Get` and `BenchmarkStore_GetWithVersion` in `src/store/store_test.go`), alongside `atomic.Pointer[map[string]*clientMailbox]` for lock-free listener lookups.
- **Path-Copying Granular Mutations**: `store.UpdateSection()` clones only the modified section map, structurally sharing all other sections across versions to minimize memory allocations. Monotonic revision counter tracks every state change.

### 3. Dynamic Service Registry & Auto-Discovery
- **Handshake Verification**: Authenticates connecting clients via SafeSocket `tcp-hello` handshakes, extracting advertised service names and listen addresses.
- **Dynamic Fleet Map**: Dispatches `BROADCAST_REGISTRY` containing all active service addresses (`name=address`) to allow microservices to discover peers without static DNS or proxy overhead.
- **Collision Resilience**: Automatically disambiguates duplicate service connections from the same host, ensuring multiple replica workers receive independent broadcast streams.

### 4. High-Performance Persistence
- **Fault-Tolerant Disk Writes**: In-memory updates acknowledge instantly and persist state + monotonic version via a 1-second background worker, while `PersistConfig` (REST/gRPC/Telegram) commits synchronously via `SaveSync()`.
- **Write-to-Temp & Rename**: Uses `os.CreateTemp`, explicit `Sync()`, and atomic `os.Rename` to guarantee immunity from partial writes during sudden power loss or process crashes.

### 5. Multi-Channel Administration
- **REST & OpenMFE**: Hosts JSON REST API (`/api/v1/config/get`, `/set`, `/delete`, `/list`, `/reload`, `/persist`, `/status`, `/contracts`) on port `3308` and serves an encapsulated Shadow DOM micro-frontend (`/static/js/mfe-loader.js`).
- **gRPC Shadow Port**: Exposes `ConfigControlService` on port `3307` for typed RPC control and health checks.
- **Tele-Remote Integration**: Synchronizes dynamic interactive Telegram menus with `tele-remote` for mobile inspection and tweaking.
