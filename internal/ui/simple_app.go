package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type SimpleApp struct {
	step        int // 0: PK, 1: Cosmos RPC, 2: EVM RPC, 3: Dashboard
	pkInput     textinput.Model
	cosmosInput textinput.Model
	evmInput    textinput.Model
	dashboard   HtopDashboard
	width       int
	height      int
}

func NewSimpleApp() SimpleApp {
	// PK input
	pkInput := textinput.New()
	pkInput.Placeholder = "Enter your private key (hex)"
	pkInput.Focus()
	pkInput.Width = 80

	// Cosmos RPC input
	cosmosInput := textinput.New()
	cosmosInput.Placeholder = "http://localhost:1317"
	cosmosInput.Width = 80

	// EVM RPC input
	evmInput := textinput.New()
	evmInput.Placeholder = "http://localhost:8545"
	evmInput.Width = 80

	return SimpleApp{
		step:        0,
		pkInput:     pkInput,
		cosmosInput: cosmosInput,
		evmInput:    evmInput,
		dashboard:   NewHtopDashboard(),
	}
}

func (m SimpleApp) Init() tea.Cmd {
	return textinput.Blink
}

func (m SimpleApp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Pass window size to dashboard
		if m.step == 3 {
			m.dashboard, cmd = m.dashboard.Update(msg)
			return m, cmd
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			switch m.step {
			case 0: // PK step
				if m.pkInput.Value() != "" {
					m.step = 1
					m.cosmosInput.Focus()
					// Use cached value if available
					if cached := GetCachedRPC("cosmos"); cached != "" {
						m.cosmosInput.SetValue(cached)
					}
					return m, nil
				}
			case 1: // Cosmos RPC step
				cosmosRPC := m.cosmosInput.Value()
				if cosmosRPC == "" {
					cosmosRPC = "http://localhost:1317"
				}
				CacheRPC("cosmos", cosmosRPC)
				m.step = 2
				m.evmInput.Focus()
				// Use cached value if available
				if cached := GetCachedRPC("evm"); cached != "" {
					m.evmInput.SetValue(cached)
				}
				return m, nil
			case 2: // EVM RPC step
				evmRPC := m.evmInput.Value()
				if evmRPC == "" {
					evmRPC = "http://localhost:8545"
				}
				CacheRPC("evm", evmRPC)

				// Initialize dashboard
				cosmosRPC := m.cosmosInput.Value()
				if cosmosRPC == "" {
					cosmosRPC = "http://localhost:1317"
				}
				m.dashboard.Init(m.pkInput.Value(), cosmosRPC, evmRPC)
				m.step = 3
				return m, nil
			}
		}
	}

	// Update appropriate input based on step
	switch m.step {
	case 0:
		m.pkInput, cmd = m.pkInput.Update(msg)
	case 1:
		m.cosmosInput, cmd = m.cosmosInput.Update(msg)
	case 2:
		m.evmInput, cmd = m.evmInput.Update(msg)
	case 3:
		m.dashboard, cmd = m.dashboard.Update(msg)
	}

	return m, cmd
}

func (m SimpleApp) View() string {
	titleStyleVar := cleanTitleStyle
	secondaryStyleVar := secondaryStyle

	switch m.step {
	case 0:
		return fmt.Sprintf(`
%s

Enter your private key:
%s

%s
`,
			titleStyleVar.Render("TXPILOT SETUP (1/3)"),
			m.pkInput.View(),
			secondaryStyleVar.Render("Press Enter to continue, Ctrl+C to quit"))

	case 1:
		return fmt.Sprintf(`
%s

Enter Cosmos RPC endpoint (or press Enter for default):
%s

%s
`,
			titleStyleVar.Render("COSMOS RPC SETUP (2/3)"),
			m.cosmosInput.View(),
			secondaryStyleVar.Render("Press Enter to continue, Ctrl+C to quit"))

	case 2:
		return fmt.Sprintf(`
%s

Enter EVM RPC endpoint (or press Enter for default):
%s

%s
`,
			titleStyleVar.Render("EVM RPC SETUP (3/3)"),
			m.evmInput.View(),
			secondaryStyleVar.Render("Press Enter to continue, Ctrl+C to quit"))

	case 3:
		return m.dashboard.View()
	}

	return ""
}
