package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TxOption struct {
	Chain       string
	Type        string
	Description string
	Ready       bool
	SignedTx    []byte
	From        string
	To          string
	Amount      string
	Key         string // Hotkey to send
}

type DashboardModel struct {
	ready      bool
	focused    int
	txOptions  []TxOption
	viewport   viewport.Model
	history    []string
	privateKey string
	cosmosRPC  string
	evmRPC     string
	width      int
	height     int
	sending    bool
	lastResult string
}

var (
	// Professional color scheme inspired by k9s/lazygit
	bgDark   = lipgloss.Color("#1a1b26")
	bgLight  = lipgloss.Color("#24283b")
	fgDim    = lipgloss.Color("#565f89")
	fgNormal = lipgloss.Color("#a9b1d6")
	fgBright = lipgloss.Color("#c0caf5")
	accent   = lipgloss.Color("#7aa2f7")
	green    = lipgloss.Color("#9ece6a")
	yellow   = lipgloss.Color("#e0af68")
	red      = lipgloss.Color("#f7768e")
	magenta  = lipgloss.Color("#bb9af7")
	cyan     = lipgloss.Color("#7dcfff")
)

func NewDashboard() DashboardModel {
	return DashboardModel{
		txOptions: []TxOption{},
		history:   []string{},
		viewport:  viewport.New(80, 10),
	}
}

func (m *DashboardModel) Init(pk, cosmosRPC, evmRPC string) {
	m.privateKey = pk
	m.cosmosRPC = cosmosRPC
	m.evmRPC = evmRPC
	m.prepareTxOptions()
	m.ready = true
}

func (m *DashboardModel) prepareTxOptions() {
	// Parse private key
	pkBytes, err := ParsePrivateKey(m.privateKey)
	if err != nil {
		m.addHistory(fmt.Sprintf("❌ Failed to parse private key: %v", err))
		return
	}

	// Prepare Cosmos transaction
	cosmosAddr, _ := DeriveCosmosAddress(pkBytes, "cosmos")
	cosmosTo, _ := GenerateRandomCosmosAddress("cosmos")

	m.txOptions = append(m.txOptions, TxOption{
		Chain:       "COSMOS",
		Type:        "bank/send",
		Description: "Send 1000 ATOM",
		From:        truncateAddr(cosmosAddr),
		To:          truncateAddr(cosmosTo),
		Amount:      "1000 ATOM",
		Key:         "c",
		Ready:       true,
	})

	// Prepare EVM transaction
	evmAddr, _ := DeriveEVMAddress(pkBytes)
	evmTo := GenerateRandomEVMAddress()

	m.txOptions = append(m.txOptions, TxOption{
		Chain:       "EVM",
		Type:        "eth/transfer",
		Description: "Send 0.1 ETH",
		From:        truncateAddr(evmAddr),
		To:          truncateAddr(evmTo),
		Amount:      "0.1 ETH",
		Key:         "e",
		Ready:       true,
	})

	m.addHistory("🚀 Transactions prepared and ready to fire!")
	m.addHistory("Press [c] for Cosmos or [e] for EVM transaction")
}

func (m *DashboardModel) addHistory(msg string) {
	timestamp := "[" + time.Now().Format("15:04:05") + "]"
	m.history = append(m.history, timestamp+" "+msg)
	if len(m.history) > 100 {
		m.history = m.history[1:]
	}
	m.viewport.SetContent(strings.Join(m.history, "\n"))
	m.viewport.GotoBottom()
}

func truncateAddr(addr string) string {
	if len(addr) > 16 {
		return addr[:8] + "..." + addr[len(addr)-6:]
	}
	return addr
}

