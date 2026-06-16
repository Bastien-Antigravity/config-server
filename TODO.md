---
microservice: obsidian-brain
type: note
status: active
tags:
- '#service/obsidian-brain'
- '#type/note'
- '#state/active'
- '#zone/3-fleet'
---# TODO: config-server

## 🚨 High Priority (Governance Gaps)
- [x] **Atomic Persistence (Purger Rule)**: Replace `os.WriteFile` with a "Write-to-Tmp & Rename" strategy to prevent file corruption during crashes (FEAT-004). (Validated with Sync and CreateTemp)
- [x] **Ghost Listener Cleanup**: In `broadcastUpdate`, ensure `removeListener(name)` is called if a write to a socket fails, to prevent zombie listeners (FEAT-003). (Validated in writer loop)
- [x] **Shadow Port Audit**: Verify that the gRPC registry automatically defaults to **[[08-Networking-Protocols#5-The-Shadow-Port-Protocol|Base Port + 1]]** for all discovered nodes. (Implemented in distributed-config)

## 🏗️ Architecture & Refactoring
- [x] Implement support for `Full Refresh` requests from clients. (Added FULL_REFRESH cmd to proto and handler)
- [ ] Optimize Broadcaster to use a worker pool for massive fleets.

## 🧪 Testing & CI/CD
- [ ] Add chaos tests for server crashes during persistence writes.

## ✅ Completed
- [x] Initial BDD Spec migration.