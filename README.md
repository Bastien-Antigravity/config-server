---
microservice: config-server
type: repository
status: active
language: go
tags:
- '#service/config-server'
- '#domain/configuration'
- '#domain/networking'
- '#zone/3-fleet'
- '#type/repository'
- '#state/active'
---

# Config Server

Config Server is a lightweight, high-performance configuration management service designed for real-time applications. It provides a centralized store for configuration data, capable of persisting changes to disk and broadcasting updates to connected clients instantly.

## Features

- **Dynamic Service Registry & Discovery**: Automatically maps incoming client nodes and their advertised listening addresses (`ServiceAddress`). Real-time address mappings (`service_addresses`) are broadcast across the fleet via `BROADCAST_REGISTRY` to enable dynamic service-to-service routing without static configuration.
- **Persistent Storage**: Backed by a human-readable JSON file (default: `config_store.json`) with debounced 5-second atomic writes.
- **Read-Fast Architecture**: Uses `sync.RWMutex` with a Copy-On-Write (COW) strategy for zero-allocation, high-concurrency reads.
- **Reliable Transport**: Built on **[safe-socket](https://github.com/Bastien-Antigravity/safe-socket)** for robust, framed TCP communication with handshake (Identity) support.
- **Generic Protobuf Messaging**: Uses the `ConfigMsg` schema from `distributed-config` for high-performance binary serialization.
- **Standalone Mode**: Can run with local configuration or integrate into a distributed system.
 
## 🛡️ Feature Specs & Governance (BDD)
The behavior of this microservice is governed by strict specifications in the **Obsidian Brain**:
- **Handshake & Identity**: [FEAT-001: Mutual Identity Handshake](../obsidian-brain/02-Business-BDD/02-Behavior-Specs/config-server/FEAT-001-Handshake-Identity.md)
- **Atomic State Swap**: [FEAT-002: Atomic State Swap](../obsidian-brain/02-Business-BDD/02-Behavior-Specs/config-server/FEAT-002-Atomic-State-Swap.md)
- **Broadcast Propagation**: [FEAT-003: Broadcast Propagation](../obsidian-brain/02-Business-BDD/02-Behavior-Specs/config-server/FEAT-003-Broadcast-Propagation.md)
- **Persistence Safety**: [FEAT-004: Configuration Persistence & Recovery](../obsidian-brain/02-Business-BDD/02-Behavior-Specs/config-server/FEAT-004-Persistence-Safety.md)

## Architecture

For a detailed deep-dive into the system design, components, and data flow, please refer to [ARCHITECTURE.md](ARCHITECTURE.md).

The project is structured into three main layers:

- **Network Layer**: Leverages `safe-socket` lib for connection management, framing (`ReadMessage`), and handshakes.
- **Core Logic** (`src/core`, `src/server`): Manages request processing (`ProcessRequest`), client mailbox lifecycle, and background persistence.
- **Storage Layer** (`src/store`): Provides a thread-safe, in-memory configuration store using COW for maximum performance.

## Getting Started

### Prerequisites

- Go 1.25 or higher

### Installation

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd config-server
   ```

2. Build the server:
   ```bash
   go build -o bin/config-server ./cmd/config-server
   ```

### Usage

Run the server using the compiled executable. Network capabilities (ports, IPs) are resolved dynamically via `microservice-toolbox` and `distributed-config` (`standalone.yaml -> ../docker-deployment/modes/local/config/native.yaml`):

- **TCP Sync Protocol**: Port `3306` (`appConfig.GetListenAddr("config_server")`) — framed TCP distribution via `safe-socket`
- **gRPC Shadow Port**: Port `3307` (`appConfig.GetGRPCAddr("config_server")`) — management and remote sync
- **REST & OpenMFE**: Port `3308` (`appConfig.GetRESTAddr("config_server")`) — HTTP REST endpoints and OpenMFE micro-frontend host

```bash
# Run locally with default persistence store (config_store.json)
./bin/config-server

# Run with custom persistence store path
./bin/config-server --store /path/to/custom_store.json
```

## API Protocol

The server communicates using the **Safe-Socket** `tcp-hello` profile:

1.  **Handshake**: Clients must perform a handshake sending their Identity (Name/Group) upon connection.
2.  **Framing**: Handled natively via `ReadMessage()`.
3.  **Payload**: The body is a serialized `ConfigMsg` Protocol Buffer message.

### Supported Operations

- **Get Config**: Retrieve the current in-memory configuration (`GET_SYNC`).
- **Update Config**: Update specific sections/keys. Triggers an async broadcast and debounced persistence (`PUT_SYNC`).
- **Service Registration**: Triggered seamlessly on socket handshake (`BROADCAST_REGISTRY`).

## Project Structure

```
config-server/
├── cmd/
│   └── config-server/  # Application entry point
├── src/
│   ├── core/           # Request processing and ConfigController business logic
│   ├── grpc_control/   # Shadow Port gRPC management service & proto contracts
│   ├── helpers/        # Utility merge helpers (ApplyUpdates)
│   ├── rest/           # HTTP REST endpoints & OpenMFE Web Component bundle
│   ├── server/         # TCP daemon, mailbox pattern, and safe connection lifecycle
│   ├── store/          # In-memory COW store and debounced atomic persistence manager
│   └── telegram/       # Tele-Remote client integration and interactive menu manager
├── quick-overview/     # Modular system architectural summaries and test playbooks
└── config_store.json   # Default persistence file
```
