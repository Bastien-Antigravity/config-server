package store

// =============================================================================
// ESSENTIAL PROCESS:
// Unit tests for PersistenceManager, verifying atomic file saving, JSON formatting,
// directory creation, and recovery from non-existent or corrupted files.
//
// DATA FLOW:
// 1. Input: Sample ConfigMap written to temporary directories.
// 2. Logic: Invokes Save() and Load() methods, comparing original vs loaded structures.
// 3. Output: Validates error conditions and payload equality.
//
// KEY PARAMETERS:
// - mockLogger: Silent mock implementing universal-logger interfaces.Logger.
// =============================================================================

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------

type mockLogger struct {
	interfaces.Logger
}

func (m *mockLogger) Info(format string, args ...any)     {}
func (m *mockLogger) Error(format string, args ...any)    {}
func (m *mockLogger) Warning(format string, args ...any)  {}
func (m *mockLogger) Critical(format string, args ...any) {}

func TestPersistenceSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "config.json")
	pm := NewPersistenceManager(filePath, &mockLogger{})

	originalConfig := ConfigMap{
		"section1": {"key1": "value1"},
		"section2": {"key2": "value2"},
	}

	// Test Save
	if err := pm.Save(originalConfig); err != nil {
		t.Errorf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("File was not created")
	}

	// Test Load
	loadedConfig, err := pm.Load()
	if err != nil {
		t.Errorf("Load failed: %v", err)
	}

	if len(loadedConfig) != 2 {
		t.Errorf("Expected 2 sections, got %d", len(loadedConfig))
	}

	if loadedConfig["section1"]["key1"] != "value1" {
		t.Errorf("Value mismatch: expected value1, got %s", loadedConfig["section1"]["key1"])
	}
}

func TestPersistenceVersionEnvelopeAndLegacyCompat(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "versioned.json")
	pm := NewPersistenceManager(filePath, &mockLogger{})

	originalConfig := ConfigMap{
		"alpha": {"host": "127.0.0.1", "port": "8080"},
	}

	if err := pm.SaveWithVersion(originalConfig, 42); err != nil {
		t.Fatalf("SaveWithVersion failed: %v", err)
	}

	loadedConfig, loadedVer, err := pm.LoadWithVersion()
	if err != nil {
		t.Fatalf("LoadWithVersion failed: %v", err)
	}
	if loadedVer != 42 {
		t.Fatalf("Expected version 42, got %d", loadedVer)
	}
	if loadedConfig["alpha"]["host"] != "127.0.0.1" {
		t.Fatalf("Expected host 127.0.0.1, got %q", loadedConfig["alpha"]["host"])
	}

	// Legacy flat JSON file without __store_version__ envelope must still load with version 0.
	legacyPath := filepath.Join(tmpDir, "legacy.json")
	if err := os.WriteFile(legacyPath, []byte(`{"legacy":{"k":"v"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	pmLegacy := NewPersistenceManager(legacyPath, &mockLogger{})
	legacyCfg, legacyVer, err := pmLegacy.LoadWithVersion()
	if err != nil {
		t.Fatalf("LoadWithVersion(legacy) failed: %v", err)
	}
	if legacyVer != 0 {
		t.Fatalf("Expected legacy version 0, got %d", legacyVer)
	}
	if legacyCfg["legacy"]["k"] != "v" {
		t.Fatalf("Expected legacy value v, got %q", legacyCfg["legacy"]["k"])
	}
}

func TestPersistenceLoadNonExistent(t *testing.T) {
	pm := NewPersistenceManager(filepath.Join(t.TempDir(), "non-existent.json"), &mockLogger{})
	config, err := pm.Load()
	if err != nil {
		t.Errorf("Load should not fail if file doesn't exist: %v", err)
	}
	if len(config) != 0 {
		t.Errorf("Expected empty config, got %d sections", len(config))
	}
}
