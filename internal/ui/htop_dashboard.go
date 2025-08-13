package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ethereum/go-ethereum/core/types"
)

type HtopDashboard struct {
	ready          bool
	focused        int
	txOptions      []TxOption
	history        []string
	privateKey     string
	cosmosRPC      string
	evmRPC         string
	width          int
	height         int
	sending        bool
	lastTxTime     time.Time
	preSignedTxs   []*types.Transaction // Pre-signed EVM transactions
	currentTxIndex int                  // Current transaction index for pre-signed txs
	showTxSelector bool                 // Whether to show transaction selector
	txSelector     TxSelectorModel      // Transaction selector model
}

// htop-style colors
var (
	htopHeader    = lipgloss.Color("15")  // White
	htopBg        = lipgloss.Color("22")  // Dark Green
	htopBorder    = lipgloss.Color("240") // Gray
	htopHighlight = lipgloss.Color("46")  // Bright Green
	htopWarning   = lipgloss.Color("226") // Yellow
	htopError     = lipgloss.Color("196") // Red
	htopMuted     = lipgloss.Color("244") // Light Gray
)

// htop-style layout styles
var (
	headerStyle = lipgloss.NewStyle().
			Background(htopBg).
			Foreground(htopHeader).
			Bold(true).
			Padding(0, 1).
			Width(80)

	footerStyle = lipgloss.NewStyle().
			Background(htopBg).
			Foreground(htopHeader).
			Padding(0, 1).
			Width(80)

	frameHeaderStyle = lipgloss.NewStyle().
				Background(htopBorder).
				Foreground(htopHeader).
				Bold(true).
				Padding(0, 1)

	contentStyle = lipgloss.NewStyle().
			Padding(0, 1)

	htopFocusedStyle = lipgloss.NewStyle().
				Background(htopHighlight).
				Foreground(lipgloss.Color("0")).
				Bold(true)

	htopReadyStyle = lipgloss.NewStyle().
			Foreground(htopHighlight).
			Bold(true)

	firingStyle = lipgloss.NewStyle().
			Background(htopWarning).
			Foreground(lipgloss.Color("0")).
			Bold(true)
)

func NewHtopDashboard() HtopDashboard {
	return HtopDashboard{
		txOptions:      []TxOption{},
		history:        []string{},
		width:          80,
		height:         24,
		preSignedTxs:   []*types.Transaction{},
		currentTxIndex: 0,
		showTxSelector: false,
		txSelector:     NewTxSelectorModel(),
	}
}

func (m *HtopDashboard) Init(pk, cosmosRPC, evmRPC string) {
	m.privateKey = pk
	m.cosmosRPC = cosmosRPC
	m.evmRPC = evmRPC
	m.prepareTxOptions()
	// Don't create pre-signed transactions here - do it when entering EVM selector
	m.ready = true
	m.addHistory("System ready")
}

func (m *HtopDashboard) prepareTxOptions() {
	m.txOptions = []TxOption{}

	pkBytes, err := ParsePrivateKey(m.privateKey)
	if err != nil {
		m.addHistory("ERROR: Invalid private key")
		return
	}

	// Cosmos transaction
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

	// EVM transaction
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

func (m *HtopDashboard) addHistory(msg string) {
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, msg)
	m.history = append(m.history, entry)

	if len(m.history) > 50 {
		m.history = m.history[1:]
	}
}

