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
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/store"

	schemas "github.com/Bastien-Antigravity/distributed-config/src/schemas"
	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	factory "github.com/Bastien-Antigravity/safe-socket"
	socket_interfaces "github.com/Bastien-Antigravity/safe-socket/src/interfaces"
	"github.com/Bastien-Antigravity/universal-logger/src/interfaces"

	"google.golang.org/protobuf/proto"
)

// -----------------------------------------------------------------------------

// clientMailbox represents a dedicated outgoing queue for a client.
type clientMailbox struct {
	name           string
	serviceAddress string
	sock           socket_interfaces.TransportConnection
	send           chan []byte
	done           chan struct{}
	closeOnce      sync.Once
	needsRefresh   atomic.Bool
}

func newClientMailbox(name, serviceAddress string, sock socket_interfaces.TransportConnection, bufferSize int) *clientMailbox {
	return &clientMailbox{
		name:           name,
		serviceAddress: serviceAddress,
		sock:           sock,
		send:           make(chan []byte, bufferSize),
		done:           make(chan struct{}),
	}
}

func (m *clientMailbox) ensureInit() {
	if m.done == nil {
		m.done = make(chan struct{})
	}
}

// Close signals the mailbox to stop accepting or draining messages without closing send.
func (m *clientMailbox) Close() {
	if m == nil {
		return
	}
	m.closeOnce.Do(func() {
		if m.done != nil {
			close(m.done)
		}
		if m.sock != nil {
			_ = m.sock.Close()
		}
	})
}

// -----------------------------------------------------------------------------

// Server represents the Config Server.
type Server struct {
	Logger        interfaces.Logger
	Store         *store.Store
	Persistence   *store.PersistenceManager
	AppConfig     *utilconf.AppConfig // Toolbox Config
	listeners     atomic.Pointer[map[string]*clientMailbox]
	listenersLock sync.Mutex
	broadcastMu   sync.Mutex
	shutdown      chan struct{}
	dirty         atomic.Bool
	onUpdate      atomic.Pointer[func()]
	serverSock    atomic.Pointer[factory.Socket]
}

// SetOnUpdate atomically registers the callback invoked when configuration state changes.
func (s *Server) SetOnUpdate(cb func()) {
	if cb == nil {
		s.onUpdate.Store(nil)
		return
	}
	s.onUpdate.Store(&cb)
}

// -----------------------------------------------------------------------------