func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 20

	case tea.KeyMsg:
		if m.sending {
			return m, nil
		}

		switch msg.String() {
		case "c", "C":
			// Send Cosmos transaction
			if len(m.txOptions) > 0 && m.txOptions[0].Chain == "COSMOS" {
				m.sending = true
				m.addHistory("🚀 FIRING COSMOS TRANSACTION...")
				return m, m.sendTransaction(0)
			}

		case "e", "E":
			// Send EVM transaction
			if len(m.txOptions) > 1 && m.txOptions[1].Chain == "EVM" {
				m.sending = true
				m.addHistory("🚀 FIRING EVM TRANSACTION...")
				return m, m.sendTransaction(1)
			}

		case "r", "R":
			// Refresh/regenerate transactions
			m.prepareTxOptions()
			m.addHistory("♻️  Transactions refreshed with new addresses")

		case "j", "down":
			m.focused = (m.focused + 1) % len(m.txOptions)

		case "k", "up":
			m.focused--
			if m.focused < 0 {
				m.focused = len(m.txOptions) - 1
			}

		case "enter", " ":
			// Send focused transaction
			if m.focused < len(m.txOptions) {
				m.sending = true
				chain := m.txOptions[m.focused].Chain
				m.addHistory(fmt.Sprintf("🚀 FIRING %s TRANSACTION...", chain))
				return m, m.sendTransaction(m.focused)
			}
		}

	case SendResultMsg:
		m.sending = false
		if msg.Err != nil {
			m.addHistory(fmt.Sprintf("❌ FAILED: %v", msg.Err))
		} else {
			m.addHistory(fmt.Sprintf("✅ SUCCESS! TX: %s", msg.Hash))
		}
		// Refresh transactions for next shot
		m.prepareTxOptions()
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m DashboardModel) sendTransaction(idx int) tea.Cmd {
	if idx >= len(m.txOptions) {
		return nil
	}

	opt := m.txOptions[idx]
	return func() tea.Msg {
		state := SharedState{
			Chain:  strings.ToLower(opt.Chain),
			TxType: opt.Type,
			PK:     m.privateKey,
			Inputs: map[string]string{},
		}

		if opt.Chain == "COSMOS" {
			state.RPC = m.cosmosRPC
			// Auto-fill Cosmos params
			pkBytes, _ := ParsePrivateKey(m.privateKey)
			fromAddr, _ := DeriveCosmosAddress(pkBytes, "cosmos")
			toAddr, _ := GenerateRandomCosmosAddress("cosmos")

			state.Inputs["from_addr"] = fromAddr
			state.Inputs["to_addr"] = toAddr
			state.Inputs["amount"] = "1000uatom"
			state.Inputs["node(rpc)"] = m.cosmosRPC
			state.Inputs["chain_id"] = "cosmoshub-4"
		} else {
			state.RPC = m.evmRPC
			// Auto-fill EVM params
			pkBytes, _ := ParsePrivateKey(m.privateKey)
			fromAddr, _ := DeriveEVMAddress(pkBytes)
			toAddr := GenerateRandomEVMAddress()

			state.Inputs["from"] = fromAddr
			state.Inputs["to"] = toAddr
			state.Inputs["value(wei)"] = "100000000000000000"
			state.Inputs["gas"] = "21000"
			state.Inputs["gas_price(wei)"] = "20000000000"
			state.Inputs["nonce(optional)"] = "0"
			state.Inputs["rpc_url"] = m.evmRPC
		}

		hash, err := txSend(state)
		return SendResultMsg{Hash: hash, Err: err}
	}
}

func (m DashboardModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var result strings.Builder

	// Header
	result.WriteString("🚀 TXPILOT - READY TO FIRE\n")
	result.WriteString("═══════════════════════════\n\n")

	// Transaction options
	for i, opt := range m.txOptions {
		prefix := "  "
		if i == m.focused {
			prefix = "► "
		}

		status := "✅ READY"
		if m.sending && i == m.focused {
			status = "🚀 FIRING"
		}

		result.WriteString(fmt.Sprintf("%s[%s] %s • %s\n", prefix, opt.Key, opt.Chain, opt.Type))
		result.WriteString(fmt.Sprintf("    %s | %s → %s | %s\n", opt.Description, opt.From, opt.To, status))
		result.WriteString("\n")
	}

	// Controls
	result.WriteString("CONTROLS: [c]osmos [e]vm [r]efresh [q]uit\n\n")

	// History
	result.WriteString("TRANSACTION HISTORY:\n")
	result.WriteString("──────────────────\n")

	if len(m.history) == 0 {
		result.WriteString("No transactions yet...\n")
	} else {
		// Show last 10 entries
		start := 0
		if len(m.history) > 10 {
			start = len(m.history) - 10
		}
		for i := start; i < len(m.history); i++ {
			result.WriteString(m.history[i] + "\n")
		}
	}

	return result.String()
}

// Keymap for better key handling
type keyMap struct {
	Cosmos  key.Binding
	EVM     key.Binding
	Refresh key.Binding
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Quit    key.Binding
}

var keys = keyMap{
	Cosmos: key.NewBinding(
		key.WithKeys("c", "C"),
		key.WithHelp("c", "send cosmos tx"),
	),
	EVM: key.NewBinding(
		key.WithKeys("e", "E"),
		key.WithHelp("e", "send evm tx"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r", "R"),
		key.WithHelp("r", "refresh"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter", " "),
		key.WithHelp("enter", "send"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}
