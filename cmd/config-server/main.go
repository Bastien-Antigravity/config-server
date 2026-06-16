package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Bastien-Antigravity/config-server/src/grpc_control"
	"github.com/Bastien-Antigravity/config-server/src/rest"
	"github.com/Bastien-Antigravity/config-server/src/server"
	"github.com/Bastien-Antigravity/config-server/src/store"
	"github.com/Bastien-Antigravity/config-server/src/telegram"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_utils "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/utils"
	unilog "github.com/Bastien-Antigravity/universal-logger/src/bootstrap"
	unilog_config "github.com/Bastien-Antigravity/universal-logger/src/config"
)

func main() {
	// 1. Initialize Toolbox Config (which handles name/IP resolution)
	appConfig, err := toolbox_config.LoadConfig("standalone", []string{"store"})
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Logger (bootstrap)
	_, appLogger := unilog.Init("config-server", "standalone", "no_lock", "INFO", false, &unilog_config.DistConfig{Config: appConfig.Config})
	defer appLogger.Close()

	appConfig.Logger = appLogger

	addr, _ := appConfig.GetListenAddr("config_server")
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

	restHandler := rest.NewRESTHandler(srv, appLogger)
	go func() {
		_ = restHandler.StartServer(3308)
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
			grpcSrv.Stop(context.Background())
		}
		return nil
	})

	lm.Register("ConfigPersistence", func() error {
		appLogger.Info("Performing final terminal state save...")
		return pm.Save(configStore.Get())
	})

	lm.Wait(context.Background())
}
