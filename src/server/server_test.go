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
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/store"

	distributed_config "github.com/Bastien-Antigravity/distributed-config"
	dist_core "github.com/Bastien-Antigravity/distributed-config/src/core"
	schemas "github.com/Bastien-Antigravity/distributed-config/src/schemas"
	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"

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

func createTestServer(t *testing.T) *Server {
	t.Helper()
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
	pm := store.NewPersistenceManager(filepath.Join(t.TempDir(), "test_store.json"), logger)

	return NewServer(appCfg, logger, s, pm)
}

// -----------------------------------------------------------------------------

func TestServer_AddAndRemoveListener(t *testing.T) {
	srv := createTestServer(t)

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
	srv := createTestServer(t)
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
	srv := createTestServer(t)
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
	srv := createTestServer(t)
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
	srv := createTestServer(t)

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
	srv := createTestServer(t)
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

// -----------------------------------------------------------------------------

func TestServer_OverflowElevatesToFullRefresh(t *testing.T) {
	srv := createTestServer(t)

	mb := &clientMailbox{
		name: "slow-worker",
		// Mailbox with capacity 1 to easily simulate overflow
		send: make(chan []byte, 1),
	}
	// Fill the buffer so next send overflows
	mb.send <- []byte("initial")

	srv.addListener("slow-worker", mb)

	// Broadcast an update: buffer is full, so message is dropped and needsRefresh is flagged
	srv.BroadcastConfig()

	if !mb.needsRefresh.Load() {
		t.Fatalf("expected needsRefresh to be set on mailbox overflow")
	}

	// Drain the buffer
	<-mb.send

	// Next broadcast: should be elevated to FULL_REFRESH
	srv.BroadcastConfig()

	select {
	case raw := <-mb.send:
		var msg schemas.ConfigMsg
		if err := proto.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if msg.Command != schemas.ConfigMsg_FULL_REFRESH {
			t.Fatalf("expected overflow recovery to elevate to FULL_REFRESH, got %v", msg.Command)
		}
		if mb.needsRefresh.Load() {
			t.Fatalf("expected needsRefresh to be cleared after sending FULL_REFRESH")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for FULL_REFRESH recovery message")
	}
}

// -----------------------------------------------------------------------------

func TestServer_SaveSyncAndVersionIncrement(t *testing.T) {
	srv := createTestServer(t)
	ctx := context.Background()

	// 1. Initial SetConfig
	err := srv.SetConfig(ctx, "cluster", "nodes", "1")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}
	v1 := srv.Store.GetVersion()

	// 2. Second SetConfig
	err = srv.SetConfig(ctx, "cluster", "nodes", "2")
	if err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}
	v2 := srv.Store.GetVersion()
	if v2 != v1+1 {
		t.Fatalf("expected version %d, got %d", v1+1, v2)
	}

	// 3. Verify OCC conflict on SetConfigWithOCC (expecting stale v1 when current is v2)
	err = srv.SetConfigWithOCC(ctx, "cluster", "nodes", "4", v1)
	if err == nil {
		t.Fatalf("expected OCC version mismatch error")
	}

	// 4. Verify OCC success on SetConfigWithOCC with matching version v2
	err = srv.SetConfigWithOCC(ctx, "cluster", "nodes", "4", v2)
	if err != nil {
		t.Fatalf("unexpected OCC error: %v", err)
	}
	if srv.Store.GetVersion() != v2+1 {
		t.Fatalf("expected version %d, got %d", v2+1, srv.Store.GetVersion())
	}

	// 5. Verify SaveSync commits cleanly
	if err := srv.SaveSync(); err != nil {
		t.Fatalf("SaveSync failed: %v", err)
	}
}
