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

# Compile production binary
make build
```

## 📦 Package Test Suites

| Package | Test File | Primary Coverage Scope |
|---|---|---|
| **`src/core`** | `request_handler_test.go` | Protobuf unmarshaling, `GET_SYNC`, `FULL_REFRESH`, atomic `PUT_SYNC`, unknown command rejection. |
| **`src/grpc_control`** | `service_test.go` | `ConfigControlService` RPC methods, Protobuf response formatting, error translations. |
| **`src/helpers`** | `config_updates_test.go` | `ApplyUpdates` delta map merging and immutability assertions. |
| **`src/rest`** | `rest_handler_test.go` | REST routing, CORS preflight headers, OpenMFE loader asset delivery, HTTP method verification. |
| **`src/server`** | `server_test.go` | Collision-resilient client registration, duplicate connection deduplication, baseline `GetConfig` fallback, telemetry. |
| **`src/store`** | `store_test.go` | Copy-On-Write (COW) atomicity, failure rollback, concurrent stress reads/writes (20 workers). |
| **`src/store`** | `persistence_test.go` | Write-to-temp atomic renaming, directory creation, missing file recovery. |
| **`src/telegram`** | `manager_test.go` | Menu action tree generation, storage operation triggers, mutation callbacks. |

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
