package server

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/Bastien-Antigravity/config-server/src/store"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	factory "github.com/Bastien-Antigravity/safe-socket"
	socket_interfaces "github.com/Bastien-Antigravity/safe-socket/src/interfaces"

	schemas "github.com/Bastien-Antigravity/distributed-config/src/schemas"
	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"

	"google.golang.org/protobuf/proto"
)

// Server represents the Config Server.
type Server struct {
	Logger        interfaces.Logger
	Store         *store.Store
	Persistence   *store.PersistenceManager
	AppConfig     *utilconf.AppConfig // Toolbox Config
	listeners     map[string]socket_interfaces.TransportConnection
	listenersLock sync.RWMutex
	shutdown      chan struct{}
}

// NewServer creates a new Config Server.
func NewServer(ac *utilconf.AppConfig, logger interfaces.Logger, s *store.Store, pm *store.PersistenceManager) *Server {
	return &Server{
		AppConfig:   ac,
		Logger:      logger,
		Store:       s,
		Persistence: pm,
		listeners:   make(map[string]socket_interfaces.TransportConnection),
		shutdown:    make(chan struct{}),
	}
}

// -----------------------------------------------------------------------------

// Start listens for incoming TCP connections.
func (s *Server) Start() error {
	// Use Toolbox Smart Resolver for Binding
	addr, err := s.AppConfig.GetListenAddr("config_server")
	if err != nil {
		s.Logger.Error("Failed to resolve bind address: " + err.Error())
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
				s.Logger.Error("Accept error: " + err.Error())
				continue
			}
		}
		go s.handleConnection(conn)
	}
}

// -----------------------------------------------------------------------------

// addListener adds a client to the broadcast list.
func (s *Server) addListener(name string, sock socket_interfaces.TransportConnection) {
	s.listenersLock.Lock()
	s.listeners[name] = sock
	s.listenersLock.Unlock()
	go s.broadcastRegistry()
}

// -----------------------------------------------------------------------------

// removeListener removes a client from the broadcast list.
func (s *Server) removeListener(name string) {
	s.listenersLock.Lock()
	delete(s.listeners, name)
	s.listenersLock.Unlock()
	go s.broadcastRegistry()
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
		s.Logger.Error("Failed to marshal registry map: " + err.Error())
		return
	}
	s.broadcastUpdate(schemas.ConfigMsg_BROADCAST_REGISTRY, payload)
}

// -----------------------------------------------------------------------------

// broadcastUpdate sends configuration updates to all connected clients.
func (s *Server) broadcastUpdate(cmd schemas.ConfigMsg_Cmd, payload []byte) {
	s.listenersLock.RLock()
	defer s.listenersLock.RUnlock()

	msg := &schemas.ConfigMsg{
		Command: cmd,
		Payload: payload,
	}
	bytes, err := proto.Marshal(msg)
	if err != nil {
		s.Logger.Error("Broadcast marshal error: " + err.Error())
		return
	}

	for name, sock := range s.listeners {
		go func(n string, sk socket_interfaces.TransportConnection) {
			sk.Write(bytes)
		}(name, sock)
	}
}
