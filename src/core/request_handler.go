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
	"errors"
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
		resp.Version = int32(s.GetVersion())
		payload, _ := json.Marshal(s.Get())
		resp.Payload = payload

	case config.ConfigMsg_PUT_SYNC:
		var updates map[string]map[string]string
		if err := json.Unmarshal(req.Payload, &updates); err != nil {
			resp.Command = config.ConfigMsg_ERROR
			resp.Payload = []byte("failed to decode JSON updates")
			break
		}

		hasDeletion := false
		for _, kv := range updates {
			for _, v := range kv {
				if v == "__DELETED__" {
					hasDeletion = true
					break
				}
			}
			if hasDeletion {
				break
			}
		}

		var err error
		if req.Version > 0 {
			err = s.UpdateAtomicWithOCC(uint64(req.Version), func(current store.ConfigMap) (store.ConfigMap, error) {
				return helpers.ApplyUpdates(current, updates), nil
			})
		} else {
			err = s.UpdateAtomic(func(current store.ConfigMap) (store.ConfigMap, error) {
				return helpers.ApplyUpdates(current, updates), nil
			})
		}

		if err == nil {
			resp.Command = config.ConfigMsg_ACK
			resp.Version = int32(s.GetVersion())

			if triggerSave != nil {
				triggerSave()
			}
			if broadcast != nil {
				if hasDeletion {
					fullPayload, _ := json.Marshal(s.Get())
					go broadcast(config.ConfigMsg_FULL_REFRESH, fullPayload)
				} else {
					payload, _ := json.Marshal(updates)
					go broadcast(config.ConfigMsg_BROADCAST_SYNC, payload)
				}
			}
		} else if errors.Is(err, store.ErrVersionMismatch) {
			resp.Command = config.ConfigMsg_ERROR
			resp.Version = int32(s.GetVersion())
			resp.Payload = []byte("version mismatch conflict")
		} else {
			resp.Command = config.ConfigMsg_ERROR
			resp.Payload = []byte("atomic update failed")
		}

	case config.ConfigMsg_DELETE_SYNC:
		var deletions map[string][]string
		if err := json.Unmarshal(req.Payload, &deletions); err != nil {
			// Also attempt to unmarshal map[string]map[string]string for wire flexibility
			var mapDeletions map[string]map[string]string
			if errMap := json.Unmarshal(req.Payload, &mapDeletions); errMap == nil {
				deletions = make(map[string][]string)
				for sec, kv := range mapDeletions {
					for k := range kv {
						deletions[sec] = append(deletions[sec], k)
					}
				}
			} else {
				resp.Command = config.ConfigMsg_ERROR
				resp.Payload = []byte("failed to decode JSON deletions: expected map[string][]string or map[string]map[string]string")
				break
			}
		}

		var err error
		if req.Version > 0 {
			err = s.UpdateAtomicWithOCC(uint64(req.Version), func(sandbox store.ConfigMap) (store.ConfigMap, error) {
				for section, keys := range deletions {
					if secMap, ok := sandbox[section]; ok {
						if len(keys) == 0 {
							delete(sandbox, section)
						} else {
							for _, k := range keys {
								delete(secMap, k)
							}
						}
					}
				}
				return sandbox, nil
			})
		} else {
			err = s.UpdateAtomic(func(sandbox store.ConfigMap) (store.ConfigMap, error) {
				for section, keys := range deletions {
					if secMap, ok := sandbox[section]; ok {
						if len(keys) == 0 {
							delete(sandbox, section)
						} else {
							for _, k := range keys {
								delete(secMap, k)
							}
						}
					}
				}
				return sandbox, nil
			})
		}

		if err == nil {
			resp.Command = config.ConfigMsg_ACK
			resp.Version = int32(s.GetVersion())

			if triggerSave != nil {
				triggerSave()
			}
			// Broadcast FULL_REFRESH so all clients immediately purge deleted keys
			fullPayload, _ := json.Marshal(s.Get())
			if broadcast != nil {
				go broadcast(config.ConfigMsg_FULL_REFRESH, fullPayload)
			}
		} else if errors.Is(err, store.ErrVersionMismatch) {
			resp.Command = config.ConfigMsg_ERROR
			resp.Version = int32(s.GetVersion())
			resp.Payload = []byte("version mismatch conflict")
		} else {
			resp.Command = config.ConfigMsg_ERROR
			resp.Payload = []byte("atomic delete failed")
		}

	default:
		return nil, fmt.Errorf("unknown command: %v", req.Command)
	}

	return resp, nil
}
