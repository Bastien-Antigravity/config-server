package grpc_control

// =============================================================================
// ESSENTIAL PROCESS:
// Implements ConfigControlServiceServer gRPC methods, translating Protobuf RPC
// requests into core.ConfigController invocations.
//
// DATA FLOW:
// 1. Input: gRPC request objects (GetConfigRequest, SetConfigRequest, etc.).
// 2. Logic: Delegates to core.ConfigController for querying, mutating, reloading, and persisting.
// 3. Output: Protobuf response messages (ControlResponse, GetConfigResponse, etc.).
//
// KEY PARAMETERS:
// - ControlServiceImpl: gRPC service handler struct.
// - controller: core.ConfigController business logic facade.
// =============================================================================

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/core"
	"github.com/Bastien-Antigravity/config-server/src/helpers"
	"github.com/Bastien-Antigravity/config-server/src/store"

	toolbox_wiretap "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/wiretap"
	unilog_interfaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------
// ControlService Implementation
// -----------------------------------------------------------------------------

type ControlServiceImpl struct {
	UnimplementedConfigControlServiceServer
	Name       string
	controller core.ConfigController
	logger     unilog_interfaces.Logger
}

// -----------------------------------------------------------------------------

// NewControlService creates a new ControlServiceImpl instance
func NewControlService(controller core.ConfigController, logger unilog_interfaces.Logger) *ControlServiceImpl {
	return &ControlServiceImpl{
		Name:       "GRPCControlService",
		controller: controller,
		logger:     logger,
	}
}

// -----------------------------------------------------------------------------
// Config Management
// -----------------------------------------------------------------------------

// GetConfig returns a specific configuration value
func (s *ControlServiceImpl) GetConfig(ctx context.Context, req *GetConfigRequest) (*GetConfigResponse, error) {
	start := time.Now()
	s.logger.Debug("%s : received GetConfig request for [%s] %s", s.Name, req.Section, req.Key)

	val, ok, err := s.controller.GetConfig(ctx, req.Section, req.Key)
	if err != nil {
		toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "GetConfig", "rpc", "inbound", req.Section+"."+req.Key, 0, float64(time.Since(start).Microseconds())/1000.0, true, false)
		return &GetConfigResponse{
			Success:   false,
			Value:     "",
			Timestamp: time.Now().Unix(),
		}, err
	}

	toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "GetConfig", "rpc", "inbound", req.Section+"."+req.Key, 0, float64(time.Since(start).Microseconds())/1000.0, false, false)
	return &GetConfigResponse{
		Success:   ok,
		Value:     val,
		Timestamp: time.Now().Unix(),
	}, nil
}

// SetConfig updates a configuration value atomically (supporting optional OCC via expected_version)
func (s *ControlServiceImpl) SetConfig(ctx context.Context, req *SetConfigRequest) (*ControlResponse, error) {
	start := time.Now()
	s.logger.Info("%s : received SetConfig request for [%s] %s = %s (expected_version: %d)", s.Name, req.Section, req.Key, helpers.RedactValue(req.Key, req.Value), req.ExpectedVersion)

	var err error
	if req.ExpectedVersion > 0 {
		err = s.controller.SetConfigWithOCC(ctx, req.Section, req.Key, req.Value, req.ExpectedVersion)
	} else {
		err = s.controller.SetConfig(ctx, req.Section, req.Key, req.Value)
	}
	if err != nil {
		errCode := "UPDATE_FAILED"
		if errors.Is(err, store.ErrVersionMismatch) {
			errCode = "CONFLICT"
		}
		toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "SetConfig", "rpc", "inbound", req.Section+"."+req.Key, 0, float64(time.Since(start).Microseconds())/1000.0, true, false)
		return &ControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: errCode,
		}, nil
	}

	toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "SetConfig", "rpc", "inbound", req.Section+"."+req.Key, 0, float64(time.Since(start).Microseconds())/1000.0, false, false)
	return &ControlResponse{
		Success:   true,
		Message:   "Configuration updated and broadcasted successfully",
		Timestamp: time.Now().Unix(),
	}, nil
}

// DeleteConfig removes a configuration key atomically and triggers FULL_REFRESH broadcast
func (s *ControlServiceImpl) DeleteConfig(ctx context.Context, req *DeleteConfigRequest) (*ControlResponse, error) {
	start := time.Now()
	s.logger.Info("%s : received DeleteConfig request for [%s] %s", s.Name, req.Section, req.Key)

	if err := s.controller.DeleteConfig(ctx, req.Section, req.Key); err != nil {
		toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "DeleteConfig", "rpc", "inbound", req.Section+"."+req.Key, 0, float64(time.Since(start).Microseconds())/1000.0, true, false)
		return &ControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to delete config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "DELETE_FAILED",
		}, nil
	}

	toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "DeleteConfig", "rpc", "inbound", req.Section+"."+req.Key, 0, float64(time.Since(start).Microseconds())/1000.0, false, false)
	return &ControlResponse{
		Success:   true,
		Message:   "Configuration key deleted and FULL_REFRESH broadcasted",
		Timestamp: time.Now().Unix(),
	}, nil
}

