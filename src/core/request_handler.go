package core

// =============================================================================
// ESSENTIAL PROCESS:
// Decodes incoming Protobuf ConfigMsg frames, executes the corresponding
// configuration command against Store, and returns response frames.
//
// DATA FLOW:
// 1. Input: Byte slice containing serialized Protobuf ConfigMsg from client socket.
// 2. Logic: Dispatches commands (GET_SYNC, FULL_REFRESH, PUT_SYNC), applies atomic updates,
//    and asynchronously triggers broadcast and persistence callbacks.
// 3. Output: Response ConfigMsg pointer or error.
//
// KEY PARAMETERS:
// - data: Raw binary frame payload.
// - s: Atomic in-memory Store.
// =============================================================================

import (
	"encoding/json"
	"fmt"

	"github.com/Bastien-Antigravity/config-server/src/helpers"
	"github.com/Bastien-Antigravity/config-server/src/store"

	config "github.com/Bastien-Antigravity/distributed-config/src/schemas"

	"google.golang.org/protobuf/proto"
)

// -----------------------------------------------------------------------------

// ProcessRequest handles the business logic for incoming configuration requests.
// It returns a response message to be sent back to the client.
// It may also trigger a broadcast and persistence via the provided callbacks.
func ProcessRequest(data []byte, s *store.Store, broadcast func(config.ConfigMsg_Cmd, []byte), triggerSave func()) (*config.ConfigMsg, error) {
	req := &config.ConfigMsg{}
	if err := proto.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("protobuf unmarshal error: %w", err)
	}

	resp := &config.ConfigMsg{}

	switch req.Command {
	case config.ConfigMsg_GET_SYNC, config.ConfigMsg_FULL_REFRESH:
		resp.Command = req.Command
		payload, _ := json.Marshal(s.Get())
		resp.Payload = payload

	case config.ConfigMsg_PUT_SYNC:
		var updates map[string]map[string]string
		if err := json.Unmarshal(req.Payload, &updates); err != nil {
			resp.Command = config.ConfigMsg_ERROR
			resp.Payload = []byte("failed to decode JSON updates")
			break
		}

		err := s.UpdateAtomic(func(current store.ConfigMap) (store.ConfigMap, error) {
			return helpers.ApplyUpdates(current, updates), nil
		})

		if err == nil {
			resp.Command = config.ConfigMsg_ACK
			payload, _ := json.Marshal(updates)

			// Asynchronous Rituals: Broadcast and Persist
			go broadcast(config.ConfigMsg_BROADCAST_SYNC, payload)
			triggerSave()
		} else {
			resp.Command = config.ConfigMsg_ERROR
			resp.Payload = []byte("atomic update failed")
		}

	default:
		return nil, fmt.Errorf("unknown command: %v", req.Command)
	}

	return resp, nil
}
