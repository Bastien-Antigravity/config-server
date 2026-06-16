package grpc_control

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/core"
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
	s.logger.Debug("%s : received GetConfig request for [%s] %s", s.Name, req.Section, req.Key)

	val, ok, err := s.controller.GetConfig(ctx, req.Section, req.Key)
	if err != nil {
		return &GetConfigResponse{
			Success:   false,
			Value:     "",
			Timestamp: time.Now().Unix(),
		}, err
	}

	return &GetConfigResponse{
		Success:   ok,
		Value:     val,
		Timestamp: time.Now().Unix(),
	}, nil
}

// SetConfig updates a configuration value atomically
func (s *ControlServiceImpl) SetConfig(ctx context.Context, req *SetConfigRequest) (*ControlResponse, error) {
	s.logger.Info("%s : received SetConfig request for [%s] %s = %s", s.Name, req.Section, req.Key, req.Value)

	err := s.controller.SetConfig(ctx, req.Section, req.Key, req.Value)
	if err != nil {
		return &ControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "UPDATE_FAILED",
		}, nil
	}

	return &ControlResponse{
		Success:   true,
		Message:   "Configuration updated and broadcasted successfully",
		Timestamp: time.Now().Unix(),
	}, nil
}

// ListConfig returns the entire configuration state as JSON
func (s *ControlServiceImpl) ListConfig(ctx context.Context, req *ListConfigRequest) (*ListConfigResponse, error) {
	s.logger.Debug("%s : received ListConfig request", s.Name)

	config, err := s.controller.ListConfig(ctx)
	if err != nil {
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

	return &ListConfigResponse{
		Success:    true,
		JsonConfig: string(data),
		Timestamp:  time.Now().Unix(),
	}, nil
}

// ReloadConfig reloads configuration from the base YAML
func (s *ControlServiceImpl) ReloadConfig(ctx context.Context, req *ReloadConfigRequest) (*ControlResponse, error) {
	s.logger.Info("%s : received ReloadConfig request", s.Name)
	if err := s.controller.ReloadConfig(ctx); err != nil {
		return &ControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to reload config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "RELOAD_FAILED",
		}, nil
	}

	return &ControlResponse{
		Success:   true,
		Message:   "Configuration reloaded from disk",
		Timestamp: time.Now().Unix(),
	}, nil
}

// PersistConfig manually triggers a save of the current memory state
func (s *ControlServiceImpl) PersistConfig(ctx context.Context, req *PersistConfigRequest) (*ControlResponse, error) {
	s.logger.Info("%s : received PersistConfig request", s.Name)
	if err := s.controller.PersistConfig(ctx); err != nil {
		return &ControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to persist config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "PERSIST_FAILED",
		}, nil
	}
	return &ControlResponse{
		Success:   true,
		Message:   "Persistence trigger activated",
		Timestamp: time.Now().Unix(),
	}, nil
}

// GetStatus returns server health and metadata
func (s *ControlServiceImpl) GetStatus(ctx context.Context, req *GetStatusRequest) (*GetStatusResponse, error) {
	s.logger.Debug("%s : received GetStatus request", s.Name)

	statusInfo, err := s.controller.GetStatus(ctx)
	if err != nil {
		return &GetStatusResponse{
			Healthy:   false,
			Status:    "Error",
			Timestamp: time.Now().Unix(),
		}, err
	}

	return &GetStatusResponse{
		Healthy:       statusInfo.Healthy,
		Status:        statusInfo.Status,
		Version:       statusInfo.Version,
		Timestamp:     statusInfo.Timestamp,
		ActiveClients: int32(statusInfo.ActiveClients),
		ClientNames:   statusInfo.ClientNames,
	}, nil
}
