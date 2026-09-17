package core

// =============================================================================
// ESSENTIAL PROCESS:
// Unit tests for core.ProcessRequest, verifying command dispatching, protobuf
// unmarshaling, atomic PUT_SYNC updates, broadcast triggering, and error handling.
//
// DATA FLOW:
// 1. Input: Serialized ConfigMsg protobuf frames.
// 2. Logic: Invokes ProcessRequest with test Store and mock broadcast/save callbacks.
// 3. Output: Validates response Command types, JSON payloads, and callback invocations.
//
// KEY PARAMETERS:
// - ProcessRequest: Core request dispatcher function.
// =============================================================================

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/store"
	config "github.com/Bastien-Antigravity/distributed-config/src/schemas"

	"google.golang.org/protobuf/proto"
)

// -----------------------------------------------------------------------------

func TestProcessRequest_GetSyncAndFullRefresh(t *testing.T) {
	s := store.NewStore()
	s.Replace(store.ConfigMap{
		"timescale_db": {"host": "127.0.0.1", "port": "5432"},
	})

	commands := []config.ConfigMsg_Cmd{
		config.ConfigMsg_GET_SYNC,
		config.ConfigMsg_FULL_REFRESH,
	}

	for _, cmd := range commands {
		req := &config.ConfigMsg{
			Command: cmd,
		}
		data, err := proto.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal request: %v", err)
		}

		resp, err := ProcessRequest(data, s, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatalf("expected non-nil response")
		}
		if resp.Command != cmd {
			t.Fatalf("expected response command %v, got %v", cmd, resp.Command)
		}

		var payload store.ConfigMap
		if err := json.Unmarshal(resp.Payload, &payload); err != nil {
			t.Fatalf("failed to unmarshal response payload: %v", err)
		}
		if payload["timescale_db"]["port"] != "5432" {
			t.Fatalf("unexpected payload value: %+v", payload)
		}
	}
}

// -----------------------------------------------------------------------------

func TestProcessRequest_PutSync_Success(t *testing.T) {
	s := store.NewStore()
	s.Replace(store.ConfigMap{
		"app": {"env": "dev"},
	})

	updates := store.ConfigMap{
		"app": {"env": "prod", "version": "2.0"},
	}
	updateBytes, _ := json.Marshal(updates)

	req := &config.ConfigMsg{
		Command: config.ConfigMsg_PUT_SYNC,
		Payload: updateBytes,
	}
	data, _ := proto.Marshal(req)

	var mu sync.Mutex
	var broadcastCmd config.ConfigMsg_Cmd
	var broadcastPayload []byte
	saveTriggered := false

	broadcastFn := func(cmd config.ConfigMsg_Cmd, payload []byte) {
		mu.Lock()
		defer mu.Unlock()
		broadcastCmd = cmd
		broadcastPayload = payload
	}
	triggerSaveFn := func() {
		mu.Lock()
		defer mu.Unlock()
		saveTriggered = true
	}

	resp, err := ProcessRequest(data, s, nil, broadcastFn, triggerSaveFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Command != config.ConfigMsg_ACK {
		t.Fatalf("expected ACK response, got %+v", resp)
	}

	// Verify store state updated
	if s.Get()["app"]["env"] != "prod" || s.Get()["app"]["version"] != "2.0" {
		t.Fatalf("store was not updated: %+v", s.Get())
	}

	// Wait briefly for asynchronous broadcast goroutine
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !saveTriggered {
		t.Fatalf("expected triggerSave to be called")
	}
	if broadcastCmd != config.ConfigMsg_BROADCAST_SYNC {
		t.Fatalf("expected broadcast command %v, got %v", config.ConfigMsg_BROADCAST_SYNC, broadcastCmd)
	}
	if len(broadcastPayload) == 0 {
		t.Fatalf("expected non-empty broadcast payload")
	}
}

// -----------------------------------------------------------------------------

func TestProcessRequest_PutSync_InvalidJSON(t *testing.T) {
	s := store.NewStore()

	req := &config.ConfigMsg{
		Command: config.ConfigMsg_PUT_SYNC,
		Payload: []byte("invalid-json"),
	}
	data, _ := proto.Marshal(req)

	resp, err := ProcessRequest(data, s, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.Command != config.ConfigMsg_ERROR {
		t.Fatalf("expected ERROR command response for invalid JSON, got %+v", resp)
	}
}

// -----------------------------------------------------------------------------

func TestProcessRequest_UnknownCommand(t *testing.T) {
	s := store.NewStore()

	req := &config.ConfigMsg{
		Command: config.ConfigMsg_Cmd(999),
	}
	data, _ := proto.Marshal(req)

	_, err := ProcessRequest(data, s, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected error for unknown command")
	}
}

// -----------------------------------------------------------------------------

func TestProcessRequest_MalformedProtobuf(t *testing.T) {
	s := store.NewStore()

	_, err := ProcessRequest([]byte("not-a-protobuf"), s, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected protobuf unmarshal error")
	}
}
