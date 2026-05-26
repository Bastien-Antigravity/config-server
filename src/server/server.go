package server

import (
	"encoding/json"
	"os"
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

// clientMailbox represents a dedicated outgoing queue for a client.
type clientMailbox struct {
	name string
	send chan []byte
}

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
}

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
		s.Logger.Error("Failed to resolve bind address: %v", err)
		os.Exit(1)
	}

	// Create a server socket using safe-socket factory
	// We use "tcp-hello" profile which automatically handles the Handshake
	serverSock, err := factory.Create("tcp-hello", addr, "127.0.0.1", "server", true)
	if err != nil {
		return err // Wrap error in caller if needed, or return raw err
	}
	defer serverSock.Close()

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
			// Perform a final save if dirty during shutdown
			if s.dirty.Swap(false) {
				s.Logger.Info("Final shutdown persistence save...")
				_ = s.Persistence.Save(s.Store.Get())
			}
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
	for name := range s.listeners {
		clients = append(clients, name)
	}
	s.listenersLock.RUnlock()

	registry["active_services"] = clients

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
		// If the channel is full, we close it to force a disconnection.
		select {
		case mailbox.send <- bytes:
			// Message queued
		default:
			s.Logger.Warning("Client %s mailbox full. Dropping connection to maintain sync.", name)
			// Closing the channel signals the writer loop to exit and close the socket.
			close(mailbox.send)
		}
	}
}
