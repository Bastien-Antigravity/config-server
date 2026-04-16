package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Bastien-Antigravity/config-server/src/server"
	"github.com/Bastien-Antigravity/config-server/src/store"

	"github.com/Bastien-Antigravity/flexible-logger/src/profiles"

	utilconf "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	"github.com/Bastien-Antigravity/universal-logger/src/logger"
)

func main() {
	// 0. Initialize Toolbox Config
	appConfig, err := utilconf.LoadConfig("standalone", nil)
	if err != nil {
		fmt.Printf("Critical Error loading config: %v\n", err)
		os.Exit(1)
	}

	dConf := appConfig.Config
	if dConf == nil {
		fmt.Println("Critical Error: Failed to load distributed configuration")
		os.Exit(1)
	}

	// 1. Create Logger (NoLock Profile)
	flexLogger := profiles.NewDevelLogger("TestServer") // Use DevelLogger for tests
	logger := logger.NewUniLog(flexLogger)
	defer logger.Close()

	addr, _ := appConfig.GetListenAddr("config_server")
	logger.Info(fmt.Sprintf("Starting Config Server on %s...", addr))

	// 2. Initialize Persistence and Store
	pm := store.NewPersistenceManager("config_store.json")

	initialConfig := dConf.Config.MemConfig
	if initialConfig == nil {
		initialConfig = make(store.ConfigMap)
	}

	logger.Info("Configuration loaded from distributed-config (standalone)")

	configStore := store.NewStore()
	configStore.Replace(initialConfig)

	srv := server.NewServer(appConfig, logger, configStore, pm)

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
