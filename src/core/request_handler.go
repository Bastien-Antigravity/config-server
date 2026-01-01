package core

import (
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
func ProcessRequest(data []byte, s *store.Store, pm *store.PersistenceManager, broadcast func(config.ConfigMsg_ConfigServerMsg, store.ConfigMap)) (*config.ConfigMsg, error) {
	req := &config.ConfigMsg{}
	if err := proto.Unmarshal(data, req); err != nil {
		return nil, fmt.Errorf("protobuf unmarshal error: %w", err)
	}

	resp := &config.ConfigMsg{}

	switch req.GetReqClient() {
	// --- Mem Config Operations ---
	case config.ConfigMsg_get_mem_config:
		resp.RespServer = config.ConfigMsg_propagate_mem_config
		resp.SectionsKeysValues = helpers.MapToProto(s.Get())

	case config.ConfigMsg_update_mem_config:
		updates := helpers.ProtoToMap(req.GetSectionsKeysValues())
		err := s.UpdateAtomic(func(current store.ConfigMap) (store.ConfigMap, error) {
			return helpers.ApplyUpdates(current, updates), nil
		})

		if err == nil {
			resp.RespServer = config.ConfigMsg_mem_config_update_done
			go broadcast(config.ConfigMsg_propagate_mem_config, updates)
			helpers.TryPersist(pm, s)
		} else {
			resp.RespServer = config.ConfigMsg_mem_config_update_failed
		}

	case config.ConfigMsg_dump_mem_config:
		if err := pm.Save(s.Get()); err == nil {
			resp.RespServer = config.ConfigMsg_mem_config_update_done
		} else {
			resp.RespServer = config.ConfigMsg_mem_config_update_failed
		}

	case config.ConfigMsg_add_config_listener:
		resp.RespServer = config.ConfigMsg_send_mem_config_init
		resp.SectionsKeysValues = helpers.MapToProto(s.Get())

	// --- Config Object Operations (Mapped to Store for consistency) ---
	case config.ConfigMsg_get_config_object:
		resp.RespServer = config.ConfigMsg_propagate_config
		resp.SectionsKeysValues = helpers.MapToProto(s.Get())

	case config.ConfigMsg_update_config_object:
		updates := helpers.ProtoToMap(req.GetSectionsKeysValues())
		err := s.UpdateAtomic(func(current store.ConfigMap) (store.ConfigMap, error) {
			return helpers.ApplyUpdates(current, updates), nil
		})

		if err == nil {
			resp.RespServer = config.ConfigMsg_config_update_done
			go broadcast(config.ConfigMsg_propagate_config, updates)
			helpers.TryPersist(pm, s)
		} else {
			resp.RespServer = config.ConfigMsg_config_update_failed
		}

	// --- LogLevel Operations (Stubbed or Mapped) ---
	case config.ConfigMsg_get_notif_loglevel:
		// Return current config as stand-in for separate log level store
		resp.RespServer = config.ConfigMsg_propagate_notif_loglevel
		resp.SectionsKeysValues = helpers.MapToProto(s.Get())

	case config.ConfigMsg_update_notif_loglevel:
		updates := helpers.ProtoToMap(req.GetSectionsKeysValues())
		err := s.UpdateAtomic(func(current store.ConfigMap) (store.ConfigMap, error) {
			return helpers.ApplyUpdates(current, updates), nil
		})
		if err == nil {
			// No specific "loglevel update done" enum, usually propagate back
			go broadcast(config.ConfigMsg_propagate_notif_loglevel, updates)
			helpers.TryPersist(pm, s)
			resp.RespServer = config.ConfigMsg_mem_config_update_done // Fallback ack
		} else {
			resp.RespServer = config.ConfigMsg_mem_config_update_failed
		}

	default:
		return nil, fmt.Errorf("unknown command: %v", req.GetReqClient())
	}

	return resp, nil
}
