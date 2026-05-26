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
---

# Config Server

Config Server is a lightweight, high-performance configuration management service designed for real-time applications. It provides a centralized store for configuration data, capable of persisting changes to disk and broadcasting updates to connected clients instantly.

## Features

- **Real-Time Updates**: Automatically propagates configuration changes to all connected clients via non-blocking mailboxes.
- **Service Registry**: Automatically maps and broadcasts tracking payloads containing all active nodes connected via safe-socket identifiers.
- **Persistent Storage**: Backed by a human-readable JSON file (default: `config_store.json`) with debounced 5-second atomic writes.
- **Read-Fast Architecture**: Uses `sync.RWMutex` with a Copy-On-Write (COW) strategy for zero-allocation, high-concurrency reads.
- **Reliable Transport**: Built on **[safe-socket](https://github.com/Bastien-Antigravity/safe-socket)** for robust, framed TCP communication with handshake (Identity) support.
- **Generic Protobuf Messaging**: Uses the `ConfigMsg` schema from `distributed-config` for high-performance binary serialization.
- **Standalone Mode**: Can run with local configuration or integrate into a distributed system.
 
## 🛡️ Feature Specs & Governance (BDD)
The behavior of this microservice is governed by strict specifications in the **[[business-bdd-brain|Business-Specs Brain]]**:
- **Handshake & Identity**: [[FEAT-001-Handshake-Identity|FEAT-001: Mutual Identity Handshake]]
- **Atomic State Swap**: [[FEAT-002-Atomic-State-Swap|FEAT-002: Read-Fast COW updates]]
- **Broadcast Propagation**: [[FEAT-003-Broadcast-Propagation|FEAT-003: Non-blocking Mailbox Broadcasting]]
- **Persistence Safety**: [[FEAT-004-Persistence-Safety|FEAT-004: Debounced Background Persistence]]

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
   go build -o config-server cmd/config-server/main.go
   ```

### Usage

Run the server using the compiled executable. Note that network capabilities (port/IP) are determined by the `distributed-config` setup, but defaults can be overridden or files specified via flags.

```bash
./config-server -port 1026 -config config_store.json
```

**Flags:**
- `-port`: Server listening port (default: "1026").
- `-config`: Path to the persistent configuration file (default: "config_store.json").

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
│   ├── config-server/  # Application entry point
│   ├── test/           # Test utilities
│   └── test_client/    # Simple test client
├── src/
│   ├── server/         # Server lifecycle, mailbox pattern, and dual-loop handler
│   ├── store/          # In-memory COW store and debounced persistence manager
│   ├── core/           # Request processing business logic
│   ├── helpers/        # Utility functions (Protobuf <-> Map conversion)
│   └── interfaces/     # Shared interfaces (Logger, etc.)
└── config_store.json   # Default persistence file
```
