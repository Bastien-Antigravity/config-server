package store

// =============================================================================
// ESSENTIAL PROCESS:
// Unit tests verifying thread-safe Copy-On-Write (COW) semantics, atomic updates,
// rollback guarantees on modification failure, and concurrency safety of Store.
//
// DATA FLOW:
// 1. Input: Sample configuration maps and concurrent update closures.
// 2. Logic: Asserts atomicity, immutability of returned section copies, and error rollbacks.
// 3. Output: Go testing PASS/FAIL assertions.
//
// KEY PARAMETERS:
// - Store: Thread-safe in-memory COW repository.
// =============================================================================

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

// -----------------------------------------------------------------------------

func TestStore_GetAndGetSection(t *testing.T) {
	s := NewStore()

	initial := ConfigMap{
		"timescale_db": {
			"host": "127.0.0.1",
			"port": "5432",
		},
		"redis": {
			"port": "6379",
		},
	}
	s.Replace(initial)

	// Verify Get()
	got := s.Get()
	if len(got) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(got))
	}
	if got["timescale_db"]["port"] != "5432" {
		t.Fatalf("expected port 5432, got %s", got["timescale_db"]["port"])
	}

	// Verify GetSection()
	sec := s.GetSection("timescale_db")
	if sec == nil || sec["host"] != "127.0.0.1" {
		t.Fatalf("unexpected section content: %v", sec)
	}

	// Verify GetSection returns a copy (mutating returned map must not affect store)
	sec["host"] = "mutated_host"
	if s.Get()["timescale_db"]["host"] != "127.0.0.1" {
		t.Fatalf("store state was modified via section map mutation")
	}

	// Verify non-existent section
	if s.GetSection("non_existent") != nil {
		t.Fatalf("expected nil for non-existent section")
	}
}

// -----------------------------------------------------------------------------

func TestStore_UpdateAtomic_SuccessAndRollback(t *testing.T) {
	s := NewStore()

	s.Replace(ConfigMap{
		"app": {"env": "staging"},
	})

	// 1. Successful update
	err := s.UpdateAtomic(func(sandbox ConfigMap) (ConfigMap, error) {
		sandbox["app"]["env"] = "production"
		sandbox["app"]["version"] = "1.0.0"
		return sandbox, nil
	})
	if err != nil {
		t.Fatalf("expected update to succeed, got %v", err)
	}
	if s.Get()["app"]["env"] != "production" || s.Get()["app"]["version"] != "1.0.0" {
		t.Fatalf("unexpected store state after update: %+v", s.Get())
	}

	// 2. Failed update with rollback
	expectedErr := errors.New("validation failure")
	err = s.UpdateAtomic(func(sandbox ConfigMap) (ConfigMap, error) {
		sandbox["app"]["env"] = "corrupted"
		return nil, expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	// Master state must remain untouched
	if s.Get()["app"]["env"] != "production" {
		t.Fatalf("store state was not rolled back after error: got %s", s.Get()["app"]["env"])
	}
}

// -----------------------------------------------------------------------------

func TestStore_UpdateSection(t *testing.T) {
	s := NewStore()
	s.Replace(ConfigMap{
		"sectionA": {"key1": "val1"},
		"sectionB": {"key2": "val2"},
	})

	err := s.UpdateSection("sectionA", func(sec map[string]string) error {
		sec["key1"] = "val1_updated"
		sec["newKey"] = "newVal"
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg := s.Get()
	if cfg["sectionA"]["key1"] != "val1_updated" || cfg["sectionA"]["newKey"] != "newVal" {
		t.Fatalf("expected updated sectionA values")
	}
	if cfg["sectionB"]["key2"] != "val2" {
		t.Fatalf("expected sectionB to be preserved")
	}

	// Test section deletion when all keys removed
	err = s.UpdateSection("sectionA", func(sec map[string]string) error {
		delete(sec, "key1")
		delete(sec, "newKey")
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := s.Get()["sectionA"]; exists {
		t.Fatalf("expected sectionA to be deleted when empty")
	}
}

// -----------------------------------------------------------------------------

func TestStore_DeepCopyNilSafe(t *testing.T) {
	copied := DeepCopy(nil)
	if copied == nil || len(copied) != 0 {
		t.Fatalf("expected empty non-nil map from nil DeepCopy")
	}
}

// -----------------------------------------------------------------------------

func TestStore_ConcurrentStress(t *testing.T) {
	s := NewStore()
	s.Replace(ConfigMap{
		"counter": {"val": "0"},
	})

	var wg sync.WaitGroup
	workers := 20
	iterations := 100

	// Concurrent Writers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = s.UpdateAtomic(func(sandbox ConfigMap) (ConfigMap, error) {
					sandbox["counter"]["val"] = fmt.Sprintf("%d-%d", workerID, j)
					return sandbox, nil
				})
			}
		}(i)
	}

	// Concurrent Readers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = s.Get()
				_ = s.GetSection("counter")
			}
		}()
	}

	wg.Wait()
}

// -----------------------------------------------------------------------------

func TestStore_VersionAndOCC(t *testing.T) {
	s := NewStore()
	if s.GetVersion() != 0 {
		t.Fatalf("expected initial version 0, got %d", s.GetVersion())
	}

	s.Replace(ConfigMap{"sec": {"k": "v"}})
	if s.GetVersion() != 1 {
		t.Fatalf("expected version 1 after Replace, got %d", s.GetVersion())
	}

	// Successful update
	err := s.UpdateAtomic(func(sandbox ConfigMap) (ConfigMap, error) {
		sandbox["sec"]["k"] = "v2"
		return sandbox, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.GetVersion() != 2 {
		t.Fatalf("expected version 2, got %d", s.GetVersion())
	}

	// Successful update with matching expectedVersion
	err = s.UpdateAtomicWithOCC(2, func(sandbox ConfigMap) (ConfigMap, error) {
		sandbox["sec"]["k"] = "v3"
		return sandbox, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.GetVersion() != 3 {
		t.Fatalf("expected version 3, got %d", s.GetVersion())
	}

	// OCC conflict with mismatched expectedVersion
	err = s.UpdateAtomicWithOCC(2, func(sandbox ConfigMap) (ConfigMap, error) {
		sandbox["sec"]["k"] = "v4"
		return sandbox, nil
	})
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatalf("expected ErrVersionMismatch, got %v", err)
	}
	if s.GetVersion() != 3 {
		t.Fatalf("version should remain 3 after conflict, got %d", s.GetVersion())
	}
	if s.Get()["sec"]["k"] != "v3" {
		t.Fatalf("store state should remain v3, got %s", s.Get()["sec"]["k"])
	}
}

// -----------------------------------------------------------------------------

func TestStore_UpdateAtomicWithOCC_OwnsCommittedMap(t *testing.T) {
	s := NewStore()
	var callerRef ConfigMap

	err := s.UpdateAtomicWithOCC(0, func(sandbox ConfigMap) (ConfigMap, error) {
		callerRef = ConfigMap{
			"sec": {"key": "committed"},
		}
		return callerRef, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mutate caller's retained reference after commit
	callerRef["sec"]["key"] = "corrupted_by_caller"
	callerRef["injected"] = map[string]string{"evil": "true"}

	got := s.Get()
	if got["sec"]["key"] != "committed" {
		t.Fatalf("expected store to own committed map ('committed'), got %q", got["sec"]["key"])
	}
	if _, exists := got["injected"]; exists {
		t.Fatal("expected external map mutation not to affect store state")
	}
}
