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
// It uses an atomic write pattern (write to temp file, sync, then rename) to 
// prevent file corruption in case of crashes during the write process.
func (pm *PersistenceManager) Save(config ConfigMap) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir := filepath.Dir(pm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 1. Create a temporary file in the same directory
	tmpFile, err := os.CreateTemp(dir, "config_*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath) // Cleanup if we return early (no-op after Rename)
	}()

	// 2. Write and Sync data
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// 3. Atomic Rename
	if err := os.Rename(tmpPath, pm.filePath); err != nil {
		return fmt.Errorf("failed to commit config file: %w", err)
	}

	// 4. Sync the directory to ensure the rename is persisted
	if df, err := os.Open(dir); err == nil {
		_ = df.Sync()
		_ = df.Close()
	}

	return nil
}