// ListConfig returns the entire configuration state as JSON
func (s *ControlServiceImpl) ListConfig(ctx context.Context, req *ListConfigRequest) (*ListConfigResponse, error) {
	start := time.Now()
	s.logger.Debug("%s : received ListConfig request", s.Name)

	config, err := s.controller.ListConfig(ctx)
	if err != nil {
		toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "ListConfig", "rpc", "inbound", "list", 0, float64(time.Since(start).Microseconds())/1000.0, true, false)
		return &ListConfigResponse{
			Success:   false,
			Timestamp: time.Now().Unix(),
		}, err
	}

	data, err := json.Marshal(config)
	if err != nil {
		return &ListConfigResponse{
			Success:   false,
			Timestamp: time.Now().Unix(),
		}, nil
	}

	toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "ListConfig", "rpc", "inbound", "list", len(data), float64(time.Since(start).Microseconds())/1000.0, false, false)
	return &ListConfigResponse{
		Success:    true,
		JsonConfig: string(data),
		Timestamp:  time.Now().Unix(),
	}, nil
}

// ReloadConfig reloads configuration from the base YAML
func (s *ControlServiceImpl) ReloadConfig(ctx context.Context, req *ReloadConfigRequest) (*ControlResponse, error) {
	start := time.Now()
	s.logger.Info("%s : received ReloadConfig request", s.Name)
	if err := s.controller.ReloadConfig(ctx); err != nil {
		toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "ReloadConfig", "rpc", "inbound", "reload", 0, float64(time.Since(start).Microseconds())/1000.0, true, false)
		return &ControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to reload config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "RELOAD_FAILED",
		}, nil
	}

	toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "ReloadConfig", "rpc", "inbound", "reload", 0, float64(time.Since(start).Microseconds())/1000.0, false, false)
	return &ControlResponse{
		Success:   true,
		Message:   "Configuration reloaded from disk",
		Timestamp: time.Now().Unix(),
	}, nil
}

// PersistConfig commits the current memory state synchronously to disk
func (s *ControlServiceImpl) PersistConfig(ctx context.Context, req *PersistConfigRequest) (*ControlResponse, error) {
	start := time.Now()
	s.logger.Info("%s : received PersistConfig request", s.Name)
	if err := s.controller.PersistConfig(ctx); err != nil {
		toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "PersistConfig", "rpc", "inbound", "persist", 0, float64(time.Since(start).Microseconds())/1000.0, true, false)
		return &ControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to persist config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "PERSIST_FAILED",
		}, nil
	}

	toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "PersistConfig", "rpc", "inbound", "persist", 0, float64(time.Since(start).Microseconds())/1000.0, false, false)
	return &ControlResponse{
		Success:   true,
		Message:   "Configuration persisted synchronously to disk",
		Timestamp: time.Now().Unix(),
	}, nil
}

// GetStatus returns server health and metadata
func (s *ControlServiceImpl) GetStatus(ctx context.Context, req *GetStatusRequest) (*GetStatusResponse, error) {
	start := time.Now()
	s.logger.Debug("%s : received GetStatus request", s.Name)

	statusInfo, err := s.controller.GetStatus(ctx)
	if err != nil {
		toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "GetStatus", "rpc", "inbound", "status", 0, float64(time.Since(start).Microseconds())/1000.0, true, false)
		return &GetStatusResponse{
			Healthy:   false,
			Status:    "Error",
			Timestamp: time.Now().Unix(),
		}, err
	}

	toolbox_wiretap.RecordEvent("grpc_client", "config_server", "GRPC", "Protobuf", "GetStatus", "rpc", "inbound", "status", 0, float64(time.Since(start).Microseconds())/1000.0, false, false)
	return &GetStatusResponse{
		Healthy:       statusInfo.Healthy,
		Status:        statusInfo.Status,
		Version:       statusInfo.Version,
		Timestamp:     statusInfo.Timestamp,
		ActiveClients: int32(statusInfo.ActiveClients),
		ClientNames:   statusInfo.ClientNames,
		StoreVersion:  statusInfo.StoreVersion,
	}, nil
}
