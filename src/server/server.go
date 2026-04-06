package server

import (
	"encoding/json"
	"os"
	"sync"

	"config-server/src/store"

	distributed_config "github.com/Bastien-Antigravity/distributed-config"
	config "github.com/Bastien-Antigravity/distributed-config/src/schemas"
	logger_interfaces "github.com/Bastien-Antigravity/flexible-logger/src/interfaces"
	factory "github.com/Bastien-Antigravity/safe-socket"
	socket_interfaces "github.com/Bastien-Antigravity/safe-socket/src/interfaces"

	"google.golang.org/protobuf/proto"
)

// Server represents the Config Server.
type Server struct {
	Logger        logger_interfaces.Logger
	Store         *store.Store
	Persistence   *store.PersistenceManager
	Config        *distributed_config.Config
	listeners     map[string]socket_interfaces.TransportConnection
	listenersLock sync.RWMutex
	shutdown      chan struct{}
}

// -----------------------------------------------------------------------------

// NewServer creates a new Config Server.
func NewServer(conf *distributed_config.Config, logger logger_interfaces.Logger, s *store.Store, pm *store.PersistenceManager) *Server {
	return &Server{
		Config:      conf,
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
	// Resolve address from config capabilities
	var cs map[string]interface{}
	if val, ok := s.Config.Capabilities["config_server"]; ok {
		cs, _ = val.(map[string]interface{})
	}
	ip, _ := cs["ip"].(string)
	port, _ := cs["port"].(string)

	if ip == "" || port == "" {
		s.Logger.Error("Config for ConfigServer capabilities not found or invalid")
		os.Exit(1)
	}

	addr := ip + ":" + port

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
	s.broadcastUpdate(config.ConfigMsg_BROADCAST_REGISTRY, payload)
}

// -----------------------------------------------------------------------------

// broadcastUpdate sends configuration updates to all connected clients.
func (s *Server) broadcastUpdate(cmd config.ConfigMsg_Cmd, payload []byte) {
	s.listenersLock.RLock()
	defer s.listenersLock.RUnlock()

	msg := &config.ConfigMsg{
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
