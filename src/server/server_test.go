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
	"os"
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
func (m *mockLogger) Logon(format string, args ...any)    {}
func (m *mockLogger) Logout(format string, args ...any)   {}
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

	mb1 := newClientMailbox("worker-1", "127.0.0.1:9001", nil, 3)
	mb2 := newClientMailbox("worker-1", "127.0.0.1:9001", nil, 3)

	srv.addListener("worker-1", mb1)
	srv.addListener("worker-1", mb2)

	select {
	case <-mb1.done:
		// expected: mb1.done is closed when superseded
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected mb1.done to be closed, timed out")
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

// -----------------------------------------------------------------------------

func TestServer_ConcurrentBroadcastAndDisconnectNoPanic(t *testing.T) {
	srv := createTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup

	// Churn listeners (add, supersede, remove)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := "churn-worker"
			if id%2 == 0 {
				name = "churn-worker-even"
			}
			for {
				select {
				case <-ctx.Done():
					return
				default:
					mb := newClientMailbox(name, "127.0.0.1:8080", nil, 3)
					srv.addListener(name, mb)
					srv.removeListener(name, mb)
				}
			}
		}(i)
	}

	// Concurrent broadcasters hammering broadcastUpdate
	for i := 0; i < 8; i++ {
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
					srv.broadcastRegistry()
				}
			}
		}()
	}

	wg.Wait()
}

// -----------------------------------------------------------------------------

func TestServer_PersistenceRetryAndReloadErrorPropagation(t *testing.T) {
	srv := createTestServer(t)
	ctx := context.Background()

	// Point PersistenceManager at a path where the parent is a regular file so Save() fails
	blockerFile := filepath.Join(t.TempDir(), "blocker")
	if err := srv.Persistence.Save(store.ConfigMap{"init": {"k": "v"}}); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}

	// Create a PersistenceManager whose target file path is an existing directory (causes Save and Load to fail)
	badDir := t.TempDir()
	srv.Persistence = store.NewPersistenceManager(badDir, srv.Logger)

	// 1. TriggerSave + flushDirtyOnce: since Save fails, dirty flag must be re-set to true for retry
	srv.TriggerSave()
	srv.flushDirtyOnce()
	if !srv.dirty.Load() {
		t.Fatal("expected dirty flag to be restored to true when background persistence Save fails")
	}

	// 2. ReloadConfig must propagate error when Persistence.Load fails (e.g., target is a directory or corrupt JSON)
	_ = blockerFile
	if err := srv.ReloadConfig(ctx); err == nil {
		t.Fatal("expected ReloadConfig to return error when Persistence.Load fails")
	}
}

// -----------------------------------------------------------------------------

func TestServer_DeterministicRegistrySorting(t *testing.T) {
	srv := createTestServer(t)

	mbZ := newClientMailbox("z-service", "10.0.0.3:9000", nil, 16)
	mbA := newClientMailbox("a-service", "10.0.0.1:9000", nil, 16)
	mbM := newClientMailbox("m-service", "10.0.0.2:9000", nil, 16)

	srv.addListener("z-service", mbZ)
	srv.addListener("a-service", mbA)
	srv.addListener("m-service", mbM)

	names := srv.GetClientNames()
	if len(names) != 3 || names[0] != "a-service" || names[1] != "m-service" || names[2] != "z-service" {
		t.Fatalf("expected sorted client names [a-service m-service z-service], got %v", names)
	}
}

// -----------------------------------------------------------------------------

func TestServer_ListConfigSkipsNonScalarCapabilityStructures(t *testing.T) {
	srv := createTestServer(t)
	srv.AppConfig.Capabilities["nats"] = map[string]interface{}{
		"ip":              "127.0.0.1",
		"port":            "4222",
		"max_connections": 100,
		"servers":         []interface{}{"nats://127.0.0.1:4222"},
		"jetstream": map[string]interface{}{
			"enabled": true,
		},
	}
	srv.AppConfig.Capabilities["rag_engine"] = map[string]interface{}{
		"ip":   "127.0.0.1",
		"port": "8090",
		"dashboard": map[string]interface{}{
			"ip":   "127.0.0.1",
			"port": "8082",
		},
	}

	listed, err := srv.ListConfig(context.Background())
	if err != nil {
		t.Fatalf("ListConfig failed: %v", err)
	}
	if listed["nats"]["ip"] != "127.0.0.1" || listed["nats"]["max_connections"] != "100" {
		t.Fatalf("expected scalar nats fields in ListConfig, got %v", listed["nats"])
	}
	if _, hasServers := listed["nats"]["servers"]; hasServers {
		t.Fatalf("expected non-scalar slice 'servers' to be excluded from flat ConfigMap, got %q", listed["nats"]["servers"])
	}
	if _, hasJetStream := listed["nats"]["jetstream"]; hasJetStream {
		t.Fatalf("expected non-scalar map 'jetstream' to be excluded from flat ConfigMap, got %q", listed["nats"]["jetstream"])
	}
	if _, hasDashboard := listed["rag_engine"]["dashboard"]; hasDashboard {
		t.Fatalf("expected non-scalar map 'dashboard' to be excluded from flat ConfigMap, got %q", listed["rag_engine"]["dashboard"])
	}
}

