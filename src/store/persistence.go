package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// PersistenceManager handles saving and loading the configuration from disk.
type PersistenceManager struct {
	filePath string
	Logger   interfaces.Logger
	mu       sync.Mutex // Ensures only one save operation happens at a time
}

// -----------------------------------------------------------------------------

// NewPersistenceManager creates a new manager for the given file path.
func NewPersistenceManager(path string, logger interfaces.Logger) *PersistenceManager {
	return &PersistenceManager{
		filePath: path,
		Logger:   logger,
	}
}

// -----------------------------------------------------------------------------

// Load reads the configuration map from disk.
// If the file does not exist, it returns an empty ConfigMap and no error.
func (pm *PersistenceManager) Load() (ConfigMap, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	data, err := os.ReadFile(pm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(ConfigMap), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ConfigMap
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}

// -----------------------------------------------------------------------------

// Save writes the given ConfigMap to disk in a human-readable JSON format.
// It uses an atomic write pattern (write to temp file, then rename) to prevent
// file corruption in case of crashes during the write process.
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

	// Write to a temporary file first
	tmpPath := pm.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary config file: %w", err)
	}

	// Rename the temporary file to the final path (atomic on most systems)
	if err := os.Rename(tmpPath, pm.filePath); err != nil {
		// Attempt to clean up temp file if rename fails
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to commit config file: %w", err)
	}

	return nil
}
