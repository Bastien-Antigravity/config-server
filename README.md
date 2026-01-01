# Config Server

Config Server is a lightweight, high-performance configuration management service designed for real-time applications. It provides a centralized store for configuration data, capable of persisting changes to disk and broadcasting updates to connected clients instantly.

## Features

- **Real-Time Updates**: Automatically propagates configuration changes to all connected clients.
- **Persistent Storage**: Backed by a human-readable JSON file (default: `config_store.json`).
- **Atomic Operations**: Uses lock-free atomic pointers for thread-safe, high-concurrency reads.
- **Reliable Transport**: Built on **[safe-socket](https://github.com/Bastien-Antigravity/safe-socket)** for robust, framed TCP communication with handshake (Identity) support.
- **Protobuf Messaging**: Uses Protocol Buffers (via `distributed-config`) for structured and efficient communication.
- **Standalone Mode**: Can run with local configuration or integrate into a distributed system.

## Architecture

For a detailed deep-dive into the system design, components, and data flow, please refer to [ARCHITECTURE.md](ARCHITECTURE.md).

The project is structured into three main layers:

- **Network Layer**: Leverages `safe-socket` lib for connection management, framing, and handshakes. Connection logic is handled in `src/server`.
- **Core Logic** (`src/core`, `src/server`): Manages request processing (`ProcessRequest`), client lifecycle, and broadcasting.
- **Storage Layer** (`src/store`): Provides an atomic, in-memory configuration store with disk persistence.

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
   go build -o server.exe cmd/main/main.go
   ```

### Usage

Run the server using the compiled executable. Note that network capabilities (port/IP) are determined by the `distributed-config` setup, but defaults can be overridden or files specified via flags.

```bash
./server.exe -port 1026 -config config_store.json
```

**Flags:**
- `-port`: Server listening port (default: "1026"). *Note: Actual binding depends on distributed-config capabilities profile.*
- `-config`: Path to the persistent configuration file (default: "config_store.json").

## API Protocol

The server communicates using the **Safe-Socket** `tcp-hello` profile:

1.  **Handshake**: Clients must perform a handshake sending their Identity (Name/Group) upon connection.
2.  **Framing**: Messages are Length-Prefixed (Big-Endian 4-byte).
3.  **Payload**: The body is a serialized `ConfigMsg` Protocol Buffer message (from `distributed-config`).

### Supported Operations

- **Get Config**: Retrieve the current in-memory configuration (`get_mem_config`).
- **Update Config**: Update specific sections/keys. Triggers a broadcast to all listeners (`update_mem_config`).
- **Dump Config**: Force a save of the current memory state to disk (`dump_mem_config`).
- **Subscribe**: Clients can register as listeners (`add_config_listener`) to receive `propagate_mem_config` messages on changes.

## Project Structure

```
config-server/
├── cmd/
│   ├── main/           # Application entry point
│   ├── test/           # Test utilities
│   └── test_client/    # Simple test client
├── src/
│   ├── server/         # Server lifecycle and connection handling
│   ├── store/          # In-memory atomic store and persistence
│   ├── core/           # Request processing business logic
│   ├── helpers/        # Utility functions (Protobuf <-> Map conversion)
│   └── interfaces/     # Shared interfaces (Logger, etc.)
└── config_store.json   # Default persistence file
```
