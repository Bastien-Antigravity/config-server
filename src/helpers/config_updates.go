package helpers

// =============================================================================
// ESSENTIAL PROCESS:
// Performs deep merge updates between current configuration map state and incoming delta key-value updates.
//
// DATA FLOW:
// 1. Input: Current ConfigMap and incoming updates ConfigMap.
// 2. Logic: Creates a deep copy of current state and overlays incoming section/key pairs.
// 3. Output: Newly allocated merged ConfigMap.
//
// KEY PARAMETERS:
// - current: Active configuration snapshot.
// - updates: Delta configuration mapping to merge.
// =============================================================================

import (
	"fmt"

	"github.com/Bastien-Antigravity/config-server/src/store"
)

// -----------------------------------------------------------------------------

// SeedCapabilities populates target with any scalar entries from capabilities
// that are not already present in target, preserving persisted overrides.
func SeedCapabilities(target store.ConfigMap, capabilities map[string]interface{}) store.ConfigMap {
	if target == nil {
		target = make(store.ConfigMap)
	}
	for capName, capVal := range capabilities {
		sec := target[capName]
		if sec == nil {
			sec = make(map[string]string)
		}
		switch capMap := capVal.(type) {
		case map[string]interface{}:
			for k, v := range capMap {
				if _, exists := sec[k]; exists {
					continue
				}
				switch typed := v.(type) {
				case string:
					if typed != "" {
						sec[k] = typed
					}
				case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
					sec[k] = fmt.Sprintf("%v", typed)
				}
			}
		case map[string]string:
			for k, v := range capMap {
				if _, exists := sec[k]; !exists && v != "" {
					sec[k] = v
				}
			}
		}
		if len(sec) > 0 {
			target[capName] = sec
		}
	}
	return target
}

// -----------------------------------------------------------------------------

// ApplyUpdates performs an efficient granular merge between current configuration
// and incoming updates, cloning only the touched sections while structurally sharing untouched sections.
// It also supports key deletions via the "__DELETED__" sentinel value.
func ApplyUpdates(current, updates store.ConfigMap) store.ConfigMap {
	newConf := make(store.ConfigMap, len(current)+len(updates))
	for sec, kv := range current {
		newConf[sec] = kv
	}

	for section, kv := range updates {
		existingSec := newConf[section]
		clonedSec := make(map[string]string, len(existingSec)+len(kv))
		for k, v := range existingSec {
			clonedSec[k] = v
		}
		for k, v := range kv {
			if v == "__DELETED__" {
				delete(clonedSec, k)
			} else {
				clonedSec[k] = v
			}
		}
		if len(clonedSec) == 0 {
			delete(newConf, section)
		} else {
			newConf[section] = clonedSec
		}
	}
	return newConf
}
