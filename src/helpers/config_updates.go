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
	"github.com/Bastien-Antigravity/config-server/src/store"
)

// -----------------------------------------------------------------------------

func ApplyUpdates(current, updates store.ConfigMap) store.ConfigMap {
	newConf := store.DeepCopy(current)
	for section, kv := range updates {
		if _, exists := newConf[section]; !exists {
			newConf[section] = make(map[string]string)
		}
		for k, v := range kv {
			newConf[section][k] = v
		}
	}
	return newConf
}
