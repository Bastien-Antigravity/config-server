# 🧬 Project DNA: config-server

## 🎯 High-Level Intent (BDD)
- **Goal**: Provide the centralized, authoritative storage and distribution hub for all system configurations and encrypted secrets.
- **Key Pattern**: **Single Point of Truth** (JSON/YAML backend with live pub-sub distribution) and **Secret Hardening** (server-side decryption/encryption).
- **Behavioral Source of Truth**: [[business-bdd-brain/02-Behavior-Specs/config-server]]

## 🛠️ Role Specifics
- **Architect**: 
    - Ensure atomic persistence of the configuration store to prevent corruption during crashes.
    - Maintain gRPC and TCP protocol parity for configuration fetching.
- **QA**: 
    - Verify that configuration changes are broadcasted to all active subscribers within < 50ms.
    - Test the "Bootstrapping" sequence (server starting without prior state).
- **Developer**:
    - Follow the strict `distconf` and `toolbox` integration rules.

## 🚦 Lifecycle & Versioning
- **Primary Branch**: `develop`
- **Protected Branches**: `main`, `master`
- **Versioning Strategy**: Semantic Versioning (vX.Y.Z).
- **Version Source of Truth**: `VERSION.txt`.
