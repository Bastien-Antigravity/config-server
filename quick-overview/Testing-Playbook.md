---
microservice: config-server
type: overview
status: active
tags:
- '#service/config-server'
- '#domain/testing'
- '#domain/quality-assurance'
- '#zone/3-fleet'
- '#type/overview'
- '#state/active'
- '#ai/ignore'
---
# Testing Playbook: config-server

Comprehensive testing guide, test commands, and BDD scenario mapping for `config-server`.

## 🧪 Test Execution Commands

```bash
# Run all unit tests with verbose output
make test
# Or:
go test -v ./...

# Run race condition detector
make race
# Or:
go test -race ./...

# Run Go static analyzer (vet)
make vet
# Or:
go vet ./...

# Run test coverage reporting across all packages
go test -cover ./...

# Run zero-allocation hot-path benchmarks
go test -bench=. -benchmem ./src/store

# Compile production binary
make build
```

## 📦 Package Test Suites

| Package | Test File | Primary Coverage Scope |
|---|---|---|
| **`src/core`** | `request_handler_test.go` | Protobuf unmarshaling, `GET_SYNC`, `FULL_REFRESH`, atomic `PUT_SYNC` & `DELETE_SYNC`, OCC conflict detection, unknown command rejection. |
| **`src/grpc_control`** | `service_test.go` | `ConfigControlService` RPC methods (`GetConfig`, `SetConfig` with OCC, `DeleteConfig`, `ListConfig`, `ReloadConfig`, `PersistConfig`, `GetStatus`), and `GRPCService` start/stop race lifecycle (`TestGRPCService_StartStopLifecycleRace`). |
| **`src/helpers`** | `config_updates_test.go` | `ApplyUpdates` delta map merging, `__DELETED__` sentinel removal, and `RedactValue` secret masking (`TestRedactValue`). |
| **`src/rest`** | `rest_handler_test.go` | REST routing (`/get`, `/set`, `/delete`, `/list`, `/reload`, `/persist`, `/status`, `/contracts`), OCC `409 Conflict`, `400 Bad Request` & `>1MB` body cap, CORS preflight, OpenMFE bundle delivery. |
| **`src/server`** | `server_test.go` | Collision-resilient client registration, serialized monotonic broadcast ordering (`TestServer_ConcurrentMutationsPreserveStrictBroadcastVersionOrder`), `ReloadConfig` baseline preservation & wildcard `0.0.0.0` filtering, synchronous `PersistConfig` durability. |
| **`src/store`** | `store_test.go` | Copy-On-Write (COW) atomicity, failure rollback, OCC version checks, concurrent stress reads/writes, and zero-allocation read benchmarks (`BenchmarkStore_Get`, `BenchmarkStore_GetWithVersion`, `BenchmarkStore_UpdateSection`). |
| **`src/store`** | `persistence_test.go` | Write-to-temp atomic renaming, versioned JSON envelope (`__store_version__`), legacy flat JSON backward compatibility, missing file recovery. |
| **`src/telegram`** | `manager_test.go` | Deterministic sorted section/key menu action tree generation, secret redaction in menu labels, storage operation triggers, mutation callbacks. |

## 🛡️ BDD Specification Traceability

- **[FEAT-001: Mutual Identity Handshake](file:///Users/imac/Desktop/Bastien-Antigravity/obsidian-brain/02-Business-BDD/02-Behavior-Specs/config-server/FEAT-001-Handshake-Identity.md)**:
  - Scenario 1 (Handshake): Verified in `server.handleConnection`.
  - Scenario 3 (Collision Avoidance): Verified in `server_test.go:TestServer_AddAndRemoveListener_CollisionSafe`.
- **[FEAT-002: Atomic State Swap](file:///Users/imac/Desktop/Bastien-Antigravity/obsidian-brain/02-Business-BDD/02-Behavior-Specs/config-server/FEAT-002-Atomic-State-Swap.md)**:
  - Scenarios 1–3 (COW, Concurrency): Verified in `store_test.go:TestStore_UpdateAtomic_SuccessAndRollback` and `TestStore_ConcurrentStress`.
- **[FEAT-003: Broadcast Propagation](file:///Users/imac/Desktop/Bastien-Antigravity/obsidian-brain/02-Business-BDD/02-Behavior-Specs/config-server/FEAT-003-Broadcast-Propagation.md)**:
  - Scenarios 1–3 (Non-blocking broadcasts, Full refresh): Verified in `core/request_handler_test.go`.
- **[FEAT-004: Persistence Safety](file:///Users/imac/Desktop/Bastien-Antigravity/obsidian-brain/02-Business-BDD/02-Behavior-Specs/config-server/FEAT-004-Persistence-Safety.md)**:
  - Scenarios 1–3 (Write-to-temp, Sync, Rename, Recovery): Verified in `store/persistence_test.go`.
