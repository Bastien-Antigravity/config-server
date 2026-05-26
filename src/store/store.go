package store

import (
	"sync"
)

// ConfigMap represents the configuration data structure (Section -> Key -> Value)
type ConfigMap map[string]map[string]string

// -----------------------------------------------------------------------------

// Store provides a thread-safe configuration store.
// It uses a RWMutex and a Copy-On-Write (COW) strategy to provide
// extremely fast reads while maintaining atomic, side-effect-free updates.
type Store struct {
	mu     sync.RWMutex
	config ConfigMap
}

// -----------------------------------------------------------------------------

// NewStore initializes a new Store with an empty config.
func NewStore() *Store {
	return &Store{
		config: make(ConfigMap),
	}
}

// -----------------------------------------------------------------------------

// Get returns the current configuration map.
// Optimization: Returns the map directly (Read-Fast path).
// Callers MUST treat the returned map as immutable.
func (s *Store) Get() ConfigMap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// -----------------------------------------------------------------------------

// GetSection returns a copy of a specific section.
func (s *Store) GetSection(section string) map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if val, ok := s.config[section]; ok {
		// We return a copy so the caller can't accidentally modify the master store
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
// Ensures the store "owns" the data by performing a deep copy.
func (s *Store) Replace(newConfig ConfigMap) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = DeepCopy(newConfig)
}

// -----------------------------------------------------------------------------

// UpdateAtomic applies a modification function to the current config.
// Implementation: Copy-On-Write. 
// It creates a deep copy to pass to the modification function. If the function
// succeeds, the internal pointer is swapped. If it fails, the master state
// remains untouched (Atomicity/Rollback).
func (s *Store) UpdateAtomic(modificationFn func(sandbox ConfigMap) (ConfigMap, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Create a sandbox for the modification
	sandbox := DeepCopy(s.config)
	
	// 2. Apply updates to the sandbox
	result, err := modificationFn(sandbox)
	if err != nil {
		return err // Atomicity: s.config is unchanged
	}

	// 3. Commit the new version
	s.config = result
	return nil
}

// -----------------------------------------------------------------------------

// Helper to deep copy the map (used for COW updates)
func DeepCopy(src ConfigMap) ConfigMap {
	if src == nil {
		return make(ConfigMap)
	}
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
