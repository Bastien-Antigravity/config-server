package helpers

import (
	"config-server/src/store"

	config "github.com/Bastien-Antigravity/distributed-config/src/schemas"
)

// -----------------------------------------------------------------------------

// map configMap to proto
func MapToProto(m store.ConfigMap) map[string]*config.KeysValues {
	res := make(map[string]*config.KeysValues)
	for k, v := range m {
		res[k] = &config.KeysValues{KeyValue: v}
	}
	return res
}

// -----------------------------------------------------------------------------

// map proto to configMap
func ProtoToMap(m map[string]*config.KeysValues) store.ConfigMap {
	res := make(store.ConfigMap)
	for k, v := range m {
		res[k] = v.GetKeyValue()
	}
	return res
}
