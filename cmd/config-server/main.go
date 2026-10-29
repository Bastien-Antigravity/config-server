package main

// =============================================================================
// ESSENTIAL PROCESS:
// Boots and initializes the config-server microservice, establishing
// dynamic capabilities discovery and REST management ports.
//
// DATA FLOW:
// 1. Input: Loads configuration using the layered microservice-toolbox loader.
// 2. Logic: Starts core synchronization loops, launches REST interface, and
//    auto-registers config-server OpenMFE with web-interface.
// 3. Output: Runs the server listening for dynamic configurations.
//
// KEY PARAMETERS:
// - config_server: Capabilities section representing the server's ports.
// - web_interface: Sibling dashboard details used for dynamic MFE registration.
// =============================================================================

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Bastien-Antigravity/config-server/src/grpc_control"
	"github.com/Bastien-Antigravity/config-server/src/rest"
	"github.com/Bastien-Antigravity/config-server/src/server"
	"github.com/Bastien-Antigravity/config-server/src/store"
	"github.com/Bastien-Antigravity/config-server/src/telegram"

	toolbox_bootstrap "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/bootstrap"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_utils "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/utils"
)

// -----------------------------------------------------------------------------

func main() {
	// 1. Initialize Service via Unified Ecosystem Bootstrapper
	appConfig, appLogger := toolbox_bootstrap.BootstrapService("config-server", "store")
	if appConfig == nil {
		panic("config-server: bootstrap failed to produce non-nil appConfig")
	}
	if appLogger == nil {
		panic("config-server: bootstrap failed to produce non-nil appLogger")
	}
	defer appLogger.Close()

	addr, err := appConfig.GetListenAddr("config_server")
	if err != nil {
		appLogger.Critical("Failed to resolve listen address for config_server: %v", err)
		os.Exit(1)
	}
	appLogger.Info("Starting Config Server on %s...", addr)

	// 3. Initialize Persistence and Store
	baseDir := toolbox_utils.GetBaseDir()
	storePath := "config_store.json"
	if baseDir != "" {
		storePath = filepath.Join(baseDir, "config_store.json")
	}

	if customPath := appConfig.Args.Extra["store"]; customPath != "" {
		storePath = customPath
		appLogger.Info("Using custom persistence store: %s", storePath)
	}
	pm := store.NewPersistenceManager(storePath, appLogger)

	initialConfig, err := pm.Load()
	if err != nil {
		appLogger.Warning("Failed to load config persistence: %v", err)
		initialConfig = make(store.ConfigMap)
	}

	configStore := store.NewStore()
	configStore.Replace(initialConfig)

	// 4. Initialize Protocol Server
	srv := server.NewServer(appConfig, appLogger, configStore, pm)

	// 5. Initialize Management Interfaces (gRPC & REST)
	grpcSrv, err := grpc_control.NewGRPCService(srv, appLogger)
	if err != nil {
		appLogger.Error("Failed to initialize gRPC management service: %v", err)
	} else {
		if err := grpcSrv.Start(); err != nil {
			appLogger.Error("Failed to start gRPC management service: %v", err)
		}
	}

	// Resolve REST address and Web Interface address dynamically from capabilities config
	restAddr, err := appConfig.GetRESTAddr("config_server")
	if err != nil {
		appLogger.Critical("Failed to resolve REST address for config_server: %v", err)
		os.Exit(1)
	}

	restHandler := rest.NewRESTHandler(srv, appLogger)
	go func() {
		if err := restHandler.StartServer(restAddr); err != nil {
			appLogger.Error("REST management server failed: %v", err)
		}
	}()

	// Register config-server OpenMFE with web-interface dynamically (non-blocking, infinite retry)
	go func() {
		mfeUrl := fmt.Sprintf("http://%s/static/js/mfe-loader.js", restAddr)
		payload := fmt.Sprintf(`{
			"name": "config-server",
			"tag": "config-server-mfe",
			"url": "%s",
			"navTitle": "⚙️ Config Server",
			"layer": "L1",
			"category": "Base Fleet",
			"priority": 20,
			"icon": "fa-cogs",
			"requiresAuth": false
		}`, mfeUrl)

		client := &http.Client{Timeout: 3 * time.Second}
		backoff := 2 * time.Second
		maxBackoff := 15 * time.Second

		for {
			webAddr, err := appConfig.GetListenAddr("web_interface")
			if err == nil && webAddr != "" {
				regUrl := fmt.Sprintf("http://%s/api/v1/register", webAddr)
				resp, postErr := client.Post(regUrl, "application/json", strings.NewReader(payload))
				if postErr == nil {
					resp.Body.Close()
					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						appLogger.Info("Successfully registered config-server OpenMFE with web-interface at %s", regUrl)
						return
					}
					appLogger.Warning("OpenMFE registration attempt failed with status %s; retrying in %v...", resp.Status, backoff)
				} else {
					appLogger.Warning("OpenMFE registration attempt failed (%v); retrying in %v...", postErr, backoff)
				}
			} else {
				appLogger.Warning("web_interface capability address unresolvable (%v); retrying in %v...", err, backoff)
			}

			time.Sleep(backoff)
			backoff = time.Duration(float64(backoff) * 1.5)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}()

	// 6. Start Main Protocol Server
	go func() {
		if err := srv.Start(); err != nil {
			appLogger.Critical("Server failed: %v", err)
		}
	}()

	// 7. Initialize Lifecycle Manager
	lm := toolbox_lifecycle.NewManagerWithLogger(appLogger)

	// 8. Initialize Tele-Remote Client (via Telegram component)
	telegram.SetupTelegram(appConfig, srv, appLogger, func(cb func()) {
		srv.OnUpdate = cb
	}, lm)

	// 9. Graceful Shutdown
	lm.Register("StopServer", func() error {
		srv.Stop()
		if grpcSrv != nil {
			_ = grpcSrv.Stop(context.Background())
		}
		if restHandler != nil {
			_ = restHandler.Stop(context.Background())
		}
		return nil
	})

	lm.Register("ConfigPersistence", func() error {
		appLogger.Info("Performing final terminal state save...")
		return pm.Save(configStore.Get())
	})

	lm.Wait(context.Background())
}
