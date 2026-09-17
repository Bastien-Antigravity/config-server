package server

// =============================================================================
// ESSENTIAL PROCESS:
// Implements the central Config Server TCP network daemon, managing client
// mailboxes, broadcast channels, registry notifications, and background state saves.
//
// DATA FLOW:
// 1. Input: Incoming TCP connections authenticated via SafeSocket tcp-hello handshake.
// 2. Logic: Enqueues outbound configuration diffs into non-blocking client mailboxes,
//    broadcasts dynamic service registries, and periodically flushes dirty store state.
// 3. Output: Outgoing TCP frames carrying protobuf ConfigMsg payloads.
//
// KEY PARAMETERS:
// - AppConfig: Ecosystem configuration facade resolving dynamic listen addresses.
// - Store: In-memory Copy-On-Write store holding configuration tree.
// - Persistence: Atomic JSON file persistence manager.
// =============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/store"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	factory "github.com/Bastien-Antigravity/safe-socket"

	schemas "github.com/Bastien-Antigravity/distributed-config/src/schemas"
	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"

	"google.golang.org/protobuf/proto"
)

// -----------------------------------------------------------------------------

// clientMailbox represents a dedicated outgoing queue for a client.
type clientMailbox struct {
	name           string
	serviceAddress string
	send           chan []byte
}

// -----------------------------------------------------------------------------

// Server represents the Config Server.
type Server struct {
	Logger        interfaces.Logger
	Store         *store.Store
	Persistence   *store.PersistenceManager
	AppConfig     *utilconf.AppConfig // Toolbox Config
	listeners     map[string]*clientMailbox
	listenersLock sync.RWMutex
	shutdown      chan struct{}
	dirty         atomic.Bool
	OnUpdate      func()
	serverSock    factory.Socket
	sockLock      sync.Mutex
}

// -----------------------------------------------------------------------------

// NewServer creates a new Config Server.
func NewServer(ac *utilconf.AppConfig, logger interfaces.Logger, s *store.Store, pm *store.PersistenceManager) *Server {
	return &Server{
		AppConfig:   ac,
		Logger:      logger,
		Store:       s,
		Persistence: pm,
		listeners:   make(map[string]*clientMailbox),
		shutdown:    make(chan struct{}),
	}
}

// -----------------------------------------------------------------------------

// Start listens for incoming TCP connections.
func (s *Server) Start() error {
	// Start Background Persistence Worker
	go s.persistenceWorker()

	// Use Toolbox Smart Resolver for Binding
	addr, err := s.AppConfig.GetListenAddr("config_server")
	if err != nil {
		return fmt.Errorf("failed to resolve bind address for config_server: %w", err)
	}

	// Create a server socket using safe-socket factory
	// We use "tcp-hello" profile which automatically handles the Handshake
	// We configure a 10-minute idle timeout for all accepted connections.
	config := factory.SocketConfig{
		Deadline: 10 * time.Minute,
	}
	serverSock, err := factory.CreateWithConfig("tcp-hello", addr, config, "server", true)
	if err != nil {
		return err
	}
	s.sockLock.Lock()
	s.serverSock = serverSock
	s.sockLock.Unlock()

	defer func() {
		s.sockLock.Lock()
		if s.serverSock != nil {
			_ = s.serverSock.Close()
			s.serverSock = nil
		}
		s.sockLock.Unlock()
	}()

	s.Logger.Info("Config Server listening on " + addr)

	for {
		conn, err := serverSock.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				return nil
			default:
				s.Logger.Error("Accept error: %v", err)
				continue
			}
		}
		go s.handleConnection(conn)
	}
}

// -----------------------------------------------------------------------------

// Stop signals the server to shutdown.
func (s *Server) Stop() {
	s.Logger.Info("Stopping Config Server...")
	select {
	case <-s.shutdown:
	default:
		close(s.shutdown)
	}

	s.sockLock.Lock()
	if s.serverSock != nil {
		_ = s.serverSock.Close()
	}
	s.sockLock.Unlock()
}

// -----------------------------------------------------------------------------

// BroadcastConfig sends a full configuration update to all connected clients.
func (s *Server) BroadcastConfig() {
	s.Logger.Info("Broadcasting full configuration update to all clients")
	if s.OnUpdate != nil {
		go s.OnUpdate()
	}
	config := s.Store.Get()
	payload, err := json.Marshal(config)
	if err != nil {
		s.Logger.Error("Failed to marshal config for broadcast: %v", err)
		return
	}
	s.broadcastUpdate(schemas.ConfigMsg_BROADCAST_SYNC, payload)
}

