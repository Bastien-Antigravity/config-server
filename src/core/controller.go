package core

// =============================================================================
// ESSENTIAL PROCESS:
// Defines the core configuration controller interface and health status data structures
// for unified management across TCP, gRPC, REST, and Telegram subsystems.
//
// DATA FLOW:
// 1. Input: Configuration queries, updates, reload signals, and persistence commands.
// 2. Logic: Enforces a uniform contract for querying, mutating, reloading, and auditing.
// 3. Output: Normalized configuration values, status metadata, and error responses.
//
// KEY PARAMETERS:
// - ConfigController: Core interface implemented by server.Server.
// - StatusInfo: Health status and client connection telemetry snapshot.
// =============================================================================

import (
	"context"

	"github.com/Bastien-Antigravity/config-server/src/store"
)

// -----------------------------------------------------------------------------

// StatusInfo represents the server health and runtime statistics.
type StatusInfo struct {
	Healthy       bool     `json:"healthy"`
	Status        string   `json:"status"`
	Version       string   `json:"version"`
	Timestamp     int64    `json:"timestamp"`
	ActiveClients int      `json:"active_clients"`
	ClientNames   []string `json:"client_names"`
}

// ConfigController defines the unified interface for configuration management.
type ConfigController interface {
	GetConfig(ctx context.Context, section, key string) (string, bool, error)
	SetConfig(ctx context.Context, section, key, value string) error
	DeleteConfig(ctx context.Context, section, key string) error
	ListConfig(ctx context.Context) (store.ConfigMap, error)
	ReloadConfig(ctx context.Context) error
	PersistConfig(ctx context.Context) error
	GetStatus(ctx context.Context) (StatusInfo, error)
}
