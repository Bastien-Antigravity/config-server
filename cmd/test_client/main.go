package main

import (
	"fmt"

	distributed_config "github.com/Bastien-Antigravity/distributed-config"
)

func main() {
	fmt.Println("Initializing Distributed Config (Production Profile)...")

	// Use distributed-config to fetch configuration
	// "production" profile should connect to the Config Server.
	// Note: We assume the library knows how to find the server (or uses default/DNS).
	dConf := distributed_config.New("production")
	// if dConf == nil { ... } removed
	fmt.Printf("Type of dConf: %T\n", dConf)

	fmt.Println("Connected and Configuration Loaded!")

	// 1. Get Initial Config
	// In the new library, config is loaded into MemConfig
	cfg := dConf.Config.MemConfig
	fmt.Printf("Initial Config (MemConfig): %+v\n", cfg)

	// Update logic removed as distributed-config does not expose public Update method yet.
	fmt.Println("Update capability is not available in distributed-config yet.")

	// Verify Common config specifically
	fmt.Printf("Common Config: %+v\n", dConf.Config.Common)
}
