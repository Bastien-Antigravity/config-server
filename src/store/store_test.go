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
