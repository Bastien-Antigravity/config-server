package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

type mockLogger struct {
	interfaces.Logger
}

func (m *mockLogger) Info(format string, args ...interface{})     {}
func (m *mockLogger) Error(format string, args ...interface{})    {}
func (m *mockLogger) Warning(format string, args ...interface{})  {}
func (m *mockLogger) Critical(format string, args ...interface{}) {}

func TestPersistenceSaveLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-server-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

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

func TestPersistenceLoadNonExistent(t *testing.T) {
	pm := NewPersistenceManager("non-existent.json", &mockLogger{})
	config, err := pm.Load()
	if err != nil {
		t.Errorf("Load should not fail if file doesn't exist: %v", err)
	}
	if len(config) != 0 {
		t.Errorf("Expected empty config, got %d sections", len(config))
	}
}