func (m HtopDashboard) Update(msg tea.Msg) (HtopDashboard, tea.Cmd) {
	// If showing transaction selector, delegate to it
	if m.showTxSelector {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if msg.String() == "esc" || msg.String() == "b" {
				m.showTxSelector = false
				return m, nil
			}
		}

		var cmd tea.Cmd
		m.txSelector, cmd = m.txSelector.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width < 80 {
			m.width = 80
		}
		if m.height < 24 {
			m.height = 24
		}

	case tea.KeyMsg:
		if m.sending {
			return m, nil
		}

		switch msg.String() {
		case "c", "C":
			if len(m.txOptions) > 0 && m.txOptions[0].Chain == "COSMOS" {
				m.sending = true
				m.lastTxTime = time.Now()
				m.addHistory("FIRING Cosmos transaction")
				return m, m.sendTransaction(0)
			}

		case "e", "E":
			if len(m.txOptions) > 1 && m.txOptions[1].Chain == "EVM" {
				// Initialize transaction selector with pre-signed transactions
				m.txSelector.Init(m.privateKey, m.evmRPC)
				m.showTxSelector = true
				m.addHistory("Opening EVM transaction selector")
				return m, nil
			}

		case "r", "R":
			m.prepareTxOptions()
			m.addHistory("Transactions refreshed")

		case "j", "down":
			m.focused = (m.focused + 1) % len(m.txOptions)

		case "k", "up":
			m.focused--
			if m.focused < 0 {
				m.focused = len(m.txOptions) - 1
			}

		case "enter":
			// Execute the focused transaction type
			if len(m.txOptions) > 0 && m.focused < len(m.txOptions) {
				selectedTx := m.txOptions[m.focused]
				if selectedTx.Chain == "COSMOS" {
					m.sending = true
					m.lastTxTime = time.Now()
					m.addHistory("FIRING Cosmos transaction")
					return m, m.sendTransaction(m.focused)
				} else if selectedTx.Chain == "EVM" {
					// Initialize transaction selector with pre-signed transactions
					m.txSelector.Init(m.privateKey, m.evmRPC)
					m.showTxSelector = true
					m.addHistory("Opening EVM transaction selector")
					return m, nil
				}
			}
		}

	case SendResultMsg:
		m.sending = false
		duration := time.Since(m.lastTxTime)
		if msg.Err != nil {
			m.addHistory(fmt.Sprintf("FAILED: %s (%.2fs)", msg.Err.Error(), duration.Seconds()))
		} else {
			m.addHistory(fmt.Sprintf("SUCCESS: %s (%.2fs)", truncateHash(msg.Hash), duration.Seconds()))
		}
		m.prepareTxOptions()
	}

	return m, nil
}

