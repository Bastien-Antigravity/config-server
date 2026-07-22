package telegram

import (
	"context"
	"fmt"

	"github.com/Bastien-Antigravity/config-server/src/core"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_teleclient "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/teleremote"
	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// MenuManager orchestrates the rebuild operations of the Telegram interactive menus.
type MenuManager struct {
	tc         *toolbox_teleclient.TeleClient
	controller core.ConfigController
	logger     unilog_ifaces.Logger
}

// NewMenuManager creates a new MenuManager.
func NewMenuManager(tc *toolbox_teleclient.TeleClient, controller core.ConfigController, logger unilog_ifaces.Logger) *MenuManager {
	return &MenuManager{
		tc:         tc,
		controller: controller,
		logger:     logger,
	}
}

// RebuildMenu dynamically pulls the configuration map and registers it with the TeleClient.
func (m *MenuManager) RebuildMenu() {
	configMap, err := m.controller.ListConfig(context.Background())
	if err != nil {
		m.logger.Error("Telegram Menu: failed to list config: %v", err)
		return
	}

	var actions []toolbox_teleclient.Action

	// 1. Storage Operations
	actions = append(actions, toolbox_teleclient.Action{
		Label: "📂 Storage Ops",
		SubMenu: []toolbox_teleclient.Action{
			{
				Label: "💾 Force Save to Disk",
				Callback: func(input string) error {
					return m.controller.PersistConfig(context.Background())
				},
			},
			{
				Label: "🔄 Reload from Disk",
				Callback: func(input string) error {
					return m.controller.ReloadConfig(context.Background())
				},
			},
		},
	})

	// 2. Dynamic Config Browser
	var sections []toolbox_teleclient.Action
	for section, keys := range configMap {
		sName := section
		var keyActions []toolbox_teleclient.Action

		for key, val := range keys {
			kName := key
			vVal := val

			keyActions = append(keyActions, toolbox_teleclient.Action{
				Label: fmt.Sprintf("🔑 %s", kName),
				SubMenu: []toolbox_teleclient.Action{
					{
						Label:    fmt.Sprintf("Value: %s", vVal),
						Callback: func(string) error { return nil },
					},
					{
						Label:       fmt.Sprintf("✏️ Edit %s", kName),
						InputPrompt: fmt.Sprintf("Enter new value for [%s] %s:", sName, kName),
						Callback: func(input string) error {
							if input == "" {
								return nil
							}
							m.logger.Info("Telegram update: [%s] %s = %s", sName, kName, input)
							return m.controller.SetConfig(context.Background(), sName, kName, input)
						},
					},
					{
						Label: fmt.Sprintf("❌ Delete %s", kName),
						Callback: func(string) error {
							return m.controller.DeleteConfig(context.Background(), sName, kName)
						},
					},
				},
			})
		}

		sections = append(sections, toolbox_teleclient.Action{
			Label:   fmt.Sprintf("📁 %s", sName),
			SubMenu: keyActions,
		})
	}

	actions = append(actions, toolbox_teleclient.Action{
		Label:   "🔍 Browse Config",
		SubMenu: sections,
	})

	m.tc.UpdateActions(actions)
	m.tc.PushMenuUpdate()
}

// SetupTelegram initializes the Tele-Remote client, binds dynamic updates, and registers with Lifecycle Manager.
func SetupTelegram(appConfig *toolbox_config.AppConfig, controller core.ConfigController, logger unilog_ifaces.Logger, onUpdateRegistry func(func()), lm *toolbox_lifecycle.Manager) {
	var teleCap struct {
		IP   string `json:"ip"`
		Port string `json:"port"`
	}
	if err := appConfig.GetCapability("tele_remote", &teleCap); err != nil {
		logger.Warning("Tele-Remote capability not found or configured: %v", err)
		return
	}
	port := 50051
	if teleCap.Port != "" {
		fmt.Sscanf(teleCap.Port, "%d", &port)
	}

	ip := "127.0.0.1"
	if teleCap.IP != "" {
		ip = teleCap.IP
	}

	teleClient := toolbox_teleclient.NewTeleClient("Config Server", ip, port, logger)
	mgr := NewMenuManager(teleClient, controller, logger)

	// Initial menu build
	mgr.RebuildMenu()

	// Register updater callback for any config updates
	onUpdateRegistry(func() {
		mgr.RebuildMenu()
	})

	teleClient.Start()

	lm.Register("TeleClient", func() error {
		teleClient.Close()
		return nil
	})
}
