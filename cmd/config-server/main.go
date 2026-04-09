package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Bastien-Antigravity/config-server/src/server"
	"github.com/Bastien-Antigravity/config-server/src/store"

	"github.com/Bastien-Antigravity/universal-logger/src/bootstrap"
	"github.com/Bastien-Antigravity/flexible-logger/src/models"
)

func main() {
	port := flag.String("port", "1026", "Server port") // Default to 1026 per config
	configPath := flag.String("config", "config_store.json", "Path to persistent config file")
	flag.Parse()

	// 1. Initialize Distributed Configuration and Logger (bootstrap)
	distConfig, appLogger := bootstrap.Init("ConfigServer", "standalone", "no_lock", models.ParseLevel("INFO"), false)
	defer appLogger.Close()

	appLogger.Info(fmt.Sprintf("Starting Config Server on port %s...", *port))

	// 3. Initialize Persistence and Store
	pm := store.NewPersistenceManager(*configPath)

	initialConfig := distConfig.MemConfig
	if initialConfig == nil {
		initialConfig = make(store.ConfigMap)
	}

	appLogger.Info("Configuration loaded from distributed-config (standalone)")

	configStore := store.NewStore()
	configStore.Replace(initialConfig)

	// 3. Initialize Protocol Server
	srv := server.NewServer(distConfig, appLogger, configStore, pm)

	// 4. Start Server in Goroutine
	go func() {
		if err := srv.Start(); err != nil {
			appLogger.Critical(fmt.Sprintf("Server failed: %v", err))
		}
	}()

	// 5. Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	appLogger.Info("Shutting down...")
	// Save state on exit
	if err := pm.Save(configStore.Get()); err != nil {
		appLogger.Error(fmt.Sprintf("Error saving config on shutdown: %v", err))
	} else {
		appLogger.Info("Config saved.")
	}
}
