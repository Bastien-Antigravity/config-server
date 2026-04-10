package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Bastien-Antigravity/config-server/src/server"
	"github.com/Bastien-Antigravity/config-server/src/store"

	"github.com/Bastien-Antigravity/flexible-logger/src/models"
	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	"github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	"github.com/Bastien-Antigravity/universal-logger/src/bootstrap"
)

func main() {
	// 1. Initialize Toolbox Config (handles --port, --host, --name, and --conf automatically)
	appConfig, err := utilconf.LoadConfig("standalone", nil)
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Retrieve the resolved port (or from --port CLI)
	bindAddr, err := appConfig.GetListenAddr("config_server")
	if err != nil {
		bindAddr = ":1026" // default
	}

	// 2. Initialize Logger (bootstrap)
	_, appLogger := bootstrap.Init("config-server", "standalone", "no_lock", models.ParseLevel("INFO"), false)
	defer appLogger.Close()

	appLogger.Info(fmt.Sprintf("Starting Config Server on %s...", bindAddr))

	// 3. Initialize Persistence and Store
	persistenceFile := "config_store.json"
	// Optional: use a specific flag for this if needed in the future
	pm := store.NewPersistenceManager(persistenceFile)

	initialConfig := appConfig.Config.MemConfig
	if initialConfig == nil {
		initialConfig = make(store.ConfigMap)
	}

	appLogger.Info("Configuration loaded via Toolbox (network-aware)")

	configStore := store.NewStore()
	configStore.Replace(initialConfig)

	// 3. Initialize Protocol Server (Now using Toolbox Config)
	srv := server.NewServer(appConfig, appLogger, configStore, pm)

	// 4. Start Server in Goroutine
	go func() {
		if err := srv.Start(); err != nil {
			appLogger.Critical(fmt.Sprintf("Server failed: %v", err))
		}
	}()

	// 5. Graceful Shutdown via Toolbox
	lm := lifecycle.NewManager()
	lm.Register("ConfigPersistence", func() error {
		appLogger.Info("Saving config state on shutdown...")
		return pm.Save(configStore.Get())
	})

	lm.Wait(context.Background())
}
