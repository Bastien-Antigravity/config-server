---
microservice: 08-Base-Scripts
type: note
status: active
tags:
- '#service/08-Base-Scripts'
- '#type/note'
- '#state/active'
- '#zone/3-fleet'
---# 🧬 Project DNA: config-server

## 🎯 High-Level Intent (BDD)
- **Goal**: Centralized configuration management for the entire microservice ecosystem.
- **Key Pattern**: **Externalized Configuration Pattern**.

## 🛠 Technical Constraints
- **Language**: Go
- **Persistence**: File-based (JSON) with local cache and debounced background persistence (COW pattern).
- **Communication**: TCP/Safe-Socket (framed, length-prefixed) using Protobuf schemas.
- **Architecture Standard**: Adheres to the ecosystem-wide standards in .

## 👥 Roles & Responsibilities
- **Architect**: 
    - Ensure configuration integrity and atomic updates via Mutex+COW.
    - Implement non-blocking broadcast synchronization.
- **QA**: 
    - Verify config propagation latency and connection pruning (IdleTimeout).
- **Developer**:
    - Follow the Go coding standards and structured logging (universal-logger).
    - Ensure new operations align with the `ConfigMsg` Protobuf schema.