func (m HtopDashboard) sendTransaction(idx int) tea.Cmd {
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

func (m HtopDashboard) sendPreSignedTransaction() tea.Cmd {
	return func() tea.Msg {
		// Check if we have pre-signed transactions available
		if len(m.preSignedTxs) == 0 || m.currentTxIndex >= len(m.preSignedTxs) {
			return SendResultMsg{Hash: "", Err: fmt.Errorf("no pre-signed transactions available")}
		}

		// Use the next pre-signed transaction
		tx := m.preSignedTxs[m.currentTxIndex]

		// Send the pre-signed transaction
		hash, err := SendPreSignedEVMTransaction(m.evmRPC, tx)

		// Increment the index for next transaction (handled in the update)

		return SendResultMsg{Hash: hash, Err: err}
	}
}

func (m HtopDashboard) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Show transaction selector if active
	if m.showTxSelector {
		return m.txSelector.View()
	}

	// Fixed layout like htop
	lines := make([]string, m.height)

	// Header bar (line 0)
	header := headerStyle.Render("TxPilot v1.0 - Transaction Launcher")
	headerLen := len(stripAnsi(header))
	padding := m.width - headerLen
	if padding < 0 {
		padding = 0
	}
	lines[0] = header + strings.Repeat(" ", padding)

	// Empty line (line 1)
	if m.width > 0 {
		lines[1] = strings.Repeat(" ", m.width)
	} else {
		lines[1] = ""
	}

	// Transaction section header (line 2)
	txHeader := frameHeaderStyle.Render("Ready Transactions")
	txHeaderLen := len(stripAnsi(txHeader))
	txPadding := m.width - txHeaderLen
	if txPadding < 0 {
		txPadding = 0
	}
	lines[2] = txHeader + strings.Repeat(" ", txPadding)

	// Transaction list (lines 3-8)
	lineNum := 3
	for i, opt := range m.txOptions {
		if lineNum >= m.height-10 { // Leave room for history
			break
		}

		// Transaction line
		prefix := "  "
		style := contentStyle
		if i == m.focused {
			prefix = "> "
			style = htopFocusedStyle
		}

		status := "READY"
		statusStyle := htopReadyStyle
		if m.sending && i == m.focused {
			status = "FIRING"
			statusStyle = firingStyle
		}

		chainName := opt.Chain
		if opt.Chain == "EVM" {
			chainName = lipgloss.NewStyle().Foreground(htopWarning).Render("EVM")
		} else {
			chainName = lipgloss.NewStyle().Foreground(htopHighlight).Render("COSMOS")
		}

		line := fmt.Sprintf("%s[%s] %s %s", prefix, opt.Key, chainName, opt.Type)
		content := style.Render(line)
		contentLen := len(stripAnsi(content))
		contentPadding := m.width - contentLen
		if contentPadding < 0 {
			contentPadding = 0
		}
		lines[lineNum] = content + strings.Repeat(" ", contentPadding)
		lineNum++

		// Details line
		details := fmt.Sprintf("    %s | %s -> %s | %s",
			opt.Description,
			htopTruncateAddr(opt.From),
			htopTruncateAddr(opt.To),
			statusStyle.Render(status))
		detailContent := contentStyle.Render(details)
		detailLen := len(stripAnsi(detailContent))
		detailPadding := m.width - detailLen
		if detailPadding < 0 {
			detailPadding = 0
		}
		lines[lineNum] = detailContent + strings.Repeat(" ", detailPadding)
		lineNum++

		// Empty line
		if m.width > 0 {
			lines[lineNum] = strings.Repeat(" ", m.width)
		} else {
			lines[lineNum] = ""
		}
		lineNum++
	}

	// Fill remaining transaction area
	for lineNum < m.height-10 {
		if m.width > 0 {
			lines[lineNum] = strings.Repeat(" ", m.width)
		} else {
			lines[lineNum] = ""
		}
		lineNum++
	}

	// Fill remaining space
	for lineNum < m.height-1 {
		if m.width > 0 {
			lines[lineNum] = strings.Repeat(" ", m.width)
		} else {
			lines[lineNum] = ""
		}
		lineNum++
	}

	// Footer (last line)
	footer := footerStyle.Render("↑/↓:Navigate  Enter:Select  C:Cosmos  E:EVM  R:Refresh  Q:Quit")
	footerLen := len(stripAnsi(footer))
	footerPadding := m.width - footerLen
	if footerPadding < 0 {
		footerPadding = 0
	}
	lines[m.height-1] = footer + strings.Repeat(" ", footerPadding)

	return strings.Join(lines, "\n")
}

// Helper to strip ANSI codes for length calculation
func stripAnsi(s string) string {
	// Simple ANSI stripping - in real app use proper library
	result := s
	ansiCodes := []string{"\033[0m", "\033[1m", "\033[22m"}
	for _, code := range ansiCodes {
		result = strings.ReplaceAll(result, code, "")
	}
	// Remove color codes (very basic)
	for i := 0; i < 10; i++ {
		result = strings.ReplaceAll(result, fmt.Sprintf("\033[3%dm", i), "")
		result = strings.ReplaceAll(result, fmt.Sprintf("\033[4%dm", i), "")
		result = strings.ReplaceAll(result, fmt.Sprintf("\033[9%dm", i), "")
		result = strings.ReplaceAll(result, fmt.Sprintf("\033[10%dm", i), "")
	}
	return result
}

func htopTruncateAddr(addr string) string {
	if len(addr) > 12 {
		return addr[:6] + ".." + addr[len(addr)-4:]
	}
	return addr
}
