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
	"strings"

	"github.com/Bastien-Antigravity/config-server/src/store"
)

// -----------------------------------------------------------------------------

// RedactValue masks sensitive configuration values (e.g. ENC(...) tokens,
// passwords, secrets, API keys) before writing audit or debug logs.
func RedactValue(key, value string) string {
	if strings.HasPrefix(value, "ENC(") {
		return "[REDACTED]"
	}
	lowerKey := strings.ToLower(key)
	if strings.Contains(lowerKey, "password") ||
		strings.Contains(lowerKey, "secret") ||
		strings.Contains(lowerKey, "token") ||
		strings.Contains(lowerKey, "private_key") ||
		strings.Contains(lowerKey, "api_key") {
		return "[REDACTED]"
	}
	return value
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
