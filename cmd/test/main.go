package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Bastien-Antigravity/config-server/src/server"
	"github.com/Bastien-Antigravity/config-server/src/store"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	unilog_bootstrap "github.com/Bastien-Antigravity/universal-logger/src/bootstrap"
	unilog_config "github.com/Bastien-Antigravity/universal-logger/src/config"
)

func main() {
	// 0. Initialize Toolbox Config
	appConfig, err := toolbox_config.LoadConfig("standalone", nil)
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 1. Setup Logger via Universal Logger (InitWithOptions)
	_, appLogger := unilog_bootstrap.InitWithOptions(unilog_bootstrap.BootstrapOptions{
		Name:           "ConfigTestServer",
		ConfigProfile:  "standalone",
		LoggerProfile:  "devel",
		ExistingConfig: &unilog_config.DistConfig{Config: appConfig.Config},
	})
	defer appLogger.Close()

	addr, _ := appConfig.GetListenAddr("config_server")
	appLogger.Info("Starting Config Test Server on %s...", addr)

	// 2. Initialize Persistence and Store
	pm := store.NewPersistenceManager("config_store.json", appLogger)

	initialConfig, err := pm.Load()
	if err != nil {
		appLogger.Warning("Failed to load config persistence: %v", err)
		initialConfig = make(store.ConfigMap)
	}

	configStore := store.NewStore()
	configStore.Replace(initialConfig)

	srv := server.NewServer(appConfig, appLogger, configStore, pm)

	// 3. Lifecycle Management
	lm := toolbox_lifecycle.NewManagerWithLogger(appLogger)

	// Register cleanup: Save state on exit
	lm.Register("ConfigPersistence", func() error {
		appLogger.Info("Saving config state on shutdown...")
		return pm.Save(configStore.Get())
	})

	// 4. Start Server in Goroutine
	go func() {
		if err := srv.Start(); err != nil {
			appLogger.Critical("Server failed: %v", err)
		}
	}()

	// 5. Wait for termination signals
	lm.Wait(context.Background())
}
