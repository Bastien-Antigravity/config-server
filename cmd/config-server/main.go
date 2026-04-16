package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Bastien-Antigravity/config-server/src/server"
	"github.com/Bastien-Antigravity/config-server/src/store"

	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	"github.com/Bastien-Antigravity/universal-logger/src/bootstrap"
	"github.com/Bastien-Antigravity/universal-logger/src/config"
)

func main() {
	// 1. Initialize Toolbox Config (which handles name/IP resolution)
	// Passing nil for specificFlags lets LoadConfig use the default flag parsing.
	appConfig, err := utilconf.LoadConfig("standalone", nil)
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Logger (bootstrap)
	_, appLogger := bootstrap.Init("config-server", "standalone", "no_lock", "INFO", false, &config.DistConfig{Config: appConfig.Config})
	defer appLogger.Close()

	// Inject logger into Config for toolbox internal logs
	appConfig.Logger = appLogger

	addr, _ := appConfig.GetListenAddr("config_server")
	appLogger.Info("Starting Config Server on %s...", addr)

	// 3. Initialize Persistence and Store
	pm := store.NewPersistenceManager("config_store.json")

	initialConfig := appConfig.Config.MemConfig
	if initialConfig == nil {
		initialConfig = make(store.ConfigMap)
	}

	appLogger.Info("Configuration loaded via Toolbox (network-aware)")

	configStore := store.NewStore()
	configStore.Replace(initialConfig)

	// 4. Initialize Protocol Server (Now using Toolbox Config)
	srv := server.NewServer(appConfig, appLogger, configStore, pm)

	// 5. Start Server in Goroutine
	go func() {
		if err := srv.Start(); err != nil {
			appLogger.Critical("Server failed: %v", err)
		}
	}()

	// 6. Graceful Shutdown via Toolbox
	lm := lifecycle.NewManagerWithLogger(appLogger)
	lm.Register("ConfigPersistence", func() error {
		appLogger.Info("Saving config state on shutdown...")
		return pm.Save(configStore.Get())
	})

	lm.Wait(context.Background())
}
