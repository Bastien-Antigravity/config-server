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
// It delegates to ProcessRequestWithVersion for atomic version-stamped execution.
func ProcessRequest(data []byte, s *store.Store, broadcast func(config.ConfigMsg_Cmd, []byte), triggerSave func()) (*config.ConfigMsg, error) {
	var wrapped func(config.ConfigMsg_Cmd, []byte, uint64)
	if broadcast != nil {
		wrapped = func(cmd config.ConfigMsg_Cmd, payload []byte, _ uint64) {
			broadcast(cmd, payload)
		}
	}
	return ProcessRequestWithVersion(data, s, wrapped, triggerSave)
}

// -----------------------------------------------------------------------------

// ProcessRequestWithVersion handles incoming configuration requests atomically,
// stamping response and broadcast frames with the exact committed store version.
func ProcessRequestWithVersion(data []byte, s *store.Store, broadcast func(config.ConfigMsg_Cmd, []byte, uint64), triggerSave func()) (*config.ConfigMsg, error) {
	req := &config.ConfigMsg{}
	if err := proto.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("protobuf unmarshal error: %w", err)
	}

	resp := &config.ConfigMsg{}

	switch req.Command {
	case config.ConfigMsg_GET_SYNC, config.ConfigMsg_FULL_REFRESH:
		resp.Command = req.Command
		snapshot, ver := s.GetWithVersion()
		resp.Version = int32(ver)
		payload, _ := json.Marshal(snapshot)
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

		var expectedVer uint64
		if req.Version > 0 {
			expectedVer = uint64(req.Version)
		}
		committedSnap, committedVer, err := s.UpdateAtomicWithVersion(expectedVer, func(current store.ConfigMap) (store.ConfigMap, error) {
			return helpers.ApplyUpdates(current, updates), nil
		})

		if err == nil {
			resp.Command = config.ConfigMsg_ACK
			resp.Version = int32(committedVer)

			if triggerSave != nil {
				triggerSave()
			}
			if broadcast != nil {
				if hasDeletion {
					fullPayload, _ := json.Marshal(committedSnap)
					broadcast(config.ConfigMsg_FULL_REFRESH, fullPayload, committedVer)
				} else {
					payload, _ := json.Marshal(updates)
					broadcast(config.ConfigMsg_BROADCAST_SYNC, payload, committedVer)
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

		var expectedVer uint64
		if req.Version > 0 {
			expectedVer = uint64(req.Version)
		}
		committedSnap, committedVer, err := s.UpdateAtomicWithVersion(expectedVer, func(sandbox store.ConfigMap) (store.ConfigMap, error) {
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

		if err == nil {
			resp.Command = config.ConfigMsg_ACK
			resp.Version = int32(committedVer)

			if triggerSave != nil {
				triggerSave()
			}
			// Broadcast FULL_REFRESH with exact committed snapshot so all clients purge deleted keys in order
			if broadcast != nil {
				fullPayload, _ := json.Marshal(committedSnap)
				broadcast(config.ConfigMsg_FULL_REFRESH, fullPayload, committedVer)
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
