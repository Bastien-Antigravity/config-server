package telegram

// =============================================================================
// ESSENTIAL PROCESS:
// Binds config-server management operations to the tele-remote interactive Telegram
// bot interface, publishing hierarchical menus for browsing, editing, and persisting state.
//
// DATA FLOW:
// 1. Input: Tele-Remote gRPC connection and dynamic configuration updates from Server.
// 2. Logic: Constructs Telegram UI actions for storage operations and live configuration tweaking,
//    registering callback handlers with TeleClient.
// 3. Output: Interactive menu frames and status updates sent to tele-remote.
//
// KEY PARAMETERS:
// - MenuManager: Interactive menu builder and callback coordinator.
// - TeleClient: gRPC streaming client connecting to tele-remote daemon.
// =============================================================================

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"

	"github.com/Bastien-Antigravity/config-server/src/core"
	"github.com/Bastien-Antigravity/config-server/src/helpers"

	toolbox_config "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/config"
	toolbox_lifecycle "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/lifecycle"
	toolbox_teleclient "github.com/Bastien-Antigravity/microservice-toolbox/go/pkg/teleremote"
	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------

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

// BuildActions constructs the deterministic Telegram action hierarchy from the current configuration map.
func (m *MenuManager) BuildActions() ([]toolbox_teleclient.Action, error) {
	configMap, err := m.controller.ListConfig(context.Background())
	if err != nil {
		return nil, err
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

	// 2. Dynamic Config Browser (sorted deterministically for stable inline menu trees)
	sectionNames := make([]string, 0, len(configMap))
	for section := range configMap {
		sectionNames = append(sectionNames, section)
	}
	sort.Strings(sectionNames)

	var sections []toolbox_teleclient.Action
	for _, sName := range sectionNames {
		keys := configMap[sName]
		keyNames := make([]string, 0, len(keys))
		for key := range keys {
			keyNames = append(keyNames, key)
		}
		sort.Strings(keyNames)

		var keyActions []toolbox_teleclient.Action
		for _, kName := range keyNames {
			vVal := helpers.RedactValue(kName, keys[kName])
			secCopy := sName
			keyCopy := kName

			keyActions = append(keyActions, toolbox_teleclient.Action{
				Label: fmt.Sprintf("🔑 %s", keyCopy),
				SubMenu: []toolbox_teleclient.Action{
					{
						Label:    fmt.Sprintf("Value: %s", vVal),
						Callback: func(string) error { return nil },
					},
					{
						Label:       fmt.Sprintf("✏️ Edit %s", keyCopy),
						InputPrompt: fmt.Sprintf("Enter new value for [%s] %s:", secCopy, keyCopy),
						Callback: func(input string) error {
							if input == "" {
								return nil
							}
							m.logger.Info("Telegram update: [%s] %s = %s", secCopy, keyCopy, helpers.RedactValue(keyCopy, input))
							return m.controller.SetConfig(context.Background(), secCopy, keyCopy, input)
						},
					},
					{
						Label: fmt.Sprintf("❌ Delete %s", keyCopy),
						Callback: func(string) error {
							return m.controller.DeleteConfig(context.Background(), secCopy, keyCopy)
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
	return actions, nil
}

// RebuildMenu dynamically pulls the configuration map and registers it with the TeleClient.
func (m *MenuManager) RebuildMenu() {
	actions, err := m.BuildActions()
	if err != nil {
		m.logger.Error("Telegram Menu: failed to list config: %v", err)
		return
	}

	m.tc.UpdateActions(actions)
	m.tc.PushMenuUpdate()
}

// SetupTelegram initializes the Tele-Remote client, binds dynamic updates, and registers with Lifecycle Manager.
func SetupTelegram(appConfig *toolbox_config.AppConfig, controller core.ConfigController, logger unilog_ifaces.Logger, onUpdateRegistry func(func()), lm *toolbox_lifecycle.Manager) {
	addr, err := appConfig.GetGRPCListenAddr("tele_remote")
	if err != nil {
		addr, err = appConfig.GetListenAddr("tele_remote")
	}
	if err != nil {
		logger.Warning("Tele-Remote capability address could not be resolved: %v", err)
		return
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		logger.Warning("Invalid tele_remote address format resolved: %s (%v)", addr, err)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		logger.Warning("Invalid tele_remote port resolved: %s (%v)", portStr, err)
		return
	}

	teleClient := toolbox_teleclient.NewTeleClient("Config Server", host, port, logger)
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
