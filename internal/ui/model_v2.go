package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AppV2 struct {
	stage     string // "init" -> "dashboard"
	dashboard DashboardModel
	initForm  InitFormModel
	width     int
	height    int
}

// InitFormModel - Quick setup to get PK and RPC endpoints
type InitFormModel struct {
	step       int // 0: PK, 1: Cosmos RPC, 2: EVM RPC
	pk         PKModel
	cosmosRPC  SimpleInputModel
	evmRPC     SimpleInputModel
	privateKey string
	cosmosURL  string
	evmURL     string
}

type SimpleInputModel struct {
	value string
	done  bool
}

func NewAppV2() AppV2 {
	return AppV2{
		stage:     "init",
		dashboard: NewDashboard(),
		initForm: InitFormModel{
			pk:        NewPKModel(),
			cosmosRPC: SimpleInputModel{},
			evmRPC:    SimpleInputModel{},
		},
	}
}

func (m AppV2) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m AppV2) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.stage == "dashboard" {
				return m, tea.Quit
			}
		}
	}

	switch m.stage {
	case "init":
		return m.updateInit(msg)
	case "dashboard":
		return m.updateDashboard(msg)
	}

	return m, nil
}

func (m AppV2) updateInit(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.initForm.step {
	case 0: // Private Key
		if kMsg, ok := msg.(tea.KeyMsg); ok {
			switch kMsg.String() {
			case "enter":
				if len(m.initForm.privateKey) >= 64 {
					m.initForm.step = 1
					// Try to use cached RPC if available
					if cachedRPC := GetCachedRPC("cosmos"); cachedRPC != "" {
						m.initForm.cosmosRPC.value = cachedRPC
					}
				}
			case "backspace":
				if len(m.initForm.privateKey) > 0 {
					m.initForm.privateKey = m.initForm.privateKey[:len(m.initForm.privateKey)-1]
				}
			default:
				// Only accept hex characters
				if len(kMsg.String()) == 1 && isHexChar(kMsg.String()[0]) && len(m.initForm.privateKey) < 64 {
					m.initForm.privateKey += kMsg.String()
				}
			}
		}
		return m, nil

	case 1: // Cosmos RPC
		if kMsg, ok := msg.(tea.KeyMsg); ok {
			switch kMsg.String() {
			case "enter":
				if m.initForm.cosmosRPC.value == "" {
					m.initForm.cosmosRPC.value = "https://cosmos-rpc.polkachu.com"
				}
				m.initForm.cosmosURL = m.initForm.cosmosRPC.value
				CacheRPC("cosmos", m.initForm.cosmosURL)
				m.initForm.step = 2

				// Try to use cached EVM RPC
				if cachedRPC := GetCachedRPC("evm"); cachedRPC != "" {
					m.initForm.evmRPC.value = cachedRPC
				}
			default:
				if len(kMsg.String()) == 1 {
					m.initForm.cosmosRPC.value += kMsg.String()
				}
			case "backspace":
				if len(m.initForm.cosmosRPC.value) > 0 {
					m.initForm.cosmosRPC.value = m.initForm.cosmosRPC.value[:len(m.initForm.cosmosRPC.value)-1]
				}
			}
		}
		return m, nil

	case 2: // EVM RPC
		if kMsg, ok := msg.(tea.KeyMsg); ok {
			switch kMsg.String() {
			case "enter":
				if m.initForm.evmRPC.value == "" {
					m.initForm.evmRPC.value = "https://eth.llamarpc.com"
				}
				m.initForm.evmURL = m.initForm.evmRPC.value
				CacheRPC("evm", m.initForm.evmURL)

				// Initialize dashboard
				m.dashboard.Init(m.initForm.privateKey, m.initForm.cosmosURL, m.initForm.evmURL)
				m.stage = "dashboard"
			default:
				if len(kMsg.String()) == 1 {
					m.initForm.evmRPC.value += kMsg.String()
				}
			case "backspace":
				if len(m.initForm.evmRPC.value) > 0 {
					m.initForm.evmRPC.value = m.initForm.evmRPC.value[:len(m.initForm.evmRPC.value)-1]
				}
			}
		}
		return m, nil
	}

	return m, nil
}

func (m AppV2) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.dashboard, cmd = m.dashboard.Update(msg)
	return m, cmd
}

func (m AppV2) View() string {
	if m.stage == "init" {
		return m.viewInit()
	}
	return m.dashboard.View()
}

func (m AppV2) viewInit() string {
	switch m.initForm.step {
	case 0:
		return lipgloss.JoinVertical(lipgloss.Left,
			"",
			"🚀 TxPilot Setup",
			"",
			"Enter private key (hex, 64 chars):",
			"┌────────────────────────────────────────────────────────────┐",
			"│ "+m.initForm.privateKey+strings.Repeat("•", 64-len(m.initForm.privateKey))+" │",
			"└────────────────────────────────────────────────────────────┘",
			"",
			"Press Enter to continue",
		)

	case 1:
		display := m.initForm.cosmosRPC.value
		if display == "" {
			display = "https://cosmos-rpc.polkachu.com"
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			"",
			"🌌 Cosmos RPC Setup",
			"",
			"Enter Cosmos RPC endpoint (or press Enter for default):",
			"┌────────────────────────────────────────────────────────────┐",
			"│ "+display+strings.Repeat(" ", max(0, 58-len(display)))+" │",
			"└────────────────────────────────────────────────────────────┘",
			"",
			"Press Enter to continue",
		)

	case 2:
		display := m.initForm.evmRPC.value
		if display == "" {
			display = "https://eth.llamarpc.com"
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			"",
			"⟠ EVM RPC Setup",
			"",
			"Enter EVM RPC endpoint (or press Enter for default):",
			"┌────────────────────────────────────────────────────────────┐",
			"│ "+display+strings.Repeat(" ", max(0, 58-len(display)))+" │",
			"└────────────────────────────────────────────────────────────┘",
			"",
			"Press Enter to continue",
		)
	}
	return ""
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