// -----------------------------------------------------------------------------

func TestServer_ReloadPreservesBaselineAndFiltersWildcardIP(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "test.yaml")
	yamlContent := "common:\n  name: \"config-server\"\n  public_ip: \"127.0.0.1\"\ncapabilities:\n  config_server:\n    ip: \"0.0.0.0\"\n    port: \"3306\"\n  nats:\n    ip: \"127.0.0.1\"\n    port: \"4222\"\n"
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", yamlPath)

	logger := &mockLogger{}
	distCfg := distributed_config.New("standalone")
	if distCfg.Config == nil || len(distCfg.Config.Capabilities) == 0 {
		t.Fatalf("expected distributed_config.New(standalone) to load capabilities from %s", yamlPath)
	}
	appCfg := &utilconf.AppConfig{Config: distCfg}
	st := store.NewStore()
	pm := store.NewPersistenceManager(filepath.Join(t.TempDir(), "store.json"), logger)
	srv := NewServer(appCfg, logger, st, pm)

	// Persist a dynamic override on disk with version 12.
	if err := pm.SaveWithVersion(store.ConfigMap{"dynamic": {"k": "v"}}, 12); err != nil {
		t.Fatalf("SaveWithVersion failed: %v", err)
	}

	if err := srv.ReloadConfig(context.Background()); err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}

	snap := srv.Store.Get()
	if snap["dynamic"]["k"] != "v" {
		t.Fatalf("expected dynamic override preserved after reload, got %+v", snap)
	}
	if snap["nats"]["port"] != "4222" {
		t.Fatalf("expected baseline nats capability preserved after reload, got %+v", snap["nats"])
	}
	if _, hasWildcard := snap["config_server"]["ip"]; hasWildcard {
		t.Fatalf("expected wildcard bind IP 0.0.0.0 to be filtered from baseline, got %q", snap["config_server"]["ip"])
	}
	if srv.Store.GetVersion() != 12 {
		t.Fatalf("expected restored version 12 after ReloadConfig, got %d", srv.Store.GetVersion())
	}
}

// -----------------------------------------------------------------------------

func TestServer_PersistConfigDurableSave(t *testing.T) {
	srv := createTestServer(t)
	ctx := context.Background()

	if err := srv.SetConfig(ctx, "sec", "k", "val"); err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}
	if err := srv.PersistConfig(ctx); err != nil {
		t.Fatalf("PersistConfig failed: %v", err)
	}

	loaded, ver, err := srv.Persistence.LoadWithVersion()
	if err != nil {
		t.Fatalf("LoadWithVersion failed: %v", err)
	}
	if loaded["sec"]["k"] != "val" || ver == 0 {
		t.Fatalf("expected durable persisted config with non-zero version, got %+v ver=%d", loaded, ver)
	}

	// Unwritable directory target must return error synchronously from PersistConfig.
	badDir := t.TempDir()
	srv.Persistence = store.NewPersistenceManager(badDir, srv.Logger)
	if err := srv.PersistConfig(ctx); err == nil {
		t.Fatal("expected PersistConfig to return synchronous disk error when save fails")
	}
}

// -----------------------------------------------------------------------------

func TestServer_ConcurrentMutationsPreserveStrictBroadcastVersionOrder(t *testing.T) {
	srv := createTestServer(t)
	ctx := context.Background()

	const totalMutations = 32
	mb := newClientMailbox("order-verifier", "127.0.0.1:9991", nil, totalMutations+16)
	srv.addListener("order-verifier", mb)

	var wg sync.WaitGroup
	for i := 0; i < totalMutations; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = srv.SetConfig(ctx, "concurrent", "k", string(rune('A'+(idx%26))))
		}(i)
	}
	wg.Wait()

	close(mb.send)
	var lastVer int32
	var syncFrames int
	for raw := range mb.send {
		var msg schemas.ConfigMsg
		if err := proto.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("failed to unmarshal frame: %v", err)
		}
		if msg.Command != schemas.ConfigMsg_BROADCAST_SYNC {
			continue
		}
		syncFrames++
		if msg.Version <= lastVer {
			t.Fatalf("out-of-order broadcast frame detected: version %d arrived after %d", msg.Version, lastVer)
		}
		lastVer = msg.Version
	}
	if syncFrames != totalMutations {
		t.Fatalf("expected %d ordered BROADCAST_SYNC frames, got %d", totalMutations, syncFrames)
	}
}
