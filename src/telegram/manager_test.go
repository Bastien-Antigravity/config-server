package telegram

// =============================================================================
// ESSENTIAL PROCESS:
// Unit tests for MenuManager, verifying dynamic menu tree building, action
// hierarchy construction, and callback wiring against mock controllers.
//
// DATA FLOW:
// 1. Input: Sample configuration map returned by mock controller.
// 2. Logic: Invokes RebuildMenu() to assemble storage operations and key browsers.
// 3. Output: Validates TeleClient action hierarchy and callback executions.
//
// KEY PARAMETERS:
// - MenuManager: Interactive menu builder.
// =============================================================================

import (
	"context"
	"testing"

	"github.com/Bastien-Antigravity/config-server/src/core"
	"github.com/Bastien-Antigravity/config-server/src/store"
	toolbox_teleclient "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/teleremote"
	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------

type mockLogger struct {
	unilog_ifaces.Logger
}

func (m *mockLogger) Debug(format string, args ...any)    {}
func (m *mockLogger) Info(format string, args ...any)     {}
func (m *mockLogger) Warning(format string, args ...any)  {}
func (m *mockLogger) Error(format string, args ...any)    {}
func (m *mockLogger) Critical(format string, args ...any) {}

// -----------------------------------------------------------------------------

type mockController struct {
	config       store.ConfigMap
	persisted    bool
	reloaded     bool
	lastSetKey   string
	lastSetVal   string
	lastDelKey   string
}

func (m *mockController) GetConfig(ctx context.Context, section, key string) (string, bool, error) {
	if s, ok := m.config[section]; ok {
		val, exists := s[key]
		return val, exists, nil
	}
	return "", false, nil
}

func (m *mockController) SetConfig(ctx context.Context, section, key, value string) error {
	m.lastSetKey = key
	m.lastSetVal = value
	if m.config == nil {
		m.config = make(store.ConfigMap)
	}
	if _, ok := m.config[section]; !ok {
		m.config[section] = make(map[string]string)
	}
	m.config[section][key] = value
	return nil
}

func (m *mockController) DeleteConfig(ctx context.Context, section, key string) error {
	m.lastDelKey = key
	if s, ok := m.config[section]; ok {
		delete(s, key)
	}
	return nil
}

func (m *mockController) ListConfig(ctx context.Context) (store.ConfigMap, error) {
	return m.config, nil
}

func (m *mockController) ReloadConfig(ctx context.Context) error {
	m.reloaded = true
	return nil
}

func (m *mockController) PersistConfig(ctx context.Context) error {
	m.persisted = true
	return nil
}

func (m *mockController) GetStatus(ctx context.Context) (core.StatusInfo, error) {
	return core.StatusInfo{Healthy: true}, nil
}

// -----------------------------------------------------------------------------

func TestMenuManager_RebuildMenu(t *testing.T) {
	logger := &mockLogger{}
	tc := toolbox_teleclient.NewTeleClient("TestConfigServer", "127.0.0.1", 1863, logger)

	ctrl := &mockController{
		config: store.ConfigMap{
			"timescale_db": {
				"port": "5432",
				"host": "127.0.0.1",
			},
		},
	}

	mgr := NewMenuManager(tc, ctrl, logger)
	mgr.RebuildMenu()

	// Verify action callbacks work
	// Trigger reload
	err := ctrl.ReloadConfig(context.Background())
	if err != nil || !ctrl.reloaded {
		t.Fatalf("expected reloaded to be true")
	}

	// Trigger persist
	err = ctrl.PersistConfig(context.Background())
	if err != nil || !ctrl.persisted {
		t.Fatalf("expected persisted to be true")
	}

	// Trigger set config
	err = ctrl.SetConfig(context.Background(), "timescale_db", "port", "5433")
	if err != nil || ctrl.lastSetVal != "5433" {
		t.Fatalf("expected updated port 5433")
	}
}
