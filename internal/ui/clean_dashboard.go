package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type CleanDashboard struct {
	ready        bool
	focused      int
	txOptions    []TxOption
	history      []string
	historyView  viewport.Model
	privateKey   string
	cosmosRPC    string
	evmRPC       string
	width        int
	height       int
	sending      bool
	lastTxTime   time.Time
}

// Color scheme - professional and clean
var (
	colorPrimary   = lipgloss.Color("39")  // Blue
	colorSuccess   = lipgloss.Color("46")  // Green  
	colorWarning   = lipgloss.Color("226") // Yellow
	colorError     = lipgloss.Color("196") // Red
	colorSecondary = lipgloss.Color("244") // Gray
	colorBorder    = lipgloss.Color("240") // Dark Gray
	colorBg        = lipgloss.Color("235") // Dark Background
)

// Styles
var (
	frameStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(1, 2)
	
	cleanTitleStyle = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true)
	
	successStyle = lipgloss.NewStyle().
		Foreground(colorSuccess).
		Bold(true)
	
	warningStyle = lipgloss.NewStyle().
		Foreground(colorWarning).
		Bold(true)
	
	errorStyle = lipgloss.NewStyle().
		Foreground(colorError).
		Bold(true)
	
	secondaryStyle = lipgloss.NewStyle().
		Foreground(colorSecondary)
	
	focusedStyle = lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true)
	
	readyStyle = lipgloss.NewStyle().
		Foreground(colorSuccess)
)

func NewCleanDashboard() CleanDashboard {
	return CleanDashboard{
		txOptions:   []TxOption{},
		history:     []string{},
		historyView: viewport.New(60, 15),
	}
}

func (m *CleanDashboard) Init(pk, cosmosRPC, evmRPC string) {
	m.privateKey = pk
	m.cosmosRPC = cosmosRPC
	m.evmRPC = evmRPC
	m.prepareTxOptions()
	m.ready = true
	m.addHistory("System initialized - transactions ready")
	m.addHistory("Press [c] for Cosmos or [e] for EVM")
}

func (m *CleanDashboard) prepareTxOptions() {
	// Clear existing options
	m.txOptions = []TxOption{}
	
	// Parse private key
	pkBytes, err := ParsePrivateKey(m.privateKey)
	if err != nil {
		m.addHistory("ERROR: Failed to parse private key")
		return
	}

	// Prepare Cosmos transaction
	cosmosAddr, _ := DeriveCosmosAddress(pkBytes, "cosmos")
	cosmosTo, _ := GenerateRandomCosmosAddress("cosmos")
	
	m.txOptions = append(m.txOptions, TxOption{
		Chain:       "COSMOS",
		Type:        "bank/send",
		Description: "Send 1000 ATOM",
		From:        cosmosAddr,
		To:          cosmosTo,
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
		From:        evmAddr,
		To:          evmTo,
		Amount:      "0.1 ETH",
		Key:         "e",
		Ready:       true,
	})
}

func (m *CleanDashboard) addHistory(msg string) {
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, msg)
	m.history = append(m.history, entry)
	
	// Keep last 100 entries
	if len(m.history) > 100 {
		m.history = m.history[1:]
	}
	
	m.historyView.SetContent(strings.Join(m.history, "\n"))
	m.historyView.GotoBottom()
}

func (m CleanDashboard) Update(msg tea.Msg) (CleanDashboard, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Set reasonable defaults if not set
		if m.width <= 0 {
			m.width = 120
		}
		if m.height <= 0 {
			m.height = 30
		}
		
	case tea.KeyMsg:
		if m.sending {
			return m, nil
		}
		
		switch msg.String() {
		case "c", "C":
			// Send Cosmos transaction
			if len(m.txOptions) > 0 && m.txOptions[0].Chain == "COSMOS" {
				m.sending = true
				m.lastTxTime = time.Now()
				m.addHistory("FIRING Cosmos transaction...")
				return m, m.sendTransaction(0)
			} else {
				m.addHistory("No Cosmos transaction available")
			}
			
		case "e", "E":
			// Send EVM transaction  
			if len(m.txOptions) > 1 && m.txOptions[1].Chain == "EVM" {
				m.sending = true
				m.lastTxTime = time.Now()
				m.addHistory("FIRING EVM transaction...")
				return m, m.sendTransaction(1)
			} else {
				m.addHistory("No EVM transaction available")
			}
			
		case "r", "R":
			// Refresh transactions
			m.prepareTxOptions()
			m.addHistory("Transactions refreshed with new addresses")
			
		case "j", "down":
			m.focused = (m.focused + 1) % len(m.txOptions)
			
		case "k", "up":
			m.focused--
			if m.focused < 0 {
				m.focused = len(m.txOptions) - 1
			}
		}
		
	case SendResultMsg:
		m.sending = false
		duration := time.Since(m.lastTxTime)
		if msg.Err != nil {
			m.addHistory(fmt.Sprintf("FAILED: %v (%.2fs)", msg.Err, duration.Seconds()))
		} else {
			m.addHistory(fmt.Sprintf("SUCCESS: %s (%.2fs)", truncateHash(msg.Hash), duration.Seconds()))
		}
		// Refresh transactions for next shot
		m.prepareTxOptions()
		m.addHistory("Transactions refreshed - ready to fire again")
	}

	// Update history viewport
	var cmd tea.Cmd
	m.historyView, cmd = m.historyView.Update(msg)
	return m, cmd
}

