package grpc_control

// =============================================================================
// ESSENTIAL PROCESS:
// Unit tests for ControlServiceImpl gRPC handler, asserting request translation,
// controller delegation, and protobuf response payload validation.
//
// DATA FLOW:
// 1. Input: Protobuf gRPC request messages.
// 2. Logic: Invokes ControlServiceImpl methods backed by mockConfigController and mockLogger.
// 3. Output: Validates response message fields, success flags, and error conditions.
//
// KEY PARAMETERS:
// - ControlServiceImpl: gRPC service handler.
// - mockConfigController: Mock implementation of core.ConfigController.
// =============================================================================

import (
	"context"
	"errors"
	"testing"

	"github.com/Bastien-Antigravity/config-server/src/core"
	"github.com/Bastien-Antigravity/config-server/src/store"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"
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

type mockConfigController struct {
	config    store.ConfigMap
	status    core.StatusInfo
	setErr    error
	reloadErr error
	saveErr   error
}

func (m *mockConfigController) GetConfig(ctx context.Context, section, key string) (string, bool, error) {
	if sec, ok := m.config[section]; ok {
		val, exists := sec[key]
		return val, exists, nil
	}
	return "", false, nil
}

func (m *mockConfigController) SetConfig(ctx context.Context, section, key, value string) error {
	if m.setErr != nil {
		return m.setErr
	}
	if m.config == nil {
		m.config = make(store.ConfigMap)
	}
	if _, ok := m.config[section]; !ok {
		m.config[section] = make(map[string]string)
	}
	m.config[section][key] = value
	return nil
}

func (m *mockConfigController) DeleteConfig(ctx context.Context, section, key string) error {
	if sec, ok := m.config[section]; ok {
		delete(sec, key)
	}
	return nil
}

func (m *mockConfigController) ListConfig(ctx context.Context) (store.ConfigMap, error) {
	return m.config, nil
}

func (m *mockConfigController) ReloadConfig(ctx context.Context) error {
	return m.reloadErr
}

func (m *mockConfigController) PersistConfig(ctx context.Context) error {
	return m.saveErr
}

func (m *mockConfigController) GetStatus(ctx context.Context) (core.StatusInfo, error) {
	return m.status, nil
}

// -----------------------------------------------------------------------------

func TestControlServiceImpl_GetConfig(t *testing.T) {
	ctrl := &mockConfigController{
		config: store.ConfigMap{
			"nats": {"port": "4222"},
		},
	}
	svc := NewControlService(ctrl, &mockLogger{})

	// 1. Existing key
	resp, err := svc.GetConfig(context.Background(), &GetConfigRequest{
		Section: "nats",
		Key:     "port",
	})
	if err != nil || !resp.Success || resp.Value != "4222" {
		t.Fatalf("unexpected response: %+v, err: %v", resp, err)
	}

	// 2. Non-existent key
	resp, err = svc.GetConfig(context.Background(), &GetConfigRequest{
		Section: "nats",
		Key:     "missing",
	})
	if err != nil || resp.Success || resp.Value != "" {
		t.Fatalf("expected not-found response, got: %+v", resp)
	}
}

// -----------------------------------------------------------------------------

func TestControlServiceImpl_SetConfig(t *testing.T) {
	ctrl := &mockConfigController{}
	svc := NewControlService(ctrl, &mockLogger{})

	// 1. Success
	resp, err := svc.SetConfig(context.Background(), &SetConfigRequest{
		Section: "app",
		Key:     "env",
		Value:   "prod",
	})
	if err != nil || !resp.Success {
		t.Fatalf("expected successful set, got: %+v", resp)
	}
	if ctrl.config["app"]["env"] != "prod" {
		t.Fatalf("controller state not updated")
	}

	// 2. Error handling
	ctrl.setErr = errors.New("disk full")
	resp, err = svc.SetConfig(context.Background(), &SetConfigRequest{
		Section: "app",
		Key:     "env",
		Value:   "fail",
	})
	if err != nil || resp.Success || resp.ErrorCode != "UPDATE_FAILED" {
		t.Fatalf("expected UPDATE_FAILED error response, got: %+v", resp)
	}
}

// -----------------------------------------------------------------------------

func TestControlServiceImpl_ListConfig(t *testing.T) {
	ctrl := &mockConfigController{
		config: store.ConfigMap{
			"db": {"host": "localhost"},
		},
	}
	svc := NewControlService(ctrl, &mockLogger{})

	resp, err := svc.ListConfig(context.Background(), &ListConfigRequest{})
	if err != nil || !resp.Success || resp.JsonConfig == "" {
		t.Fatalf("expected successful list, got: %+v", resp)
	}
}

// -----------------------------------------------------------------------------

func TestControlServiceImpl_ReloadAndPersist(t *testing.T) {
	ctrl := &mockConfigController{}
	svc := NewControlService(ctrl, &mockLogger{})

	// Reload success
	resp, err := svc.ReloadConfig(context.Background(), &ReloadConfigRequest{})
	if err != nil || !resp.Success {
		t.Fatalf("expected successful reload, got: %+v", resp)
	}

	// Persist success
	resp, err = svc.PersistConfig(context.Background(), &PersistConfigRequest{})
	if err != nil || !resp.Success {
		t.Fatalf("expected successful persist, got: %+v", resp)
	}
}

// -----------------------------------------------------------------------------

func TestControlServiceImpl_GetStatus(t *testing.T) {
	ctrl := &mockConfigController{
		status: core.StatusInfo{
			Healthy:       true,
			Status:        "Operational",
			Version:       "0.0.1",
			ActiveClients: 3,
			ClientNames:   []string{"client1", "client2", "client3"},
		},
	}
	svc := NewControlService(ctrl, &mockLogger{})

	resp, err := svc.GetStatus(context.Background(), &GetStatusRequest{})
	if err != nil || !resp.Healthy || resp.ActiveClients != 3 {
		t.Fatalf("unexpected status response: %+v", resp)
	}
}
