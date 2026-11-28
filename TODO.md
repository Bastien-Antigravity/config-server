---
microservice: config-server
type: note
status: active
tags:
- '#service/config-server'
- '#type/note'
- '#state/active'
- '#zone/3-fleet'
---

# TODO: config-server

## 🚨 High Priority (Governance Gaps)
- [x] **Atomic Persistence (Purger Rule)**: Replace `os.WriteFile` with a "Write-to-Tmp & Rename" strategy to prevent file corruption during crashes (FEAT-004). (Validated with Sync and CreateTemp)
- [x] **Ghost Listener Cleanup**: In `broadcastUpdate`, ensure `removeListener(name)` is called if a write to a socket fails, to prevent zombie listeners (FEAT-003). (Validated in writer loop)
- [x] **Shadow Port Audit**: Verify that the gRPC registry automatically defaults to **[[08-Networking-Protocols#5-The-Shadow-Port-Protocol|Base Port + 1]]** for all discovered nodes. (Implemented in distributed-config)
- [x] **Connection Teardown Hardening**: Single-execution `sync.Once` kill switch prevents closed-channel panic between reader and writer loops.
- [x] **Multi-Instance Collision Resilience**: Disambiguate duplicate service client connections from identical hosts (FEAT-001 Scenario 3).
- [x] **OpenMFE Shadow DOM Isolation**: Encapsulate MFE CSS styles and modal element queries within open Shadow Root.

## 🏗️ Architecture & Refactoring
- [x] Implement support for `Full Refresh` requests from clients. (Added FULL_REFRESH cmd to proto and handler)
- [x] Synchronize `GetConfig` fallback with merged `AppConfig` baseline.
- [x] Remove dead code (`cmd/test`, `cmd/test_client`, unused `pm` param in `core.ProcessRequest`).
- [ ] Optimize Broadcaster to use a worker pool for massive fleets (1000+ nodes).

## 🧪 Testing & CI/CD
- [x] Unit test coverage for `src/server` (collision handling, GetConfig fallback, status telemetry).
- [x] Unit test coverage for `src/telegram` (MenuManager rebuild).
- [ ] Add chaos tests for server crashes during persistence writes.

## ✅ Completed
- [x] Initial BDD Spec migration.
- [x] Complete quick-overview documentation set.

## 🔎 Deep Audit — 2026-11-21

### Critical Findings

- [x] **[RESOLVED] Mailbox send/close race**: Added `done chan struct{}` and `closeOnce sync.Once` (`Close()`) to `clientMailbox` so `mailbox.send` is never closed while `broadcastUpdate` or response delivery selects on `case <-mailbox.done:`.
  - Evidence: `src/server/server.go` and `src/server/connection.go`.
  - Verified by `TestServer_ConcurrentBroadcastAndDisconnectNoPanic` and `TestServer_SuperseededListenerClosesMailbox` under `go test -race ./...`.

### High Findings

- [x] **[RESOLVED] Broadcast ordering**: TCP and Controller mutations now return the exact committed store snapshot & version inside `Store` critical sections and enqueue outbound frames synchronously under `s.broadcastMu` without unordered goroutines (`src/core/request_handler.go`, `src/server/controller.go`, `src/server/server.go`).
- [x] **[RESOLVED] Revisions are now persisted alongside config state**: `PersistenceManager` saves/loads `__store_version__` envelopes (`SaveWithVersion`/`LoadWithVersion`) while keeping backward compatibility with legacy flat JSON files, and `distributed-config` clients adopt authoritative `GET_SYNC`/`FULL_REFRESH` versions after server restarts. (`ConfigMsg.Version` wire width remains `int32` across the shared proto schema).

- [x] **[RESOLVED] Reload error handling ignores persistence failures**: `ReloadConfig` now logs and returns `fmt.Errorf("failed to load persisted store: %w", err)` when `s.Persistence.Load()` fails, and `persistenceWorker` / `SaveSync` re-mark `s.dirty.Store(true)` when `s.Persistence.Save()` fails so failed saves are retried on the next tick.
  - Evidence: `src/server/server.go: ReloadConfig`, `flushDirtyOnce`, and `SaveSync`.
  - Verified by `TestServer_PersistenceRetryAndReloadErrorPropagation`.

- [ ] **[STATIC REVIEW] Management API lacks authentication/authorization**: REST mutation endpoints and wildcard CORS are reachable without an authentication or authorization guard. This includes set, reload, persist, list, status, and contracts endpoints.
  - Evidence: `src/rest/rest_handler.go: Handler`.
  - Required coverage: unauthorized set/reload/persist requests, origin constraints, authenticated mutation success, cross-service access denial.

- [x] **[RESOLVED] Management protocol OCC and delete parity**: Added `POST /api/v1/config/delete` (REST + OpenMFE), `DeleteConfig` RPC, `SetConfigRequest.expected_version`, and `GetStatusResponse.store_version` (gRPC).
  - Verified by `TestHandler_DeleteConfigAndValidation` (`src/rest`) and `TestControlServiceImpl_DeleteAndOCCConflict` (`src/grpc_control`).

### Medium Findings

- [x] **[RESOLVED] gRPC running state is now synchronized**: Converted `GRPCService.running` to `atomic.Bool` and guarded listener/server access with `sync.Mutex` (`src/grpc_control/grpc_service.go`).

- [x] **[RESOLVED] `PersistConfig` executes durable synchronous commit**: `Server.PersistConfig` now calls `SaveSync()` returning disk I/O errors directly to REST, gRPC, and Telegram callers (`TestServer_PersistConfigDurableSave`).

- [ ] **[STATIC REVIEW] REST timestamps do not follow the ecosystem UTC RFC3339 contract**: Several REST responses use Unix seconds rather than RFC3339 UTC strings.
  - Evidence: `src/rest/rest_handler.go`.
  - Required coverage: status/config/reload/persist response timestamp format and UTC suffix/value assertions.

- [x] **[RESOLVED] REST set/delete requests enforce strict body constraints**: `handleSetConfig` and `handleDeleteConfig` apply `http.MaxBytesReader` (1 MB limit) and return `400 Bad Request` on missing `section`/`key` (`TestHandler_DeleteConfigAndValidation`).

- [x] **[RESOLVED] Registry broadcasts are serialized**: `broadcastRegistry` acquires `s.broadcastMu` before reading `s.listeners` and enqueueing `BROADCAST_REGISTRY`, preventing out-of-order registry frames under listener churn.

- [ ] **[STATIC REVIEW] Health reporting is unconditional**: `GetStatus` returns `Healthy: true` regardless of persistence, listener, network, or internal degradation state.
  - Evidence: `src/server/controller.go: GetStatus`.
  - Required coverage: persistence failure, TCP accept failure, listener saturation, and degraded status behavior.

- [x] **[RESOLVED] TEST coverage — broadcast ordering and mailbox close races**: Added `TestServer_ConcurrentMutationsPreserveStrictBroadcastVersionOrder` alongside `TestServer_ConcurrentBroadcastAndDisconnectNoPanic`.

- [x] **[RESOLVED] TEST coverage — persistence crash/recovery and reload failures**: Covered by `TestServer_PersistenceRetryAndReloadErrorPropagation`, `TestServer_PersistConfigDurableSave`, and `TestPersistenceVersionEnvelopeAndLegacyCompat`.

- [ ] **[STATIC REVIEW] TEST gap — API security contract**: Existing REST tests validate CORS and status/method behavior but do not test authorization or mutation denial from unauthorized clients.

- [x] **[RESOLVED] TEST coverage — OCC/revision continuity**: Covered by `TestPersistenceVersionEnvelopeAndLegacyCompat`, `TestServer_ReloadPreservesBaselineAndFiltersWildcardIP`, and `TestProtoHandler_ServerRestartResetAndAckDedup`.

- [x] **[RESOLVED] TEST coverage — protocol parity**: Covered by `TestHandler_DeleteConfigAndValidation`, `TestControlServiceImpl_DeleteAndOCCConflict`, and `TestMenuManager_RebuildMenu`.

- [x] **[RESOLVED] TEST coverage — resource and malformed-input behavior**: Covered by `TestHandler_DeleteConfigAndValidation` (`>1MB` payload rejection and empty section/key `400 Bad Request`).

### Verification Results

- [x] `go test ./...` passes.
- [x] `go test -race ./...` passes across `src/core`, `src/grpc_control`, `src/helpers`, `src/rest`, `src/server`, `src/store`, and `src/telegram`.
- [x] Added race-focused tests for `GRPCService` start/stop lifecycle (`TestGRPCService_StartStopLifecycleRace`) and mailbox replacement/removal versus broadcast.
- [ ] Add a socket-level chaos suite for reconnect churn and duplicate client identities.
- [x] Added tests for concurrent mutation ordering and broadcast snapshot correctness (`TestServer_ConcurrentMutationsPreserveStrictBroadcastVersionOrder`).