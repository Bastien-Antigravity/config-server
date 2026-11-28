package rest

// =============================================================================
// ESSENTIAL PROCESS:
// Builds exhaustive, microservice-authored Exchange Contracts for config-server
// across HTTP/REST, gRPC Control, SafeSocket TCP (ConfigMsg Protobuf), and
// outbound Tele-Remote streaming.
//
// DATA FLOW:
// 1. Reads embedded config_server.proto + compiled distributed-config Protobuf
//    reflection descriptors + live SafeSocket connected client roster.
// 2. Packages contracts via toolbox_spec.ExchangeContract for both
//    GET /api/v1/contracts and ServiceSpec PublishWithLifecycle.
//
// KEY PARAMETERS:
// - appConfig: Optional AppConfig resolving live TCP/REST/gRPC endpoints.
// - clientNames: Active SafeSocket TCP clients reported by Controller.
// =============================================================================

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Bastien-Antigravity/config-server/src/grpc_control"

	config_schemas "github.com/Bastien-Antigravity/distributed-config/src/schemas"
	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_spec "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/spec"
	toolbox_version "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/version"
)

// -----------------------------------------------------------------------------

// BuildExchangeContracts constructs the exhaustive contract catalog from live config-server sources.
func BuildExchangeContracts(appConfig *toolbox_config.AppConfig, clientNames []string) []toolbox_spec.ExchangeContract {
	var tcpAddr, restAddr, grpcAddr, teleAddr string
	if appConfig != nil {
		tcpAddr, _ = appConfig.GetListenAddr("config_server")
		restAddr, _ = appConfig.GetRESTAddr("config_server")
		grpcAddr, _ = appConfig.GetGRPCListenAddr("config_server")
		teleAddr, _ = appConfig.GetGRPCListenAddr("tele_remote")
		if teleAddr == "" {
			teleAddr, _ = appConfig.GetListenAddr("tele_remote")
		}
	}

	grpcOps := toolbox_spec.ExtractProtoTextOperations(grpc_control.ConfigServerProtoSchema)
	grpcOps = append(grpcOps, toolbox_spec.ContractOperation{
		Name:        "Check",
		Kind:        "RPC",
		Target:      "grpc.health.v1.Health/Check",
		Input:       "HealthCheckRequest { service }",
		Output:      "HealthCheckResponse { status }",
		Description: "Standard gRPC health probe registered on the control listener",
	})

	tcpPeers := []toolbox_spec.ContractPeer{
		{Service: "watchdog_agent", Role: "client", Description: "Pulls distributed config & receives COW broadcast pushes"},
		{Service: "notif_server", Role: "client", Description: "Synchronizes alerting rules & provider credentials"},
		{Service: "web_interface", Role: "client", Description: "Subscribes to capability & ecosystem configuration updates"},
		{Service: "tele_remote", Role: "client", Description: "Consumes Telegram & logging capability settings"},
		{Service: "log_server", Role: "client", Description: "Loads centralized logging ports via standalone/distributed config"},
		{Service: "rag_engine", Role: "client", Description: "Syncs PG/LLM configuration via C/CGO distributed-config client"},
		{Service: "agent_squad", Role: "client", Description: "Reads fleet capability addresses & branch settings"},
	}
	if len(clientNames) > 0 {
		tcpPeers = append(tcpPeers, toolbox_spec.ContractPeer{
			Service:     "Live Connected Mailboxes",
			Role:        "client",
			Description: fmt.Sprintf("Active clients: %s", strings.Join(clientNames, ", ")),
		})
	}

	configProtoSchema := toolbox_spec.RenderProtoIDL((&config_schemas.ConfigMsg{}).ProtoReflect().Descriptor().ParentFile())
	safeSocketSchema := fmt.Sprintf(`// 1. SafeSocket TCP Framing & HelloMsg Handshake
//    - Frame header: uint32 big-endian length prefix (0 = keepalive heartbeat, max 10MB)
//    - Initial frame: unpacked Cap'n Proto HelloMsg { fromName, fromHost, fromAddress, toAddress, fromPublicIP }
//    - Subsequent frames: binary protobuf ConfigMsg frames + server-pushed BROADCAST_SYNC updates

// 2. Compiled distributed-config Protobuf Payload
%s`, configProtoSchema)

	contracts := []toolbox_spec.ExchangeContract{
		{
			ID:           "rest",
			Type:         "HTTP/REST API",
			TabLabel:     "HTTP/REST API",
			Icon:         "fa-globe",
			Direction:    "inbound",
			Transport:    "HTTP/1.1 + CORS JSON",
			Address:      toolbox_spec.DisplayAddress(restAddr),
			Port:         toolbox_spec.ResolveContractPort(restAddr),
			Framing:      "HTTP JSON request/response with CORS headers",
			Auth:         "Internal mesh REST management",
			Summary:      "Exposes atomic ConfigStore inspection/mutation endpoints and serves the embedded OpenMFE <config-server-mfe> bundle.",
			Source:       "config-server/src/rest/rest_handler.go",
			SchemaFormat: "http",
			Schema: strings.Join([]string{
				"GET  /api/v1/status",
				"GET  /api/v1/contracts",
				"GET  /api/v1/config/list",
				"GET  /api/v1/config/get?section={section}&key={key}",
				"POST /api/v1/config/set        body: {\"section\": string, \"key\": string, \"value\": string, \"expected_version\": uint64?}",
				"POST /api/v1/config/delete     body: {\"section\": string, \"key\": string}",
				"POST /api/v1/config/reload",
				"POST /api/v1/config/persist",
				"GET  /static/mfe.js            (alias: /static/js/mfe-loader.js)",
			}, "\n"),
			Peers: []toolbox_spec.ContractPeer{
				{Service: "web_interface", Role: "consumer", Description: "Loads <config-server-mfe> Web Component and queries config REST APIs"},
			},
			Operations: []toolbox_spec.ContractOperation{
				{Name: "Status", Kind: "GET", Target: "/api/v1/status", Output: "{ healthy, status, version, store_version, timestamp, active_clients, client_names }", Description: "Live server status & SafeSocket client list"},
				{Name: "ListConfig", Kind: "GET", Target: "/api/v1/config/list", Output: "{ success, json_config, version, timestamp }", Description: "Returns full Copy-On-Write configuration tree as JSON"},
				{Name: "GetConfig", Kind: "GET", Target: "/api/v1/config/get?section=&key=", Input: "query: section, key", Output: "{ success, value, version, timestamp }", Description: "Read a single configuration key"},
				{Name: "SetConfig", Kind: "POST", Target: "/api/v1/config/set", Input: "{ section, key, value, expected_version? }", Output: "{ success, message, version, timestamp }", Description: "Atomically update key & broadcast via TCP mailboxes"},
				{Name: "DeleteConfig", Kind: "POST", Target: "/api/v1/config/delete", Input: "{ section, key }", Output: "{ success, message, version, timestamp }", Description: "Atomically delete key & broadcast FULL_REFRESH via TCP mailboxes"},
				{Name: "ReloadConfig", Kind: "POST", Target: "/api/v1/config/reload", Output: "{ success, message, version, timestamp }", Description: "Reload configuration baseline + persistence state from disk"},
				{Name: "PersistConfig", Kind: "POST", Target: "/api/v1/config/persist", Output: "{ success, message, timestamp }", Description: "Synchronously flush in-memory store atomically to disk"},
				{Name: "Contracts", Kind: "GET", Target: "/api/v1/contracts", Output: "ContractsEnvelope JSON", Description: "Self-describing microservice exchange contracts"},
				{Name: "MFEScript", Kind: "GET", Target: "/static/mfe.js", Output: "application/javascript", Description: "Embedded OpenMFE <config-server-mfe> custom element"},
			},
		},
		{
			ID:        "grpc",
			Type:      "gRPC / Protobuf",
			TabLabel:  "gRPC Control",
			Icon:      "fa-code",
			Direction: "inbound",
			Transport: "HTTP/2 + Protobuf",
			Address:   toolbox_spec.DisplayAddress(grpcAddr),
			Port:      toolbox_spec.ResolveContractPort(grpcAddr),
			Framing:   "Length-prefixed HTTP/2 DATA frames (ConfigControlService)",
			Auth:      "Internal gRPC insecure/TLS transport",
			Summary:   "Exposes ConfigControlService RPCs alongside standard grpc.health.v1 Health checks.",
			Source:    "config-server/src/grpc_control/config_server.proto (go:embed)",
			Peers: []toolbox_spec.ContractPeer{
				{Service: "web_interface", Role: "consumer", Description: "Optional gRPC fleet management control client"},
			},
			Operations:   grpcOps,
			SchemaFormat: "proto3",
			Schema:       grpc_control.ConfigServerProtoSchema,
		},
		{
			ID:        "safesocket-protobuf",
			Type:      "SafeSocket TCP + ConfigMsg",
			TabLabel:  "SafeSocket TCP",
			Icon:      "fa-plug",
			Direction: "inbound",
			Transport: "TCP + SafeSocket Framing + Protobuf",
			Address:   toolbox_spec.DisplayAddress(tcpAddr),
			Port:      toolbox_spec.ResolveContractPort(tcpAddr),
			Framing:   "4-byte BE length header + HelloMsg Cap'n Proto handshake + binary ConfigMsg Protobuf",
			Auth:      "SafeSocket tcp-hello handshake identity",
			Summary:   "High-speed persistent Copy-On-Write configuration distribution bus using per-client non-blocking mailboxes.",
			Source:    "config-server/src/server/connection.go + distributed-config/src/schemas/config.proto",
			Peers:     tcpPeers,
			Operations: []toolbox_spec.ContractOperation{
				{Name: "HelloHandshake", Kind: "FRAME", Target: "SafeSocket HelloMsg", Input: "fromName, fromHost, fromAddress, toAddress, fromPublicIP", Description: "Initial unpacked Cap'n Proto client identification"},
				{Name: "GET_SYNC", Kind: "RPC", Target: "ConfigMsg.Command=0", Input: "ConfigMsg { Command: GET_SYNC }", Output: "ConfigMsg { Payload: ConfigMap JSON }", Description: "Fetch current snapshot of full ConfigMap"},
				{Name: "FULL_REFRESH", Kind: "RPC", Target: "ConfigMsg.Command=6", Input: "ConfigMsg { Command: FULL_REFRESH }", Output: "ConfigMsg { Payload: ConfigMap JSON }", Description: "Request complete state resynchronization"},
				{Name: "PUT_SYNC", Kind: "RPC", Target: "ConfigMsg.Command=1", Input: "ConfigMsg { Payload: nested updates JSON }", Output: "ACK (3) / ERROR (4)", Description: "Apply atomic update & trigger disk save + broadcast"},
				{Name: "BROADCAST_SYNC", Kind: "SERVER_STREAM", Target: "ConfigMsg.Command=2", Output: "ConfigMsg { Payload: diff JSON }", Description: "Server push to all registered client mailboxes on update"},
			},
			SchemaFormat: "proto3",
			Schema:       safeSocketSchema,
		},
		{
			ID:        "teleremote",
			Type:      "Tele-Remote gRPC Client",
			TabLabel:  "Tele-Remote Stream",
			Icon:      "fa-paper-plane",
			Direction: "outbound",
			Transport: "gRPC Bidirectional Stream",
			Address:   toolbox_spec.DisplayAddress(teleAddr),
			Port:      toolbox_spec.ResolveContractPort(teleAddr),
			Framing:   "teleremote.TeleRemoteService/Connect HTTP/2 stream",
			Auth:      "Bot gateway component registration",
			Summary:   "Registers dynamic Storage Ops and hierarchical Browse/Edit/Delete config menus with Tele-Remote Telegram Bot gateway.",
			Source:    "config-server/src/telegram/manager.go",
			Peers: []toolbox_spec.ContractPeer{
				{Service: "tele_remote", Role: "server", Description: "Receives interactive menu JSON & dispatches BotCommand callbacks"},
			},
			Operations: []toolbox_spec.ContractOperation{
				{Name: "Storage Ops", Kind: "BIDI_STREAM", Target: "Force Save / Reload from Disk", Output: "PersistConfig / ReloadConfig", Description: "Remote disk persistence triggers via Telegram inline buttons"},
				{Name: "Browse Config", Kind: "BIDI_STREAM", Target: "Edit / Delete section keys", Input: "Telegram InputPrompt value", Output: "SetConfig / DeleteConfig + PushMenuUpdate", Description: "Interactive live config inspection & mutation from Telegram"},
			},
			SchemaFormat: "wire",
			Schema:       "Service: teleremote.TeleRemoteService/Connect (stream ComponentMessage) returns (stream BotCommand)\nAdvertised componentName: \"Config Server\"\nTriggers PushMenuUpdate() via srv.SetOnUpdate callback.",
		},
	}

	return contracts
}

// -----------------------------------------------------------------------------

func (h *RESTHandler) handleContracts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var clients []string
	var version string
	if statusInfo, err := h.control.GetStatus(r.Context()); err == nil {
		clients = statusInfo.ClientNames
		version = statusInfo.Version
	}
	if version == "" {
		if h.appConfig != nil && h.appConfig.Config != nil {
			version = h.appConfig.GetVersion()
		}
		if version == "" {
			version = toolbox_version.Get()
		}
	}

	contracts := BuildExchangeContracts(h.appConfig, clients)
	h.sendJSON(w, toolbox_spec.NewContractsEnvelope("config_server", version, "config-server (live controller + embedded contracts)", contracts))
}
