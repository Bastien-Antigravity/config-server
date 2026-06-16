package grpc_control

import (
	"context"
	"fmt"
	"net"

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
	server   *grpc.Server
	listener net.Listener
	logger   unilog_interfaces.Logger
	srv      *server.Server
	running  bool

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
		running:  false,
	}, nil
}

// -----------------------------------------------------------------------------

// Start starts the gRPC server (Non-blocking)
func (g *GRPCService) Start() error {
	g.logger.Info("Starting gRPC management service on %s", g.listener.Addr().String())

	// Register services
	g.ControlService = NewControlService(g.srv, g.logger)
	RegisterConfigControlServiceServer(g.server, g.ControlService)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(g.server, healthServer)
	healthServer.SetServingStatus("grpc_control.ConfigControlService", grpc_health_v1.HealthCheckResponse_SERVING)

	// Start server in goroutine
	go func() {
		g.running = true
		if err := g.server.Serve(g.listener); err != nil && err != grpc.ErrServerStopped {
			g.logger.Error("gRPC management server failed: %v", err)
		}
		g.running = false
	}()

	g.logger.Info("gRPC management service initialized successfully on %s", g.listener.Addr().String())
	return nil
}

// -----------------------------------------------------------------------------

// Stop gracefully stops the gRPC server
func (g *GRPCService) Stop(ctx context.Context) error {
	g.logger.Info("Stopping gRPC management service...")

	if g.server != nil {
		// Graceful stop
		done := make(chan struct{})
		go func() {
			g.server.GracefulStop()
			close(done)
		}()

		select {
		case <-ctx.Done():
			g.logger.Warning("gRPC graceful shutdown timeout, forcing stop...")
			g.server.Stop()
		case <-done:
			g.logger.Info("gRPC management service stopped gracefully")
		}
	}

	if g.listener != nil {
		g.listener.Close()
	}

	g.running = false
	return nil
}

// -----------------------------------------------------------------------------

// IsRunning returns whether the gRPC server is running
func (g *GRPCService) IsRunning() bool {
	return g.running
}
