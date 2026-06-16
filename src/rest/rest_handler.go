package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/core"
	unilog_interfaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------
// RESTHandler handles HTTP management requests
// -----------------------------------------------------------------------------

type RESTHandler struct {
	logger  unilog_interfaces.Logger
	control core.ConfigController
}

// -----------------------------------------------------------------------------

// NewRESTHandler creates a new RESTHandler instance
func NewRESTHandler(control core.ConfigController, logger unilog_interfaces.Logger) *RESTHandler {
	return &RESTHandler{
		logger:  logger,
		control: control,
	}
}

// -----------------------------------------------------------------------------

// RegisterRoutes registers the REST routes to the provided mux
func (h *RESTHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/config/get", h.handleGetConfig)
	mux.HandleFunc("/api/v1/config/set", h.handleSetConfig)
	mux.HandleFunc("/api/v1/config/list", h.handleListConfig)
	mux.HandleFunc("/api/v1/config/reload", h.handleReloadConfig)
	mux.HandleFunc("/api/v1/config/persist", h.handlePersistConfig)
	mux.HandleFunc("/api/v1/status", h.handleStatus)
}

// -----------------------------------------------------------------------------

type httpGetResponse struct {
	Success   bool   `json:"success"`
	Value     string `json:"value"`
	Timestamp int64  `json:"timestamp"`
}

func (h *RESTHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	section := r.URL.Query().Get("section")
	key := r.URL.Query().Get("key")

	if section == "" || key == "" {
		http.Error(w, "Missing section or key parameter", http.StatusBadRequest)
		return
	}

	val, ok, err := h.control.GetConfig(r.Context(), section, key)
	if err != nil {
		h.sendJSON(w, httpGetResponse{
			Success:   false,
			Value:     "",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.sendJSON(w, httpGetResponse{
		Success:   ok,
		Value:     val,
		Timestamp: time.Now().Unix(),
	})
}

type httpSetConfigRequest struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

type httpControlResponse struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	Timestamp int64  `json:"timestamp"`
	ErrorCode string `json:"error_code,omitempty"`
}

func (h *RESTHandler) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req httpSetConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.control.SetConfig(r.Context(), req.Section, req.Key, req.Value)
	if err != nil {
		h.sendJSON(w, httpControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to update config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "UPDATE_FAILED",
		})
		return
	}

	h.sendJSON(w, httpControlResponse{
		Success:   true,
		Message:   "Configuration updated and broadcasted successfully",
		Timestamp: time.Now().Unix(),
	})
}

type httpListResponse struct {
	Success    bool   `json:"success"`
	JsonConfig string `json:"json_config"`
	Timestamp  int64  `json:"timestamp"`
}

func (h *RESTHandler) handleListConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.control.ListConfig(r.Context())
	if err != nil {
		h.sendJSON(w, httpListResponse{
			Success:   false,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	data, err := json.Marshal(config)
	if err != nil {
		h.sendJSON(w, httpListResponse{
			Success:   false,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.sendJSON(w, httpListResponse{
		Success:    true,
		JsonConfig: string(data),
		Timestamp:  time.Now().Unix(),
	})
}

func (h *RESTHandler) handleReloadConfig(w http.ResponseWriter, r *http.Request) {
	err := h.control.ReloadConfig(r.Context())
	if err != nil {
		h.sendJSON(w, httpControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to reload config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "RELOAD_FAILED",
		})
		return
	}

	h.sendJSON(w, httpControlResponse{
		Success:   true,
		Message:   "Configuration reloaded from disk",
		Timestamp: time.Now().Unix(),
	})
}

func (h *RESTHandler) handlePersistConfig(w http.ResponseWriter, r *http.Request) {
	err := h.control.PersistConfig(r.Context())
	if err != nil {
		h.sendJSON(w, httpControlResponse{
			Success:   false,
			Message:   fmt.Sprintf("Failed to persist config: %v", err),
			Timestamp: time.Now().Unix(),
			ErrorCode: "PERSIST_FAILED",
		})
		return
	}

	h.sendJSON(w, httpControlResponse{
		Success:   true,
		Message:   "Persistence trigger activated",
		Timestamp: time.Now().Unix(),
	})
}

type httpStatusResponse struct {
	Healthy       bool     `json:"healthy"`
	Status        string   `json:"status"`
	Version       string   `json:"version"`
	Timestamp     int64    `json:"timestamp"`
	ActiveClients int32    `json:"active_clients"`
	ClientNames   []string `json:"client_names"`
}

func (h *RESTHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	statusInfo, err := h.control.GetStatus(r.Context())
	if err != nil {
		h.sendJSON(w, httpStatusResponse{
			Healthy:   false,
			Status:    "Error",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.sendJSON(w, httpStatusResponse{
		Healthy:       statusInfo.Healthy,
		Status:        statusInfo.Status,
		Version:       statusInfo.Version,
		Timestamp:     statusInfo.Timestamp,
		ActiveClients: int32(statusInfo.ActiveClients),
		ClientNames:   statusInfo.ClientNames,
	})
}

// -----------------------------------------------------------------------------

func (h *RESTHandler) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// -----------------------------------------------------------------------------

// StartServer starts a simple HTTP server for the REST API
func (h *RESTHandler) StartServer(port int) error {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%d", port)
	h.logger.Info("Starting REST management server on %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return server.ListenAndServe()
}
