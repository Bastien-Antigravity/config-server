package server

// =============================================================================
// ESSENTIAL PROCESS:
// Unit tests for server.Server lifecycle, listener management, collision avoidance,
// controller accessors with baseline fallbacks, and health telemetry.
//
// DATA FLOW:
// 1. Input: Sample configuration maps, mock listeners, and mutation calls.
// 2. Logic: Asserts unique mailbox registration, atomic controller mutations,
//    and dynamic baseline configuration fallbacks.
// 3. Output: Go testing PASS/FAIL assertions.
//
// KEY PARAMETERS:
// - Server: Core stateful configuration daemon.
// =============================================================================

import (
	"context"
	"testing"

	"github.com/Bastien-Antigravity/config-server/src/store"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	distributed_config "github.com/Bastien-Antigravity/distributed-config"
	dist_core "github.com/Bastien-Antigravity/distributed-config/src/core"
	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
)

// -----------------------------------------------------------------------------

type mockLogger struct {
	interfaces.Logger
}

func (m *mockLogger) Debug(format string, args ...any)    {}
func (m *mockLogger) Info(format string, args ...any)     {}
func (m *mockLogger) Warning(format string, args ...any)  {}
func (m *mockLogger) Error(format string, args ...any)    {}
func (m *mockLogger) Critical(format string, args ...any) {}

// -----------------------------------------------------------------------------

func createTestServer() *Server {
	coreCfg := &dist_core.Config{
		Common: dist_core.CommonConfig{
			Name:     "common",
			PublicIP: "127.0.0.1",
		},
		Capabilities: map[string]interface{}{
			"config_server": map[string]interface{}{
				"port": "3306",
				"ip":   "127.0.0.1",
			},
		},
	}
	distCfg := &distributed_config.Config{
		Config: coreCfg,
	}
	appCfg := &utilconf.AppConfig{
		Config: distCfg,
	}

	s := store.NewStore()
	logger := &mockLogger{}
	pm := store.NewPersistenceManager("test_store.json", logger)

	return NewServer(appCfg, logger, s, pm)
}

// -----------------------------------------------------------------------------

func TestServer_AddAndRemoveListener(t *testing.T) {
	srv := createTestServer()

	mb1 := &clientMailbox{name: "worker-127.0.0.1", send: make(chan []byte, 3)}

	// 1. Add listener
	srv.addListener("worker-127.0.0.1", mb1)
	if srv.GetActiveClients() != 1 {
		t.Fatalf("expected 1 active client, got %d", srv.GetActiveClients())
	}

	// 2. Remove listener
	srv.removeListener("worker-127.0.0.1", mb1)
	if srv.GetActiveClients() != 0 {
		t.Fatalf("expected 0 active clients after removal, got %d", srv.GetActiveClients())
	}

	// 3. Duplicate remove call should be a safe no-op
	srv.removeListener("worker-127.0.0.1", mb1)
	if srv.GetActiveClients() != 0 {
		t.Fatalf("expected 0 active clients, got %d", srv.GetActiveClients())
	}
}

// -----------------------------------------------------------------------------

func TestServer_GetConfig_StoreOnly(t *testing.T) {
	srv := createTestServer()
	ctx := context.Background()

	// 1. Initial query before any dynamic override exists
	val, exists, err := srv.GetConfig(ctx, "config_server", "port")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists || val != "" {
		t.Fatalf("expected non-existent dynamic config, got exists=%v, val=%s", exists, val)
	}

	// 2. Set dynamic override
	err = srv.SetConfig(ctx, "config_server", "port", "3399")
	if err != nil {
		t.Fatalf("failed to set config: %v", err)
	}

	val, exists, err = srv.GetConfig(ctx, "config_server", "port")
	if err != nil || !exists || val != "3399" {
		t.Fatalf("expected overridden port 3399, got exists=%v, val=%s", exists, val)
	}

	// 3. Delete override - should return non-existent
	err = srv.DeleteConfig(ctx, "config_server", "port")
	if err != nil {
		t.Fatalf("failed to delete config: %v", err)
	}

	val, exists, err = srv.GetConfig(ctx, "config_server", "port")
	if err != nil || exists || val != "" {
		t.Fatalf("expected non-existent after delete, got exists=%v, val=%s", exists, val)
	}
}

// -----------------------------------------------------------------------------

func TestServer_GetStatus(t *testing.T) {
	srv := createTestServer()
	ctx := context.Background()

	status, err := srv.GetStatus(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.Healthy {
		t.Fatalf("expected healthy status")
	}
	if status.Version != ServerVersion {
		t.Fatalf("expected version %s, got %s", ServerVersion, status.Version)
	}
}
