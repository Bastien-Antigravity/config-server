package main

import (
	"context"
	"flag"
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
	port := flag.String("port", "1026", "Server port") // Default to 1026 per config
	configPath := flag.String("config", "config_store.json", "Path to persistent config file")
	flag.Parse()

	// 1. Initialize Toolbox Config (which handles name/IP resolution)
	appConfig, err := utilconf.LoadConfig("standalone")
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize Logger (bootstrap)
	_, appLogger := bootstrap.Init("config-server", "standalone", "no_lock", models.ParseLevel("INFO"), false)
	defer appLogger.Close()

	appLogger.Info(fmt.Sprintf("Starting Config Server on port %s...", *port))

	// 3. Initialize Persistence and Store
	pm := store.NewPersistenceManager(*configPath)

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
