---
microservice: config-server
type: overview
status: active
tags:
- '#service/config-server'
- '#domain/configuration'
- '#domain/operations'
- '#zone/3-fleet'
- '#type/overview'
- '#state/active'
- '#ai/ignore'
---
# General & Operational Guide: config-server

Operational runbook, environment parameters, troubleshooting guidelines, and security policies for `config-server`.

## ⚙️ Port Allocations & Environment Overrides

Dynamic ports are resolved via `microservice-toolbox` from `standalone.yaml` (symlinked to `../docker-deployment/modes/local/config/native.yaml`):

| Protocol | Default Port | Config Key | Environment Variable Override |
|---|---|---|---|
| **SafeSocket TCP** | `3306` | `capabilities.config_server.port` | `CF_PORT` |
| **gRPC Management** | `3307` | `capabilities.config_server.grpc_port` | `CF_GRPC_PORT` |
| **REST & OpenMFE** | `3308` | `capabilities.config_server.rest_port` | `CF_REST_PORT` |

## 🔐 Zero-Knowledge Secret Policy

- **No Server-Side Decryption**: `config-server` treats encrypted secret strings (`ENC(...)`) as opaque strings. The server never decrypts sensitive parameters.
- **Client-Side Decryption**: Authorized microservices receive encrypted tokens during sync and decrypt them in-memory using their local cryptographic master keys via `appConfig.DecryptSecret()`.
- **OpenMFE Masking**: The OpenMFE UI detects `ENC(...)` values, displays a security badge, and masks cipher text to prevent shoulder-surfing in dashboards.

## 🛠️ Operational Runbook

### Starting the Service
```bash
# Standard local start (uses standalone.yaml -> native.yaml)
./bin/config-server

# Custom persistence store location
./bin/config-server --store /var/data/custom_store.json
```

### Health Check & Inspection
```bash
# Check REST health
curl -s http://127.0.0.1:3308/api/v1/status | jq

# Inspect merged configuration
curl -s http://127.0.0.1:3308/api/v1/config/list | jq

# Trigger manual persistence flush
curl -X POST http://127.0.0.1:3308/api/v1/config/persist
```

## 🔍 Troubleshooting & Gotchas

1. **Client Dropped with "Mailbox Full"**:
   - Cause: The client socket is blocked or experiencing extreme network lag, filling its 3-message buffer.
   - Resolution: Check client CPU load or network connection. The client will automatically reconnect with backoff.
2. **"Failed to load config persistence" on Startup**:
   - Cause: First run (no `config_store.json` exists) or JSON syntax corruption.
   - Resolution: Normal on first run (initializes empty store). If corrupted, restore from backup or check permissions.
3. **OpenMFE Registration Retries**:
   - On boot, `config-server` attempts 15 registration pings to `web-interface` (port `5000`). If `web-interface` starts after `config-server`, registration succeeds once the web server becomes reachable.
