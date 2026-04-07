package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Bastien-Antigravity/config-server/src/server"
	"github.com/Bastien-Antigravity/config-server/src/store"

	"github.com/Bastien-Antigravity/flexible-logger/src/profiles"

	distributed_config "github.com/Bastien-Antigravity/distributed-config"
)

func main() {
	port := flag.String("port", "1026", "Server port") // Default to 1026 per config
	configPath := flag.String("config", "config_store.json", "Path to persistent config file")
	flag.Parse()

	// 0. Load Distributed Configuration (Standalone)
	dConf := distributed_config.New("standalone")
	if dConf == nil {
		fmt.Println("Critical Error: Failed to load distributed configuration")
		os.Exit(1)
	}

	// 1. Create Logger (NoLock Profile)
	logger := profiles.NewDevelLogger("TestServer") // Use DevelLogger for tests
	defer logger.Close()

	logger.Info(fmt.Sprintf("Starting Config Server on port %s...", *port))

	// 2. Initialize Persistence and Store
	pm := store.NewPersistenceManager(*configPath)

	initialConfig := dConf.Config.MemConfig
	if initialConfig == nil {
		initialConfig = make(store.ConfigMap)
	}

	logger.Info("Configuration loaded from distributed-config (standalone)")

	configStore := store.NewStore()
	configStore.Replace(initialConfig)

	// 2. Initialize Protocol Server
	srv := server.NewServer(dConf, logger, configStore, pm)

	// 3. Start Server in Goroutine
	go func() {
		if err := srv.Start(); err != nil {
			logger.Critical(fmt.Sprintf("Server failed: %v", err))
		}
	}()

	// 4. Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("Shutting down...")
	// Save state on exit
	if err := pm.Save(configStore.Get()); err != nil {
		logger.Error(fmt.Sprintf("Error saving config on shutdown: %v", err))
	} else {
		logger.Info("Config saved.")
	}
}
