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

- [ ] **[CONFIRMED] Mailbox send/close race**: `broadcastUpdate` can load a mailbox from the atomic listener snapshot, while `addListener` or `removeListener` simultaneously closes `mailbox.send`. The subsequent send can panic the process with `send on closed channel`. This affects both `broadcastUpdate` and TCP response delivery. Highest priority because no mailbox lifetime synchronization protects the lock-free listener map.
  - Evidence: `src/server/server.go` closes mailboxes in `addListener` and `removeListener`; `broadcastUpdate` and `handleConnection` send to the same channels.
  - Required coverage: concurrent broadcast plus listener replacement/removal, duplicate-name reconnection, reader disconnect during broadcast, and repeated stress/fuzz runs.

### High Findings

- [ ] **[STATIC REVIEW] Broadcast ordering**: TCP mutations launch broadcasts in `go` routines. Concurrent PUT/DELETE operations can therefore enqueue full/delta payloads in a different order than store mutations. This can leave a client divergent from the authoritative state despite receiving ACKs.
  - Evidence: `src/core/request_handler.go` launches `broadcast` through `go` for successful PUT_SYNC and DELETE_SYNC requests.
  - Required coverage: concurrent mutations to the same key/section, ordered delivery assertions, ACK-versus-broadcast sequence assertions.

- [ ] **[STATIC REVIEW] Revisions are not persisted and are narrowed to 32-bit on the wire**: Persistence stores only configuration data. `Store.Replace(initialConfig)` restarts/increments the revision, and broadcast/request/response paths cast `uint64` revisions to `int32`. This breaks OCC continuity across restart and can wrap after `2^31-1` mutations.
  - Evidence: `src/store/persistence.go`, `src/store/store.go`, `src/server/server.go`, and `src/core/request_handler.go`.
  - Required coverage: restart with persisted state, stale OCC after restart, successful OCC after restart, and revision overflow/32-bit boundary testing.

- [ ] **[STATIC REVIEW] Reload error handling ignores persistence failures**: `ReloadConfig` reloads AppConfig and ignores `Persistence.Load()` errors. It can therefore report success and broadcast stale/full state even after the persisted file cannot be read or is invalid.
  - Evidence: `src/server/server.go: ReloadConfig`.
  - Required coverage: corrupt JSON, unreadable file, missing file, load failure followed by reload response, and full-refresh consistency assertions.

- [ ] **[STATIC REVIEW] Management API lacks authentication/authorization**: REST mutation endpoints and wildcard CORS are reachable without an authentication or authorization guard. This includes set, reload, persist, list, status, and contracts endpoints.
  - Evidence: `src/rest/rest_handler.go: Handler`.
  - Required coverage: unauthorized set/reload/persist requests, origin constraints, authenticated mutation success, cross-service access denial.

- [ ] **[STATIC REVIEW] Management protocols have incompatible OCC and deletion contracts**:
  - REST supports `expected_version`.
  - gRPC `SetConfig` calls last-write-wins and has no OCC field.
  - REST and gRPC lack delete operations despite TCP exposing DELETE_SYNC.
  - Evidence: `src/rest/rest_handler.go`, `src/grpc_control/config_server.proto`, `src/grpc_control/service.go`, and `src/core/controller.go`.
  - Required coverage: contract parity matrix for get/set/delete/reload/persist/status, OCC conflict behavior, and delete propagation across every protocol.

### Medium Findings

- [ ] **[STATIC REVIEW] gRPC running state is unsynchronized**: `GRPCService.running` is a plain boolean read and written concurrently by lifecycle goroutines.
  - Evidence: `src/grpc_control/grpc_service.go`.
  - Required coverage: repeated start/stop cycles and race-detector lifecycle tests.

- [ ] **[STATIC REVIEW] Persistence can report success without confirming a durable save**: The `/persist` and dirty-worker paths do not provide a synchronous success/failure result to management clients. Background save failures can remain unreported and a failed write can remain dirty only after a later mutation.
  - Evidence: `src/server/controller.go: PersistConfig`, `src/server/server.go: persistenceWorker`, and `src/store/persistence.go`.
  - Required coverage: simulated write, sync, rename, and directory-sync failures; persisted-state verification after a successful administrative flush; retry behavior.

- [ ] **[STATIC REVIEW] REST timestamps do not follow the ecosystem UTC RFC3339 contract**: Several REST responses use Unix seconds rather than RFC3339 UTC strings.
  - Evidence: `src/rest/rest_handler.go`.
  - Required coverage: status/config/reload/persist response timestamp format and UTC suffix/value assertions.

- [ ] **[STATIC REVIEW] REST set requests lack strict body constraints**: The set endpoint lacks an explicit body limit, strict JSON decoding, and strong section/key/value validation.
  - Evidence: `src/rest/rest_handler.go: handleSetConfig`.
  - Required coverage: oversized body, trailing JSON, unknown fields, empty section/key, non-string values, malformed JSON, and resource-limit behavior.

- [ ] **[STATIC REVIEW] Registry broadcasts can reorder under listener churn**: Add/remove operations launch independent registry broadcast goroutines.
  - Evidence: `src/server/server.go: addListener` and `removeListener`.
  - Required coverage: rapid add/remove/replacement bursts and ordered registry snapshot assertions.

- [ ] **[STATIC REVIEW] Health reporting is unconditional**: `GetStatus` returns `Healthy: true` regardless of persistence, listener, network, or internal degradation state.
  - Evidence: `src/server/controller.go: GetStatus`.
  - Required coverage: persistence failure, TCP accept failure, listener saturation, and degraded status behavior.

- [ ] **[STATIC REVIEW] TEST gap — broadcast ordering and mailbox close races**: Current tests do not reproduce `send on closed channel` under concurrent churn or verify mutation/broadcast ordering under concurrent writes.

- [ ] **[STATIC REVIEW] TEST gap — persistence crash/recovery and reload failures**: No deterministic tests cover corrupted, unwritable, temporarily missing, or crash-interrupted persistence files.

- [ ] **[STATIC REVIEW] TEST gap — API security contract**: Existing REST tests validate CORS and status/method behavior but do not test authorization or mutation denial from unauthorized clients.

- [ ] **[STATIC REVIEW] TEST gap — OCC/revision continuity**: OCC behavior is tested in memory, but not across persistence reload/restart or at revision boundary/overflow values.

- [ ] **[STATIC REVIEW] TEST gap — protocol parity**: REST, gRPC, TCP, and Telegram mutation behavior do not share a complete delete/OCC parity test matrix.

- [ ] **[STATIC REVIEW] TEST gap — resource and malformed-input behavior**: Existing tests do not cover oversized REST bodies, trailing JSON, strict field types, or malformed gRPC/REST requests.

### Verification Results

- [x] `go test ./...` passes.
- [x] `go test -race ./...` passes across `src/core`, `src/grpc_control`, `src/helpers`, `src/rest`, `src/server`, `src/store`, and `src/telegram`.
- [ ] Add race-focused tests for mailbox replacement/removal versus broadcast and direct response delivery.
- [ ] Add a socket-level chaos suite for reconnect churn and duplicate client identities.
- [ ] Add tests for concurrent mutation ordering and broadcast snapshot correctness.