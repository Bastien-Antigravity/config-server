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
	"sync"
	"testing"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/store"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	distributed_config "github.com/Bastien-Antigravity/distributed-config"
	dist_core "github.com/Bastien-Antigravity/distributed-config/src/core"
	schemas "github.com/Bastien-Antigravity/distributed-config/src/schemas"
	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	"google.golang.org/protobuf/proto"
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

// -----------------------------------------------------------------------------

func TestServer_ConcurrentOnUpdateAndBroadcast(t *testing.T) {
	srv := createTestServer()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					srv.SetOnUpdate(func() {})
				}
			}
		}()
	}

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
					srv.BroadcastConfig()
					srv.BroadcastFullRefresh()
				}
			}
		}()
	}

	wg.Wait()
}

// -----------------------------------------------------------------------------

func TestServer_SuperseededListenerClosesMailbox(t *testing.T) {
	srv := createTestServer()

	mb1 := &clientMailbox{name: "worker-1", send: make(chan []byte, 3)}
	mb2 := &clientMailbox{name: "worker-1", send: make(chan []byte, 3)}

	srv.addListener("worker-1", mb1)
	srv.addListener("worker-1", mb2)

	select {
	case _, ok := <-mb1.send:
		if ok {
			t.Fatalf("expected mb1.send to be closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected mb1.send to be closed, timed out")
	}

	select {
	case mb2.send <- []byte("test"):
		// ok
	default:
		t.Fatalf("expected mb2.send to be open")
	}
}

// -----------------------------------------------------------------------------

func TestServer_BroadcastFullRefreshOnDelete(t *testing.T) {
	srv := createTestServer()
	ctx := context.Background()

	mb := &clientMailbox{name: "worker-test", send: make(chan []byte, 64)}
	srv.addListener("worker-test", mb)

	// Drain initial broadcastRegistry triggered by addListener
	select {
	case <-mb.send:
	case <-time.After(500 * time.Millisecond):
	}

	// SetConfig must broadcast BROADCAST_SYNC
	err := srv.SetConfig(ctx, "app", "mode", "prod")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}

	var syncMsg schemas.ConfigMsg
	select {
	case raw := <-mb.send:
		if err := proto.Unmarshal(raw, &syncMsg); err != nil {
			t.Fatalf("failed to unmarshal msg: %v", err)
		}
		if syncMsg.Command != schemas.ConfigMsg_BROADCAST_SYNC {
			t.Fatalf("expected BROADCAST_SYNC for SetConfig, got %v", syncMsg.Command)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for SetConfig broadcast")
	}

	// DeleteConfig must broadcast FULL_REFRESH
	err = srv.DeleteConfig(ctx, "app", "mode")
	if err != nil {
		t.Fatalf("DeleteConfig failed: %v", err)
	}

	var refreshMsg schemas.ConfigMsg
	select {
	case raw := <-mb.send:
		if err := proto.Unmarshal(raw, &refreshMsg); err != nil {
			t.Fatalf("failed to unmarshal msg: %v", err)
		}
		if refreshMsg.Command != schemas.ConfigMsg_FULL_REFRESH {
			t.Fatalf("expected FULL_REFRESH for DeleteConfig, got %v", refreshMsg.Command)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for DeleteConfig broadcast")
	}
}
