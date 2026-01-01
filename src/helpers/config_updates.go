package helpers

import (
	"config-server/src/store"
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
