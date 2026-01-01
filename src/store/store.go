package store

import (
	"sync/atomic"
)

// ConfigMap represents the configuration data structure (Section -> Key -> Value)
type ConfigMap map[string]map[string]string

// -----------------------------------------------------------------------------

// Store provides a thread-safe atomic configuration store.
// Reads are lock-free using atomic.Pointer.
// Writes use CompareAndSwapLoop (or just Store if we enforce a single writer via a mutex wrapper,
// but here we provide a Replace method for full atomic swaps).
type Store struct {
	// config holds the pointer to the immutable map
	config atomic.Pointer[ConfigMap]
}

// -----------------------------------------------------------------------------

// NewStore initializes a new Store with an empty config.
func NewStore() *Store {
	s := &Store{}
	empty := make(ConfigMap)
	s.config.Store(&empty)
	return s
}

// -----------------------------------------------------------------------------

// Get returns the current configuration map.
// This is a lock-free operation.
// The returned map SHOULD NOT be modified effectively (treat as immutable).
func (s *Store) Get() ConfigMap {
	val := s.config.Load()
	if val == nil {
		return make(ConfigMap)
	}
	return *val
}

// -----------------------------------------------------------------------------

// GetSection returns a copy of a specific section.
func (s *Store) GetSection(section string) map[string]string {
	conf := s.Get()
	if val, ok := conf[section]; ok {
		// Return a copy to prevent modification of the shared map from outside
		copyMap := make(map[string]string, len(val))
		for k, v := range val {
			copyMap[k] = v
		}
		return copyMap
	}
	return nil
}

// -----------------------------------------------------------------------------

// Replace atomically replaces the entire configuration with a new one.
// This is the "Drop-In Replacement" strategy.
func (s *Store) Replace(newConfig ConfigMap) {
	s.config.Store(&newConfig)
}

// -----------------------------------------------------------------------------

// UpdateAtomic applies a modification function to the current config and atomically updates it.
// It retries if the config has changed in the meantime (Compare-And-Swap loop).
// modificationFn should return the new state based on the current state.
func (s *Store) UpdateAtomic(modificationFn func(current ConfigMap) (ConfigMap, error)) error {
	for {
		currentPtr := s.config.Load()
		current := *currentPtr

		// Create a deep copy to modify
		newConfig, err := modificationFn(current)
		if err != nil {
			return err
		}

		// Attempt to swap
		if s.config.CompareAndSwap(currentPtr, &newConfig) {
			return nil
		}
		// If failed, loop and try again with the new current value
	}
}

// -----------------------------------------------------------------------------

// Helper to deep copy the map (used during updates)
func DeepCopy(src ConfigMap) ConfigMap {
	dst := make(ConfigMap)
	for sect, kv := range src {
		dstSect := make(map[string]string, len(kv))
		for k, v := range kv {
			dstSect[k] = v
		}
		dst[sect] = dstSect
	}
	return dst
}
