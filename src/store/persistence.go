package store

// =============================================================================
// ESSENTIAL PROCESS:
// Persists configuration state to disk and restores it upon startup, ensuring
// fault tolerance through atomic file creation, sync, and rename semantics.
//
// DATA FLOW:
// 1. Input: ConfigMap to serialize to JSON, or filesystem path on startup.
// 2. Logic: Reads from disk on Load; writes to a temporary file, calls sync(),
//    and atomically renames to target on Save.
// 3. Output: Loaded ConfigMap on boot, or nil error upon safe disk commit.
//
// KEY PARAMETERS:
// - filePath: Target configuration store JSON path (e.g. config_store.json).
// - PersistenceManager: Mutex-guarded atomic disk persistence worker.
// =============================================================================

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	unilog_interfaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------

type persistedEnvelope struct {
	Version uint64    `json:"__store_version__"`
	Data    ConfigMap `json:"__store_data__"`
}

// -----------------------------------------------------------------------------

// PersistenceManager handles saving and loading the configuration from disk.
type PersistenceManager struct {
	filePath string
	Logger   unilog_interfaces.Logger
	mu       sync.Mutex // Ensures only one save operation happens at a time
}

// -----------------------------------------------------------------------------

// NewPersistenceManager creates a new manager for the given file path.
func NewPersistenceManager(path string, logger unilog_interfaces.Logger) *PersistenceManager {
	return &PersistenceManager{
		filePath: path,
		Logger:   logger,
	}
}

// -----------------------------------------------------------------------------

// Load reads the configuration map from disk.
// If the file does not exist, it returns an empty ConfigMap and no error.
func (pm *PersistenceManager) Load() (ConfigMap, error) {
	cfg, _, err := pm.LoadWithVersion()
	return cfg, err
}

// -----------------------------------------------------------------------------

// LoadWithVersion reads the configuration map and persisted monotonic version from disk.
// Backward-compatible with legacy flat ConfigMap JSON files (returns version 0).
func (pm *PersistenceManager) LoadWithVersion() (ConfigMap, uint64, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	data, err := os.ReadFile(pm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(ConfigMap), 0, nil
		}
		return nil, 0, fmt.Errorf("failed to read config file: %w", err)
	}

	if bytes.Contains(data, []byte(`"__store_data__"`)) {
		var env persistedEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal config envelope: %w", err)
		}
		if env.Data == nil {
			env.Data = make(ConfigMap)
		}
		return env.Data, env.Version, nil
	}

	var config ConfigMap
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, 0, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if config == nil {
		config = make(ConfigMap)
	}

	return config, 0, nil
}

// -----------------------------------------------------------------------------

// Save writes the given ConfigMap to disk in a human-readable JSON format.
// It uses an atomic write pattern (write to temp file, sync, then rename) to
// prevent file corruption in case of crashes during the write process.
func (pm *PersistenceManager) Save(config ConfigMap) error {
	return pm.SaveWithVersion(config, 0)
}

// -----------------------------------------------------------------------------

// SaveWithVersion writes the given ConfigMap (and optional version > 0) atomically to disk.
func (pm *PersistenceManager) SaveWithVersion(config ConfigMap, version uint64) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	dir := filepath.Dir(pm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var payload any = config
	if version > 0 {
		payload = persistedEnvelope{
			Version: version,
			Data:    config,
		}
	}

	data, err := json.MarshalIndent(payload, "", "  ")
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
