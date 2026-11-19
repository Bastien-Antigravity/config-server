package helpers

// =============================================================================
// ESSENTIAL PROCESS:
// Unit tests verifying ApplyUpdates delta merging, ensuring new sections are
// created, existing keys are updated, and input maps remain immutable.
//
// DATA FLOW:
// 1. Input: Sample current ConfigMap and delta updates ConfigMap.
// 2. Logic: Executes ApplyUpdates and validates resulting key-value pairs.
// 3. Output: Go testing assertions for merged map state.
//
// KEY PARAMETERS:
// - ApplyUpdates: Pure merge function returning a new ConfigMap.
// =============================================================================

import (
	"testing"

	"github.com/Bastien-Antigravity/config-server/src/store"
)

// -----------------------------------------------------------------------------

func TestApplyUpdates(t *testing.T) {
	current := store.ConfigMap{
		"section1": {
			"keyA": "valA",
			"keyB": "valB",
		},
		"section2": {
			"keyC": "valC",
		},
	}

	updates := store.ConfigMap{
		"section1": {
			"keyB": "valB_updated",
			"keyD": "valD_new",
		},
		"section3": {
			"keyE": "valE",
		},
	}

	merged := ApplyUpdates(current, updates)

	// Verify original current map was not mutated
	if current["section1"]["keyB"] != "valB" {
		t.Fatalf("current map was mutated in-place")
	}

	// Verify updated key
	if merged["section1"]["keyB"] != "valB_updated" {
		t.Fatalf("expected updated value valB_updated, got %s", merged["section1"]["keyB"])
	}

	// Verify untouched key in section1
	if merged["section1"]["keyA"] != "valA" {
		t.Fatalf("expected untouched keyA to remain valA, got %s", merged["section1"]["keyA"])
	}

	// Verify new key in existing section
	if merged["section1"]["keyD"] != "valD_new" {
		t.Fatalf("expected keyD to be valD_new, got %s", merged["section1"]["keyD"])
	}

	// Verify untouched section2
	if merged["section2"]["keyC"] != "valC" {
		t.Fatalf("expected section2 to remain valC, got %s", merged["section2"]["keyC"])
	}

	// Verify newly created section3
	if merged["section3"]["keyE"] != "valE" {
		t.Fatalf("expected section3 keyE to be valE, got %s", merged["section3"]["keyE"])
	}

	// Verify sentinel deletion
	delUpdates := store.ConfigMap{
		"section1": {
			"keyA": "__DELETED__",
		},
	}
	delMerged := ApplyUpdates(merged, delUpdates)
	if _, exists := delMerged["section1"]["keyA"]; exists {
		t.Fatalf("expected keyA to be deleted via __DELETED__ sentinel")
	}
	if delMerged["section1"]["keyB"] != "valB_updated" {
		t.Fatalf("expected keyB to remain valB_updated, got %s", delMerged["section1"]["keyB"])
	}
}

// -----------------------------------------------------------------------------

func TestSeedCapabilities(t *testing.T) {
	initial := store.ConfigMap{
		"config_server": {
			"port": "3399", // Persisted override must not be clobbered
		},
	}
	caps := map[string]interface{}{
		"config_server": map[string]interface{}{
			"ip":   "127.0.0.2",
			"port": "3306",
		},
		"log_server": map[string]interface{}{
			"ip":   "127.0.0.2",
			"port": 9020,
		},
		"rag_engine": map[string]interface{}{
			"ip":   "127.0.0.2",
			"port": "8090",
			"mcp": map[string]interface{}{
				"ip":   "127.0.0.2",
				"port": "8090",
			},
		},
	}

	seeded := SeedCapabilities(initial, caps)
	if seeded["config_server"]["port"] != "3399" {
		t.Fatalf("expected existing override port 3399 to be preserved, got %s", seeded["config_server"]["port"])
	}
	if seeded["config_server"]["ip"] != "127.0.0.2" {
		t.Fatalf("expected seeded config_server.ip 127.0.0.2, got %s", seeded["config_server"]["ip"])
	}
	if seeded["log_server"]["port"] != "9020" {
		t.Fatalf("expected seeded log_server.port 9020, got %s", seeded["log_server"]["port"])
	}
	if _, hasNested := seeded["rag_engine"]["mcp"]; hasNested {
		t.Fatalf("expected nested map rag_engine.mcp to be skipped in flat ConfigMap")
	}
}
