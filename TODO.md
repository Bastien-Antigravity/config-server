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