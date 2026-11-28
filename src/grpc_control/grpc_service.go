package grpc_control

// =============================================================================
// ESSENTIAL PROCESS:
// Manages the gRPC server lifecycle on the Shadow Port (3307), registering
// ConfigControlServiceServer and health checks for remote management.
//
// DATA FLOW:
// 1. Input: TCP listener bound to AppConfig's gRPC shadow address.
// 2. Logic: Registers ControlServiceImpl and gRPC health server, manages graceful shutdown.
// 3. Output: Binary gRPC stream endpoints serving configuration RPCs.
//
// KEY PARAMETERS:
// - GRPCService: Lifecycle manager wrapping *grpc.Server and net.Listener.
// - ControlService: Implementation of ConfigControlServiceServer.
// =============================================================================

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/Bastien-Antigravity/config-server/src/server"

	unilog_interfaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// -----------------------------------------------------------------------------
// GRPCService handles gRPC server lifecycle
// -----------------------------------------------------------------------------

type GRPCService struct {
	mu       sync.Mutex
	server   *grpc.Server
	listener net.Listener
	logger   unilog_interfaces.Logger
	srv      *server.Server
	running  atomic.Bool

	ControlService *ControlServiceImpl
}

// -----------------------------------------------------------------------------

// NewGRPCService creates a new GRPCService instance
func NewGRPCService(srv *server.Server, logger unilog_interfaces.Logger) (*GRPCService, error) {
	// Use Toolbox Smart Resolver for Binding (Shadow Port Protocol)
	addr, err := srv.AppConfig.GetGRPCListenAddr("config_server")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve gRPC address: %w", err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// Create gRPC server with options
	serverOptions := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(10 * 1024 * 1024), // 10MB
		grpc.MaxSendMsgSize(10 * 1024 * 1024), // 10MB
	}

	server := grpc.NewServer(serverOptions...)

	return &GRPCService{
		server:   server,
		listener: listener,
		logger:   logger,
		srv:      srv,
	}, nil
}

// -----------------------------------------------------------------------------

// Start starts the gRPC server (Non-blocking)
func (g *GRPCService) Start() error {
	g.mu.Lock()
	g.logger.Info("Starting gRPC management service on %s", g.listener.Addr().String())

	// Register services
	g.ControlService = NewControlService(g.srv, g.logger)
	RegisterConfigControlServiceServer(g.server, g.ControlService)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(g.server, healthServer)
	healthServer.SetServingStatus("grpc_control.ConfigControlService", grpc_health_v1.HealthCheckResponse_SERVING)

	srv := g.server
	ln := g.listener
	g.running.Store(true)
	g.mu.Unlock()

	// Start server in goroutine
	go func() {
		defer g.running.Store(false)
		if err := srv.Serve(ln); err != nil && err != grpc.ErrServerStopped {
			g.logger.Error("gRPC management server failed: %v", err)
		}
	}()

	g.logger.Info("gRPC management service initialized successfully on %s", ln.Addr().String())
	return nil
}

// -----------------------------------------------------------------------------

// Stop gracefully stops the gRPC server
func (g *GRPCService) Stop(ctx context.Context) error {
	g.logger.Info("Stopping gRPC management service...")
	g.mu.Lock()
	srv := g.server
	ln := g.listener
	g.mu.Unlock()

	if srv != nil {
		// Graceful stop
		done := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(done)
		}()

		select {
		case <-ctx.Done():
			g.logger.Warning("gRPC graceful shutdown timeout, forcing stop...")
			srv.Stop()
		case <-done:
			g.logger.Info("gRPC management service stopped gracefully")
		}
	}

	if ln != nil {
		_ = ln.Close()
	}

	g.running.Store(false)
	return nil
}

// -----------------------------------------------------------------------------

// IsRunning returns whether the gRPC server is running
func (g *GRPCService) IsRunning() bool {
	return g.running.Load()
}