// GetActiveClients returns the current number of connected clients.
func (s *Server) GetActiveClients() int {
	s.listenersLock.RLock()
	defer s.listenersLock.RUnlock()
	return len(s.listeners)
}

// GetClientNames returns the names of all currently connected clients.
func (s *Server) GetClientNames() []string {
	s.listenersLock.RLock()
	defer s.listenersLock.RUnlock()
	names := make([]string, 0, len(s.listeners))
	for name := range s.listeners {
		names = append(names, name)
	}
	return names
}

// ReloadConfig reloads the configuration from the base YAML file.
func (s *Server) ReloadConfig(ctx context.Context) error {
	s.Logger.Info("Reloading configuration from disk...")

	if err := s.AppConfig.Config.Reload(); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	s.Logger.Info("Config reloaded successfully")
	s.BroadcastConfig()
	return nil
}

// -----------------------------------------------------------------------------

// addListener adds a client mailbox to the broadcast list.
func (s *Server) addListener(name string, mailbox *clientMailbox) {
	s.listenersLock.Lock()
	s.listeners[name] = mailbox
	s.listenersLock.Unlock()
	go s.broadcastRegistry()
}

// -----------------------------------------------------------------------------

// removeListener removes a client from the broadcast list only if it's the same instance.
func (s *Server) removeListener(name string, mailbox *clientMailbox) {
	s.listenersLock.Lock()
	if current, ok := s.listeners[name]; ok && current == mailbox {
		delete(s.listeners, name)
		close(mailbox.send) // Safe to close here as we removed it from the map under lock
		s.Logger.Info("Listener removed: %s", name)
	}
	s.listenersLock.Unlock()
	go s.broadcastRegistry()
}

// -----------------------------------------------------------------------------

// TriggerSave marks the state as dirty to trigger background persistence.
func (s *Server) TriggerSave() {
	s.dirty.Store(true)
}

// persistenceWorker periodically saves the configuration to disk if dirty.
func (s *Server) persistenceWorker() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.dirty.Swap(false) {
				s.Logger.Info("Background persistence: Saving dirty state...")
				if err := s.Persistence.Save(s.Store.Get()); err != nil {
					s.Logger.Error("Background persistence failed: %v", err)
				}
			}
		case <-s.shutdown:
			// Loop exit only. Final save is handled by the Lifecycle Manager in main.go
			// to ensure exclusive, non-simultaneous access.
			s.Logger.Info("Persistence worker exiting.")
			return
		}
	}
}

// -----------------------------------------------------------------------------

// broadcastRegistry sends the list of all connected active clients.
func (s *Server) broadcastRegistry() {
	s.listenersLock.RLock()
	registry := make(map[string][]string)
	var clients []string
	var addresses []string
	for name, mb := range s.listeners {
		clients = append(clients, name)
		if mb != nil && mb.serviceAddress != "" {
			addresses = append(addresses, fmt.Sprintf("%s=%s", name, mb.serviceAddress))
		}
	}
	s.listenersLock.RUnlock()

	registry["active_services"] = clients
	if len(addresses) > 0 {
		registry["service_addresses"] = addresses
	}

	payload, err := json.Marshal(registry)
	if err != nil {
		s.Logger.Error("Failed to marshal registry map: %v", err)
		return
	}
	s.broadcastUpdate(schemas.ConfigMsg_BROADCAST_REGISTRY, payload)
}

// -----------------------------------------------------------------------------

// broadcastUpdate sends configuration updates to all connected client mailboxes.
func (s *Server) broadcastUpdate(cmd schemas.ConfigMsg_Cmd, payload []byte) {
	s.listenersLock.RLock()
	defer s.listenersLock.RUnlock()

	msg := &schemas.ConfigMsg{
		Command: cmd,
		Payload: payload,
	}
	bytes, err := proto.Marshal(msg)
	if err != nil {
		s.Logger.Error("Broadcast marshal error: %v", err)
		return
	}

	for name, mailbox := range s.listeners {
		// Non-blocking send to mailbox.
		// If the channel is full, we drop the message for that client to avoid blocking.
		// The client will eventually be cleaned up by the idle timeout if it remains stuck.
		select {
		case mailbox.send <- bytes:
			// Message queued
		default:
			s.Logger.Warning("Client %s mailbox full. Dropping message.", name)
		}
	}
}
