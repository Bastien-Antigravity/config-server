package store

// =============================================================================
// ESSENTIAL PROCESS:
// Implements the high-concurrency, thread-safe in-memory configuration store using
// Copy-On-Write (COW) semantics protected by sync.RWMutex.
//
// DATA FLOW:
// 1. Input: Direct queries or atomic transformation closures.
// 2. Logic: Provides zero-allocation read access, creates sandbox deep copies for mutations,
//    and atomically updates the master pointer upon successful validation.
// 3. Output: Read-only ConfigMap snapshots or section sub-maps.
//
// KEY PARAMETERS:
// - ConfigMap: Section -> Key -> Value configuration dictionary.
// - Store: Concurrent state holder.
// =============================================================================

import (
	"errors"
	"sync"
	"sync/atomic"
)

// -----------------------------------------------------------------------------

// ErrVersionMismatch indicates an optimistic concurrency check failed.
var ErrVersionMismatch = errors.New("optimistic concurrency conflict: version mismatch")

// -----------------------------------------------------------------------------

// ConfigMap represents the configuration data structure (Section -> Key -> Value)
type ConfigMap map[string]map[string]string

// -----------------------------------------------------------------------------

// Store provides a thread-safe configuration store.
// It uses an atomic.Pointer and a Copy-On-Write (COW) strategy to provide
// 100% lock-free reads while maintaining atomic, side-effect-free updates.
type Store struct {
	mu      sync.Mutex
	config  atomic.Pointer[ConfigMap]
	version atomic.Uint64
}

// -----------------------------------------------------------------------------

// NewStore initializes a new Store with an empty config.
func NewStore() *Store {
	s := &Store{}
	initial := make(ConfigMap)
	s.config.Store(&initial)
	return s
}

// -----------------------------------------------------------------------------

// GetVersion returns the current monotonic configuration version lock-free.
func (s *Store) GetVersion() uint64 {
	return s.version.Load()
}

// SetVersion sets the monotonic version counter (e.g. during initial restore).
func (s *Store) SetVersion(v uint64) {
	s.version.Store(v)
}

// -----------------------------------------------------------------------------

// Get returns the current configuration map lock-free via atomic pointer load.
// Callers MUST treat the returned map as immutable.
func (s *Store) Get() ConfigMap {
	ptr := s.config.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}

// -----------------------------------------------------------------------------

// GetSection returns a copy of a specific section lock-free.
func (s *Store) GetSection(section string) map[string]string {
	ptr := s.config.Load()
	if ptr == nil {
		return nil
	}
	if val, ok := (*ptr)[section]; ok {
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
	cp := DeepCopy(newConfig)
	s.config.Store(&cp)
	s.version.Add(1)
}

// -----------------------------------------------------------------------------

// UpdateAtomic applies a modification function to the current config.
// Implementation: Copy-On-Write + atomic pointer swap.
func (s *Store) UpdateAtomic(modificationFn func(sandbox ConfigMap) (ConfigMap, error)) error {
	return s.UpdateAtomicWithOCC(0, modificationFn)
}

// -----------------------------------------------------------------------------

// UpdateAtomicWithOCC applies a modification function only if the current version matches expectedVersion (or expectedVersion == 0).
func (s *Store) UpdateAtomicWithOCC(expectedVersion uint64, modificationFn func(sandbox ConfigMap) (ConfigMap, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if expectedVersion > 0 && s.version.Load() != expectedVersion {
		return ErrVersionMismatch
	}

	// 1. Create a sandbox for the modification
	sandbox := DeepCopy(s.Get())

	// 2. Apply updates to the sandbox
	result, err := modificationFn(sandbox)
	if err != nil {
		return err // Atomicity: s.config is unchanged
	}

	// 3. Atomically swap the master pointer
	s.config.Store(&result)
	s.version.Add(1)
	return nil
}

// -----------------------------------------------------------------------------

// UpdateSection performs a high-performance granular mutation on a single section,
// cloning only the target section while structurally sharing all other sections.
func (s *Store) UpdateSection(section string, updateFn func(sec map[string]string) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur := s.Get()
	newMap := make(ConfigMap, len(cur)+1)
	for k, v := range cur {
		newMap[k] = v
	}

	clonedSec := make(map[string]string, len(cur[section])+1)
	for k, v := range cur[section] {
		clonedSec[k] = v
	}

	if err := updateFn(clonedSec); err != nil {
		return err
	}

	if len(clonedSec) == 0 {
		delete(newMap, section)
	} else {
		newMap[section] = clonedSec
	}

	s.config.Store(&newMap)
	s.version.Add(1)
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