func (m CleanDashboard) sendTransaction(idx int) tea.Cmd {
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
			state.Inputs["from_addr"] = opt.From
			state.Inputs["to_addr"] = opt.To
			state.Inputs["amount"] = "1000uatom"
			state.Inputs["node(rpc)"] = m.cosmosRPC
			state.Inputs["chain_id"] = "cosmoshub-4"
		} else {
			state.RPC = m.evmRPC
			state.Inputs["from"] = opt.From
			state.Inputs["to"] = opt.To
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

func (m CleanDashboard) View() string {
	if !m.ready {
		return "Initializing system..."
	}

	// Header
	header := cleanTitleStyle.Render("TXPILOT - TRANSACTION LAUNCHER")
	separator := strings.Repeat("─", 80)
	
	// Transaction Options Panel - left side
	var txLines []string
	txLines = append(txLines, cleanTitleStyle.Render("READY TRANSACTIONS"))
	txLines = append(txLines, "")
	
	for i, opt := range m.txOptions {
		prefix := "  "
		if i == m.focused {
			prefix = "> "
		}
		
		status := "READY"
		statusStyle := readyStyle
		if m.sending && i == m.focused {
			status = "FIRING"
			statusStyle = warningStyle
		}
		
		chainStyle := successStyle
		if opt.Chain == "EVM" {
			chainStyle = warningStyle
		}
		
		// Format: [key] CHAIN • type
		line1 := fmt.Sprintf("%s[%s] %s • %s", 
			prefix,
			opt.Key,
			chainStyle.Render(opt.Chain),
			opt.Type)
		
		// Truncate line if too long
		if len(line1) > 45 {
			line1 = line1[:42] + "..."
		}
		
		// Format: description | from → to | status (truncate for width)
		line2 := fmt.Sprintf("    %s | %s → %s | %s",
			opt.Description,
			cleanTruncateAddr(opt.From),
			cleanTruncateAddr(opt.To),
			statusStyle.Render(status))
		
		// Truncate line if too long
		if len(line2) > 45 {
			line2 = line2[:42] + "..."
		}
		
		txLines = append(txLines, line1)
		txLines = append(txLines, line2)
		txLines = append(txLines, "")
	}
	
	// Add some padding lines to make left panel bigger
	for len(txLines) < 12 {
		txLines = append(txLines, "")
	}
	
	txPanel := frameStyle.Render(strings.Join(txLines, "\n"))
	
	// History Panel - right side  
	var historyLines []string
	historyLines = append(historyLines, cleanTitleStyle.Render("TRANSACTION HISTORY"))
	historyLines = append(historyLines, "")
	
	if len(m.history) == 0 {
		historyLines = append(historyLines, secondaryStyle.Render("No transactions yet..."))
	} else {
		// Show last 8 entries 
		start := 0
		if len(m.history) > 8 {
			start = len(m.history) - 8
		}
		for i := start; i < len(m.history); i++ {
			line := m.history[i]
			// Truncate history lines to match panel width
			if len(line) > 45 {
				line = line[:42] + "..."
			}
			historyLines = append(historyLines, line)
		}
	}
	
	// Add padding lines to match left panel height
	for len(historyLines) < 12 {
		historyLines = append(historyLines, "")
	}
	
	historyPanel := frameStyle.Render(strings.Join(historyLines, "\n"))
	
	// Controls Panel
	controls := secondaryStyle.Render("CONTROLS: [c]osmos [e]vm [r]efresh [q]uit")
	
	// Layout - split screen
	mainView := lipgloss.JoinHorizontal(lipgloss.Top,
		txPanel,
		"   ", // spacing
		historyPanel,
	)
	
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		separator,
		"",
		mainView,
		"",
		controls,
	)
}

func cleanTruncateAddr(addr string) string {
	if len(addr) > 16 {
		return addr[:8] + "..." + addr[len(addr)-6:]
	}
	return addr
}

func truncateHash(hash string) string {
	if len(hash) > 20 {
		return hash[:10] + "..." + hash[len(hash)-10:]
	}
	return hash
}