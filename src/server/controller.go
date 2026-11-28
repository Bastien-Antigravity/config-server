package server

// =============================================================================
// ESSENTIAL PROCESS:
// Implements the core.ConfigController interface on Server, providing atomic
// configuration accessors, mutators, dynamic listing, and health status reporting.
//
// DATA FLOW:
// 1. Input: Management requests from REST, gRPC, and Telegram adapters.
// 2. Logic: Updates the in-memory Store with Copy-On-Write semantics, triggers
//    dirty persistence, and broadcasts change notifications.
// 3. Output: Returns configuration values, status metadata, and error codes.
//
// KEY PARAMETERS:
// - Server: Core stateful daemon holding store and listeners.
// - StatusInfo: Runtime telemetry structure reporting service status and connected clients.
// =============================================================================

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/core"
	"github.com/Bastien-Antigravity/config-server/src/helpers"
	"github.com/Bastien-Antigravity/config-server/src/store"

	toolbox_version "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/version"
)

// -----------------------------------------------------------------------------

// Ensure *Server implements core.ConfigController
var _ core.ConfigController = (*Server)(nil)

// -----------------------------------------------------------------------------

// GetConfig returns a specific configuration value from the dynamic store.
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

// -----------------------------------------------------------------------------

// SetConfig updates a configuration value atomically and broadcasts the change.
func (s *Server) SetConfig(ctx context.Context, section, key, value string) error {
	return s.SetConfigWithOCC(ctx, section, key, value, 0)
}

// -----------------------------------------------------------------------------

// SetConfigWithOCC updates a configuration value atomically with OCC verification, commits to persistence, and broadcasts.
func (s *Server) SetConfigWithOCC(ctx context.Context, section, key, value string, expectedVersion uint64) error {
	if section == "" || key == "" {
		return fmt.Errorf("section and key must not be empty")
	}
	s.Logger.Info("Controller : SetConfig request for [%s] %s = %s (expected_version: %d)", section, key, helpers.RedactValue(key, value), expectedVersion)

	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()

	var committedVer uint64
	var err error
	if expectedVersion == 0 {
		committedVer, err = s.Store.UpdateSectionWithVersion(section, func(sec map[string]string) error {
			sec[key] = value
			return nil
		})
	} else {
		_, committedVer, err = s.Store.UpdateAtomicWithVersion(expectedVersion, func(sandbox store.ConfigMap) (store.ConfigMap, error) {
			if _, ok := sandbox[section]; !ok {
				sandbox[section] = make(map[string]string)
			}
			sandbox[section][key] = value
			return sandbox, nil
		})
	}
	if err != nil {
		return err
	}

	s.TriggerSave()
	// Broadcast only the O(1) single-key delta rather than marshaling the full configuration tree
	delta := store.ConfigMap{
		section: {key: value},
	}
	s.broadcastConfigLocked(delta, committedVer)
	return nil
}

// -----------------------------------------------------------------------------

// DeleteConfig deletes a configuration key atomically and broadcasts the change.
func (s *Server) DeleteConfig(ctx context.Context, section, key string) error {
	if section == "" || key == "" {
		return fmt.Errorf("section and key must not be empty")
	}
	s.Logger.Info("Controller : DeleteConfig request for [%s] %s", section, key)

	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()

	committedVer, err := s.Store.UpdateSectionWithVersion(section, func(sec map[string]string) error {
		delete(sec, key)
		return nil
	})
	if err != nil {
		return err
	}

	s.TriggerSave()
	s.broadcastFullRefreshLocked(s.Store.Get(), committedVer)
	return nil
}

// -----------------------------------------------------------------------------

func isWildcardBindIP(ip string) bool {
	trimmed := strings.TrimSpace(ip)
	return trimmed == "0.0.0.0" || trimmed == "::" || trimmed == "[::]"
}

// BuildBaselineConfig merges static AppConfig Common and non-wildcard Capabilities
// underneath any overlay (store snapshot or persisted state).
func (s *Server) BuildBaselineConfig(overlay store.ConfigMap) store.ConfigMap {
	merged := store.DeepCopy(overlay)
	if s.AppConfig == nil {
		return merged
	}

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

	// 2. Merge capability sections from AppConfig if not overridden (filtering wildcard bind IPs)
	for capName, capVal := range s.AppConfig.Capabilities {
		if capMap, ok := capVal.(map[string]any); ok {
			if _, ok := merged[capName]; !ok {
				merged[capName] = make(map[string]string)
			}
			sec := merged[capName]
			for k, v := range capMap {
				switch typed := v.(type) {
				case string:
					if k == "ip" && isWildcardBindIP(typed) {
						continue
					}
					setIfEmpty(sec, k, typed)
				case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
					setIfEmpty(sec, k, fmt.Sprintf("%v", typed))
				}
			}
			if len(sec) == 0 {
				delete(merged, capName)
			}
		}
	}

	return merged
}

// -----------------------------------------------------------------------------

// ListConfig returns the full current configuration state, merging static base configurations with dynamic overrides.
func (s *Server) ListConfig(ctx context.Context) (store.ConfigMap, error) {
	s.Logger.Debug("Controller : ListConfig request")
	return s.BuildBaselineConfig(s.Store.Get()), nil
}

// -----------------------------------------------------------------------------

// PersistConfig commits the current store state synchronously to disk, returning any persistence failure.
func (s *Server) PersistConfig(ctx context.Context) error {
	s.Logger.Info("Controller : PersistConfig request")
	if s.Persistence == nil {
		s.TriggerSave()
		return nil
	}
	return s.SaveSync()
}

// -----------------------------------------------------------------------------

// GetStoreVersion returns the current monotonic configuration version.
func (s *Server) GetStoreVersion(ctx context.Context) uint64 {
	return s.Store.GetVersion()
}

// -----------------------------------------------------------------------------

// ServerVersion defines the fallback release version for minimal unit structs without AppConfig.
var ServerVersion = toolbox_version.Get()

// GetStatus returns the health status, active client counts, and client names.
func (s *Server) GetStatus(ctx context.Context) (core.StatusInfo, error) {
	s.Logger.Debug("Controller : GetStatus request")
	ver := ServerVersion
	if s.AppConfig != nil && s.AppConfig.Config != nil {
		if resolved := s.AppConfig.GetVersion(); resolved != "" {
			ver = resolved
		}
	}
	return core.StatusInfo{
		Healthy:       true,
		Status:        "Operational",
		Version:       ver,
		StoreVersion:  s.Store.GetVersion(),
		Timestamp:     time.Now().Unix(),
		ActiveClients: s.GetActiveClients(),
		ClientNames:   s.GetClientNames(),
	}, nil
}
