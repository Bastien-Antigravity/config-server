package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PersistenceManager handles saving and loading the configuration from disk.
type PersistenceManager struct {
	filePath string
	mu       sync.Mutex // Ensures only one save operation happens at a time
}

// -----------------------------------------------------------------------------

// NewPersistenceManager creates a new manager for the given file path.
func NewPersistenceManager(path string) *PersistenceManager {
	return &PersistenceManager{
		filePath: path,
	}
}

// -----------------------------------------------------------------------------

// Save writes the given ConfigMap to disk in a human-readable JSON format.
func (pm *PersistenceManager) Save(config ConfigMap) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(pm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshaling with Indent for human readability
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(pm.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
