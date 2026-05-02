# TODO: config-server

## 🚨 High Priority (Governance Gaps)
- [ ] **Atomic Persistence (Purger Rule)**: Replace `os.WriteFile` with a "Write-to-Tmp & Rename" strategy to prevent file corruption during crashes (FEAT-004). (Approval Required)
- [ ] **Ghost Listener Cleanup**: In `broadcastUpdate`, ensure `removeListener(name)` is called if a write to a socket fails, to prevent zombie listeners (FEAT-003). (Approval Required)

## 🏗️ Architecture & Refactoring
- [ ] Implement support for `Full Refresh` requests from clients.
- [ ] Optimize Broadcaster to use a worker pool for massive fleets.

## 🧪 Testing & CI/CD
- [ ] Add chaos tests for server crashes during persistence writes.

## ✅ Completed
- [x] Initial BDD Spec migration.