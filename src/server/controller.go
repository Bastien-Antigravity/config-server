package server

import (
	"context"
	"fmt"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/core"
	"github.com/Bastien-Antigravity/config-server/src/store"
)

// Ensure *Server implements core.ConfigController
var _ core.ConfigController = (*Server)(nil)

// GetConfig returns a specific configuration value.
func (s *Server) GetConfig(ctx context.Context, section, key string) (string, bool, error) {
	s.Logger.Debug("Controller : GetConfig request for [%s] %s", section, key)

	val := s.Store.Get()
	if secMap, ok := val[section]; ok {
		if v, ok := secMap[key]; ok {
			return v, true, nil
		}
	}
	return "", false, nil
}

// SetConfig updates a configuration value atomically and broadcasts the change.
func (s *Server) SetConfig(ctx context.Context, section, key, value string) error {
	s.Logger.Info("Controller : SetConfig request for [%s] %s = %s", section, key, value)

	err := s.Store.UpdateAtomic(func(sandbox store.ConfigMap) (store.ConfigMap, error) {
		if _, ok := sandbox[section]; !ok {
			sandbox[section] = make(map[string]string)
		}
		sandbox[section][key] = value
		return sandbox, nil
	})
	if err != nil {
		return err
	}

	s.TriggerSave()
	s.BroadcastConfig()
	return nil
}

// DeleteConfig deletes a configuration key atomically and broadcasts the change.
func (s *Server) DeleteConfig(ctx context.Context, section, key string) error {
	s.Logger.Info("Controller : DeleteConfig request for [%s] %s", section, key)

	err := s.Store.UpdateAtomic(func(sandbox store.ConfigMap) (store.ConfigMap, error) {
		if keys, ok := sandbox[section]; ok {
			delete(keys, key)
		}
		return sandbox, nil
	})
	if err != nil {
		return err
	}

	s.TriggerSave()
	s.BroadcastConfig()
	return nil
}

// ListConfig returns the full current configuration state, merging static base configurations with dynamic overrides.
func (s *Server) ListConfig(ctx context.Context) (store.ConfigMap, error) {
	s.Logger.Debug("Controller : ListConfig request")

	// Start with a copy of the dynamic store overrides
	merged := store.DeepCopy(s.Store.Get())

	// Helper to set a configuration value only if no override exists
	setIfEmpty := func(sec map[string]string, k, v string) {
		if _, exists := sec[k]; !exists && v != "" {
			sec[k] = v
		}
	}

	// 1. Merge "common" section from AppConfig if not overridden
	if _, ok := merged["common"]; !ok {
		merged["common"] = make(map[string]string)
	}
	commonSec := merged["common"]
	setIfEmpty(commonSec, "name", s.AppConfig.Common.Name)
	setIfEmpty(commonSec, "common_file_path", s.AppConfig.Common.CommonFilePath)
	setIfEmpty(commonSec, "public_key", s.AppConfig.Common.PublicKey)
	setIfEmpty(commonSec, "public_ip", s.AppConfig.Common.PublicIP)
	setIfEmpty(commonSec, "retry_base_ms", s.AppConfig.Common.RetryBaseMS)
	setIfEmpty(commonSec, "retry_max_sec", s.AppConfig.Common.RetryMaxSec)

	if len(commonSec) == 0 {
		delete(merged, "common")
	}

	// 2. Merge capability sections from AppConfig if not overridden
	for capName, capVal := range s.AppConfig.Capabilities {
		if capMap, ok := capVal.(map[string]interface{}); ok {
			if _, ok := merged[capName]; !ok {
				merged[capName] = make(map[string]string)
			}
			sec := merged[capName]
			for k, v := range capMap {
				setIfEmpty(sec, k, fmt.Sprintf("%v", v))
			}
			if len(sec) == 0 {
				delete(merged, capName)
			}
		}
	}

	return merged, nil
}

// PersistConfig manually triggers a save of the configuration state.
func (s *Server) PersistConfig(ctx context.Context) error {
	s.Logger.Info("Controller : PersistConfig request")
	s.TriggerSave()
	return nil
}

// GetStatus returns the health status, active client counts, and client names.
func (s *Server) GetStatus(ctx context.Context) (core.StatusInfo, error) {
	s.Logger.Debug("Controller : GetStatus request")
	return core.StatusInfo{
		Healthy:       true,
		Status:        "Operational",
		Version:       "1.2.0",
		Timestamp:     time.Now().Unix(),
		ActiveClients: s.GetActiveClients(),
		ClientNames:   s.GetClientNames(),
	}, nil
}
