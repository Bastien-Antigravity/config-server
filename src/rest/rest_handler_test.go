package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bastien-Antigravity/config-server/src/core"
	"github.com/Bastien-Antigravity/config-server/src/store"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

type mockLogger struct{}

func (m *mockLogger) Debug(format string, args ...any)                           {}
func (m *mockLogger) Info(format string, args ...any)                            {}
func (m *mockLogger) Warning(format string, args ...any)                         {}
func (m *mockLogger) Error(format string, args ...any)                           {}
func (m *mockLogger) Critical(format string, args ...any)                        {}
func (m *mockLogger) Stream(format string, args ...any)                          {}
func (m *mockLogger) Logon(format string, args ...any)                           {}
func (m *mockLogger) Logout(format string, args ...any)                          {}
func (m *mockLogger) Trade(format string, args ...any)                           {}
func (m *mockLogger) Schedule(format string, args ...any)                        {}
func (m *mockLogger) Report(format string, args ...any)                          {}
func (m *mockLogger) GetNotifQueue() <-chan *interfaces.NotifMessage             { return nil }
func (m *mockLogger) SetLocalNotifQueue(notifChan chan *interfaces.NotifMessage) {}
func (m *mockLogger) Log(level interfaces.Level, format string, args ...any)     {}
func (m *mockLogger) SetLevel(level interfaces.Level)                            {}
func (m *mockLogger) GetLevel() interfaces.Level                                 { return interfaces.LevelInfo }
func (m *mockLogger) SetCallerSkip(skip int)                                     {}
func (m *mockLogger) SetMetadata(metadata map[string]string)                     {}
func (m *mockLogger) AddMetadata(key, value string)                              {}
func (m *mockLogger) Close()                                                     {}

type mockControl struct {
	config         store.ConfigMap
	status         core.StatusInfo
	setRequests    []struct{ section, key, value string }
	reloadCalls    int
	persistCalls   int
	deleteRequests []struct{ section, key string }
}

func (m *mockControl) GetConfig(ctx context.Context, section, key string) (string, bool, error) {
	if sectionMap, ok := m.config[section]; ok {
		value, exists := sectionMap[key]
		return value, exists, nil
	}
	return "", false, nil
}

func (m *mockControl) SetConfig(ctx context.Context, section, key, value string) error {
	m.setRequests = append(m.setRequests, struct{ section, key, value string }{section, key, value})
	return nil
}

func (m *mockControl) DeleteConfig(ctx context.Context, section, key string) error {
	m.deleteRequests = append(m.deleteRequests, struct{ section, key string }{section, key})
	return nil
}

func (m *mockControl) ListConfig(ctx context.Context) (store.ConfigMap, error) {
	return m.config, nil
}

func (m *mockControl) ReloadConfig(ctx context.Context) error {
	m.reloadCalls++
	return nil
}

func (m *mockControl) PersistConfig(ctx context.Context) error {
	m.persistCalls++
	return nil
}

func (m *mockControl) GetStatus(ctx context.Context) (core.StatusInfo, error) {
	return m.status, nil
}

func newTestHandler(t *testing.T, control *mockControl) http.Handler {
	t.Helper()
	return NewRESTHandler(control, &mockLogger{}).Handler()
}

func TestHandlerServesStaticMFEAndCORSPreflight(t *testing.T) {
	handler := newTestHandler(t, &mockControl{})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/config/list", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected OPTIONS status 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard CORS origin, got %q", got)
	}

	// Test new MFE loader path
	req = httptest.NewRequest(http.MethodGet, "/static/js/mfe-loader.js", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected static MFE loader status 200, got %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("config-server-mfe")) {
		t.Fatalf("expected static MFE loader body to contain custom element registration")
	}

	// Test legacy MFE path
	req = httptest.NewRequest(http.MethodGet, "/static/mfe.js", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected legacy MFE status 200, got %d", rr.Code)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("config-server-mfe")) {
		t.Fatalf("expected legacy MFE body to contain custom element registration")
	}
}

func TestHandlerConfigEndpoints(t *testing.T) {
	control := &mockControl{
		config: store.ConfigMap{
			"timescale_db": map[string]string{"user": "dbuser"},
		},
		status: core.StatusInfo{
			Healthy:       true,
			Status:        "Running",
			Version:       "test",
			Timestamp:     123,
			ActiveClients: 2,
			ClientNames:   []string{"web-interface", "market-observer"},
		},
	}
	handler := newTestHandler(t, control)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config/list", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", rr.Code)
	}
	var listResp httpListResponse
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	if !listResp.Success || listResp.JsonConfig == "" {
		t.Fatalf("expected successful list response with json_config, got %+v", listResp)
	}

	body := bytes.NewBufferString(`{"section":"timescale_db","key":"password","value":"secret"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/config/set", body)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected set status 200, got %d", rr.Code)
	}
	if len(control.setRequests) != 1 || control.setRequests[0].key != "password" {
		t.Fatalf("expected set request to be recorded, got %+v", control.setRequests)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/config/reload", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || control.reloadCalls != 1 {
		t.Fatalf("expected reload status 200 and one call, got status=%d calls=%d", rr.Code, control.reloadCalls)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/config/persist", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || control.persistCalls != 1 {
		t.Fatalf("expected persist status 200 and one call, got status=%d calls=%d", rr.Code, control.persistCalls)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status endpoint status 200, got %d", rr.Code)
	}
	var statusResp httpStatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&statusResp); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
	}
	if !statusResp.Healthy || statusResp.ActiveClients != 2 {
		t.Fatalf("unexpected status response: %+v", statusResp)
	}
}

func TestHandlerRejectsWrongMethods(t *testing.T) {
	handler := newTestHandler(t, &mockControl{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/config/list"},
		{http.MethodGet, "/api/v1/config/reload"},
		{http.MethodGet, "/api/v1/config/persist"},
		{http.MethodPost, "/api/v1/status"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s: expected 405, got %d", tc.method, tc.path, rr.Code)
		}
	}
}