// NewServer creates a new Config Server.
func NewServer(ac *utilconf.AppConfig, logger interfaces.Logger, st *store.Store, pm *store.PersistenceManager) *Server {
	srv := &Server{
		AppConfig:   ac,
		Logger:      logger,
		Store:       st,
		Persistence: pm,
		shutdown:    make(chan struct{}),
	}
	initialListeners := make(map[string]*clientMailbox)
	srv.listeners.Store(&initialListeners)
	return srv
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
	s.serverSock.Store(&serverSock)

	defer func() {
		if old := s.serverSock.Swap(nil); old != nil && *old != nil {
			_ = (*old).Close()
		}
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

// Stop signals the server to shutdown and closes active client transports.
func (s *Server) Stop() {
	s.Logger.Info("Stopping Config Server...")
	select {
	case <-s.shutdown:
	default:
		close(s.shutdown)
	}

	if sockPtr := s.serverSock.Load(); sockPtr != nil && *sockPtr != nil {
		_ = (*sockPtr).Close()
	}

	s.listenersLock.Lock()
	ptr := s.listeners.Load()
	var active []*clientMailbox
	if ptr != nil {
		for _, mb := range *ptr {
			active = append(active, mb)
		}
		empty := make(map[string]*clientMailbox)
		s.listeners.Store(&empty)
	}
	s.listenersLock.Unlock()

	for _, mb := range active {
		mb.Close()
	}
}

// -----------------------------------------------------------------------------

// BroadcastConfig sends a delta/merge configuration update to all connected clients.
func (s *Server) BroadcastConfig() {
	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()
	snap, ver := s.Store.GetWithVersion()
	s.broadcastConfigLocked(snap, ver)
}

func (s *Server) broadcastConfigWithVersion(snap store.ConfigMap, version uint64) {
	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()
	s.broadcastConfigLocked(snap, version)
}

func (s *Server) broadcastConfigLocked(snap store.ConfigMap, version uint64) {
	s.Logger.Info("Broadcasting configuration update to all clients (version: %d)", version)
	if cbPtr := s.onUpdate.Load(); cbPtr != nil && *cbPtr != nil {
		go (*cbPtr)()
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		s.Logger.Error("Failed to marshal config for broadcast: %v", err)
		return
	}
	s.broadcastUpdateLocked(schemas.ConfigMsg_BROADCAST_SYNC, payload, version)
}

// BroadcastFullRefresh sends a full replacement configuration snapshot to all connected clients.
// Used when keys are deleted or configuration is reloaded from disk so clients purge removed keys.
func (s *Server) BroadcastFullRefresh() {
	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()
	snap, ver := s.Store.GetWithVersion()
	s.broadcastFullRefreshLocked(snap, ver)
}

func (s *Server) broadcastFullRefreshWithVersion(snap store.ConfigMap, version uint64) {
	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()
	s.broadcastFullRefreshLocked(snap, version)
}

func (s *Server) broadcastFullRefreshLocked(snap store.ConfigMap, version uint64) {
	s.Logger.Info("Broadcasting full configuration refresh to all clients (version: %d)", version)
	if cbPtr := s.onUpdate.Load(); cbPtr != nil && *cbPtr != nil {
		go (*cbPtr)()
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		s.Logger.Error("Failed to marshal config for full refresh: %v", err)
		return
	}
	s.broadcastUpdateLocked(schemas.ConfigMsg_FULL_REFRESH, payload, version)
}

// GetActiveClients returns the current number of connected clients lock-free.
func (s *Server) GetActiveClients() int {
	ptr := s.listeners.Load()
	if ptr == nil {
		return 0
	}
	return len(*ptr)
}

// GetClientNames returns the names of all currently connected clients lock-free in sorted order.
func (s *Server) GetClientNames() []string {
	ptr := s.listeners.Load()
	if ptr == nil {
		return nil
	}
	current := *ptr
	names := make([]string, 0, len(current))
	for name := range current {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ReloadConfig reloads the configuration from the base YAML file and persistence store,
// re-merging AppConfig baseline capabilities before broadcasting a FULL_REFRESH.
func (s *Server) ReloadConfig(ctx context.Context) error {
	s.Logger.Info("Reloading configuration from disk...")

	if err := s.AppConfig.Config.Reload(); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	var persisted store.ConfigMap
	var persistedVer uint64
	if s.Persistence != nil {
		loaded, ver, err := s.Persistence.LoadWithVersion()
		if err != nil {
			s.Logger.Error("Failed to load persisted store during reload: %v", err)
			return fmt.Errorf("failed to load persisted store: %w", err)
		}
		persisted = loaded
		persistedVer = ver
	}

	merged := s.BuildBaselineConfig(persisted)
	s.broadcastMu.Lock()
	ver := s.Store.ReplaceWithVersion(merged, persistedVer)
	s.Logger.Info("Config reloaded successfully (version: %d)", ver)
	s.broadcastFullRefreshLocked(merged, ver)
	s.broadcastMu.Unlock()
	return nil
}

// -----------------------------------------------------------------------------

// addListener adds a client mailbox to the broadcast list via atomic pointer swap.
func (s *Server) addListener(name string, mailbox *clientMailbox) {
	s.listenersLock.Lock()
	mailbox.ensureInit()
	oldPtr := s.listeners.Load()
	next := make(map[string]*clientMailbox)
	if oldPtr != nil {
		for k, v := range *oldPtr {
			next[k] = v
		}
	}
	if old, exists := next[name]; exists && old != mailbox {
		old.Close()
		s.Logger.Info("Replaced existing listener for %s", name)
	}
	next[name] = mailbox
	s.listeners.Store(&next)
	s.listenersLock.Unlock()
	go s.broadcastRegistry()
}

// -----------------------------------------------------------------------------

// removeListener removes a client from the broadcast list via atomic pointer swap.
func (s *Server) removeListener(name string, mailbox *clientMailbox) {
	s.listenersLock.Lock()
	oldPtr := s.listeners.Load()
	if oldPtr != nil {
		if current, ok := (*oldPtr)[name]; ok && current == mailbox {
			next := make(map[string]*clientMailbox, len(*oldPtr)-1)
			for k, v := range *oldPtr {
				if k != name {
					next[k] = v
				}
			}
			s.listeners.Store(&next)
			mailbox.Close()
			s.Logger.Info("Listener removed: %s", name)
		}
	}
	s.listenersLock.Unlock()
	go s.broadcastRegistry()
}

// -----------------------------------------------------------------------------

// TriggerSave marks the state as dirty to trigger background persistence.
func (s *Server) TriggerSave() {
	s.dirty.Store(true)
}

// SaveSync immediately commits the current store state and version to disk synchronously, clearing dirty status.
func (s *Server) SaveSync() error {
	if s.Persistence == nil {
		return nil
	}
	s.dirty.Store(false)
	snap, ver := s.Store.GetWithVersion()
	if err := s.Persistence.SaveWithVersion(snap, ver); err != nil {
		s.dirty.Store(true)
		return err
	}
	return nil
}

func (s *Server) flushDirtyOnce() {
	if s.dirty.Swap(false) {
		if s.Persistence == nil {
			return
		}
		s.Logger.Info("Background persistence: Saving dirty state...")
		snap, ver := s.Store.GetWithVersion()
		if err := s.Persistence.SaveWithVersion(snap, ver); err != nil {
			s.dirty.Store(true)
			s.Logger.Error("Background persistence failed: %v", err)
		}
	}
}

// persistenceWorker periodically saves the configuration to disk if dirty.
func (s *Server) persistenceWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flushDirtyOnce()
		case <-s.shutdown:
			// Loop exit only. Final save is handled by the Lifecycle Manager in main.go
			// to ensure exclusive, non-simultaneous access.
			s.Logger.Info("Persistence worker exiting.")
			return
		}
	}
}

// -----------------------------------------------------------------------------

// broadcastRegistry builds and broadcasts the active client registry deterministically under s.broadcastMu.
func (s *Server) broadcastRegistry() {
	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()

	ptr := s.listeners.Load()
	registry := make(map[string][]string)
	var clients []string
	var addresses []string
	if ptr != nil {
		for name, mb := range *ptr {
			clients = append(clients, name)
			if mb != nil && mb.serviceAddress != "" {
				addresses = append(addresses, fmt.Sprintf("%s=%s", name, mb.serviceAddress))
			}
		}
	}

	sort.Strings(clients)
	sort.Strings(addresses)

	registry["active_services"] = clients
	if len(addresses) > 0 {
		registry["service_addresses"] = addresses
	}

	payload, err := json.Marshal(registry)
	if err != nil {
		s.Logger.Error("Failed to marshal registry map: %v", err)
		return
	}
	s.broadcastUpdateLocked(schemas.ConfigMsg_BROADCAST_REGISTRY, payload, s.Store.GetVersion())
}

// -----------------------------------------------------------------------------

// broadcastUpdate sends configuration updates using the current store version.
func (s *Server) broadcastUpdate(cmd schemas.ConfigMsg_Cmd, payload []byte) {
	s.broadcastUpdateWithVersion(cmd, payload, s.Store.GetVersion())
}

// broadcastUpdateWithVersion serializes mailbox enqueueing across concurrent mutators.
func (s *Server) broadcastUpdateWithVersion(cmd schemas.ConfigMsg_Cmd, payload []byte, version uint64) {
	s.broadcastMu.Lock()
	defer s.broadcastMu.Unlock()
	s.broadcastUpdateLocked(cmd, payload, version)
}

func (s *Server) broadcastUpdateLocked(cmd schemas.ConfigMsg_Cmd, payload []byte, version uint64) {
	ptr := s.listeners.Load()
	if ptr == nil {
		return
	}

	verInt32 := int32(version)
	msg := &schemas.ConfigMsg{
		Command: cmd,
		Payload: payload,
		Version: verInt32,
	}
	bytes, err := proto.Marshal(msg)
	if err != nil {
		s.Logger.Error("Broadcast marshal error: %v", err)
		return
	}

	var fullRefreshBytes []byte
	getFullRefreshBytes := func() []byte {
		if fullRefreshBytes == nil {
			fullPayload, _ := json.Marshal(s.Store.Get())
			fullMsg := &schemas.ConfigMsg{
				Command: schemas.ConfigMsg_FULL_REFRESH,
				Payload: fullPayload,
				Version: verInt32,
			}
			fullRefreshBytes, _ = proto.Marshal(fullMsg)
		}
		return fullRefreshBytes
	}

	for name, mailbox := range *ptr {
		select {
		case <-mailbox.done:
			continue
		default:
		}

		targetBytes := bytes

		// If this client had a prior overflow, elevate to FULL_REFRESH to restore parity
		if mailbox.needsRefresh.Load() {
			if fb := getFullRefreshBytes(); fb != nil {
				targetBytes = fb
			}
		}

		select {
		case <-mailbox.done:
			continue
		case mailbox.send <- targetBytes:
			// Message queued successfully; clear overflow recovery flag if sent
			if mailbox.needsRefresh.Load() {
				mailbox.needsRefresh.Store(false)
			}
		default:
			// Buffer transiently full: flag for FULL_REFRESH on next broadcast without dropping connection
			s.Logger.Warning("Client %s mailbox full (transient burst). Flagged for FULL_REFRESH recovery on next message.", name)
			mailbox.needsRefresh.Store(true)
		}
	}
}
