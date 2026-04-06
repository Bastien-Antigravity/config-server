package core

import (
	"encoding/json"
	"fmt"

	"config-server/src/helpers"
	"config-server/src/store"

	config "github.com/Bastien-Antigravity/distributed-config/src/schemas"

	"google.golang.org/protobuf/proto"
)

// ProcessRequest handles the business logic for incoming configuration requests.
// It returns a response message to be sent back to the client.
// It may also trigger a broadcast via the provided callback.
// -----------------------------------------------------------------------------
func ProcessRequest(data []byte, s *store.Store, pm *store.PersistenceManager, broadcast func(config.ConfigMsg_Cmd, []byte)) (*config.ConfigMsg, error) {
	req := &config.ConfigMsg{}
	if err := proto.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("protobuf unmarshal error: %w", err)
	}

	resp := &config.ConfigMsg{}

	switch req.Command {
	case config.ConfigMsg_GET_SYNC:
		resp.Command = config.ConfigMsg_BROADCAST_SYNC
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
			go broadcast(config.ConfigMsg_BROADCAST_SYNC, payload)
			helpers.TryPersist(pm, s)
		} else {
			resp.Command = config.ConfigMsg_ERROR
			resp.Payload = []byte("atomic update failed")
		}

	default:
		return nil, fmt.Errorf("unknown command: %v", req.Command)
	}

	return resp, nil
}
