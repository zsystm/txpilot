package ui

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type TxSelectorModel struct {
	ready        bool
	focused      int
	preSignedTxs []*types.Transaction
	privateKey   string
	evmRPC       string
	width        int
	height       int
	sending      bool
	lastTxTime   time.Time
	history      []string
	txStatuses   map[int]TxStatus // Track status of each transaction
}

type TxStatus struct {
	Sent      bool
	Confirmed bool
	Hash      string
	Checking  bool // Whether we're currently checking this transaction
}

type TxSentMsg struct {
	Hash string
	Err  error
}

type TxConfirmedMsg struct {
	TxIndex   int // Which transaction index this result is for
	Hash      string
	Confirmed bool
	Err       error
}

type CheckReceiptTickMsg struct {
	TxIndex int // Which transaction to check
}

// Colors for transaction selector
var (
	selectorHeaderStyle = lipgloss.NewStyle().
				Background(htopBg).
				Foreground(htopHeader).
				Bold(true).
				Padding(0, 1).
				Width(80)

	selectorTxStyle = lipgloss.NewStyle().
			Padding(0, 1)

	selectorFocusedStyle = lipgloss.NewStyle().
				Background(htopHighlight).
				Foreground(lipgloss.Color("0")).
				Bold(true).
				Padding(0, 1)

	loadingStyle = lipgloss.NewStyle().
			Foreground(htopWarning).
			Bold(true)

	confirmedStyle = lipgloss.NewStyle().
			Foreground(htopHighlight).
			Bold(true)
)

func NewTxSelectorModel() TxSelectorModel {
	return TxSelectorModel{
		preSignedTxs: []*types.Transaction{},
		focused:      0,
		width:        80,
		height:       24,
		history:      []string{},
		sending:      false,
		txStatuses:   make(map[int]TxStatus),
	}
}

func (m *TxSelectorModel) Init(pk, evmRPC string) {
	m.privateKey = pk
	m.evmRPC = evmRPC
	m.createPreSignedTransactions()
	m.ready = true
	m.addHistory("Transaction selector ready")
}

func (m *TxSelectorModel) createPreSignedTransactions() {
	randomTo := GenerateRandomEVMAddress()

	preSignedTxs, err := CreatePreSignedEVMTransactions(
		m.privateKey,
		m.evmRPC,
		randomTo,
		"100000000000000000", // 0.1 ETH in wei
		10,                   // Create 10 pre-signed transactions
	)

	if err != nil {
		m.addHistory(fmt.Sprintf("ERROR: Failed to create pre-signed txs: %v", err))
		m.preSignedTxs = []*types.Transaction{}
	} else {
		m.preSignedTxs = preSignedTxs
		m.focused = 0
		// Reset transaction statuses
		m.txStatuses = make(map[int]TxStatus)
		for i := range preSignedTxs {
			m.txStatuses[i] = TxStatus{Sent: false, Confirmed: false, Hash: "", Checking: false}
		}
		m.addHistory(fmt.Sprintf("Created %d pre-signed EVM transactions", len(preSignedTxs)))
	}
}

func (m *TxSelectorModel) addHistory(msg string) {
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, msg)
	m.history = append(m.history, entry)

	if len(m.history) > 20 {
		m.history = m.history[1:]
	}
}

func (m TxSelectorModel) Update(msg tea.Msg) (TxSelectorModel, tea.Cmd) {
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
		case "up", "k":
			if m.focused > 0 {
				m.focused--
			}

		case "down", "j":
			if m.focused < len(m.preSignedTxs)-1 {
				m.focused++
			}

		case "enter":
			if len(m.preSignedTxs) > 0 && m.focused < len(m.preSignedTxs) {
				m.sending = true
				m.lastTxTime = time.Now()
				selectedTx := m.preSignedTxs[m.focused]
				m.addHistory(fmt.Sprintf("Sending transaction with nonce %d...", selectedTx.Nonce()))
				return m, m.sendSelectedTransaction()
			}

		case "r", "R":
			m.createPreSignedTransactions()
			m.addHistory("Pre-signed transactions refreshed")

		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case TxSentMsg:
		m.sending = false
		if msg.Err != nil {
			duration := time.Since(m.lastTxTime)
			m.addHistory(fmt.Sprintf("FAILED: %s (%.2fs)", msg.Err.Error(), duration.Seconds()))
		} else {
			// Mark transaction as sent and start checking
			if status, exists := m.txStatuses[m.focused]; exists {
				status.Sent = true
				status.Hash = msg.Hash
				status.Checking = true
				m.txStatuses[m.focused] = status
			}
			m.addHistory(fmt.Sprintf("Transaction sent: %s", msg.Hash))
			m.addHistory("Waiting for confirmation...")

			// Start independent goroutine for this transaction
			txIndex := m.focused
			return m, m.startReceiptChecking(txIndex, msg.Hash)
		}

	case CheckReceiptTickMsg:
		// Check specific transaction and continue checking if not confirmed
		txIndex := msg.TxIndex
		if status, exists := m.txStatuses[txIndex]; exists && status.Checking && !status.Confirmed {
			// Continue checking this specific transaction
			return m, tea.Batch(
				m.checkSpecificTransactionReceipt(txIndex),
				tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
					return CheckReceiptTickMsg{TxIndex: txIndex}
				}),
			)
		}

	case TxConfirmedMsg:
		// Handle confirmation result for specific transaction
		if status, exists := m.txStatuses[msg.TxIndex]; exists {
			if msg.Err != nil {
				m.addHistory(fmt.Sprintf("Receipt check failed for tx %d: %v", msg.TxIndex+1, msg.Err))
				// Keep checking - the goroutine will retry
			} else if msg.Confirmed {
				// Mark as confirmed and stop checking
				status.Confirmed = true
				status.Checking = false
				m.txStatuses[msg.TxIndex] = status
				duration := time.Since(m.lastTxTime)
				m.addHistory(fmt.Sprintf("✅ CONFIRMED tx %d: %s (%.2fs)", msg.TxIndex+1, msg.Hash, duration.Seconds()))
			} else {
				// Still waiting - the goroutine will continue checking
				m.addHistory(fmt.Sprintf("Still waiting for tx %d confirmation...", msg.TxIndex+1))
			}
		}
	}

	return m, nil
}

func (m TxSelectorModel) sendSelectedTransaction() tea.Cmd {
	return func() tea.Msg {
		if m.focused >= len(m.preSignedTxs) {
			return TxSentMsg{Hash: "", Err: fmt.Errorf("invalid transaction index")}
		}

		tx := m.preSignedTxs[m.focused]
		hash, err := SendPreSignedEVMTransaction(m.evmRPC, tx)
		return TxSentMsg{Hash: hash, Err: err}
	}
}

func (m TxSelectorModel) startReceiptChecking(txIndex int, txHash string) tea.Cmd {
	// Start independent ticker for this transaction
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return CheckReceiptTickMsg{TxIndex: txIndex}
	})
}

func (m TxSelectorModel) checkSpecificTransactionReceipt(txIndex int) tea.Cmd {
	return func() tea.Msg {
		if status, exists := m.txStatuses[txIndex]; exists && status.Hash != "" {
			confirmed, err := GetTransactionReceipt(m.evmRPC, status.Hash)
			return TxConfirmedMsg{
				TxIndex:   txIndex,
				Hash:      status.Hash,
				Confirmed: confirmed,
				Err:       err,
			}
		}
		return TxConfirmedMsg{TxIndex: txIndex, Err: fmt.Errorf("transaction not found")}
	}
}

func (m TxSelectorModel) View() string {
	if !m.ready {
		return "Initializing transaction selector..."
	}

	lines := make([]string, m.height)
	currentLine := 0

	// Header
	header := selectorHeaderStyle.Render("TxPilot - Pre-Signed Transaction Selector")
	headerLen := len(stripAnsi(header))
	padding := m.width - headerLen
	if padding < 0 {
		padding = 0
	}
	lines[currentLine] = header + strings.Repeat(" ", padding)
	currentLine++

	// Empty line
	lines[currentLine] = strings.Repeat(" ", m.width)
	currentLine++

	// Get from address for display
	var fromAddr string
	if len(m.preSignedTxs) > 0 {
		privateKeyBytes, _ := ParsePrivateKey(m.privateKey)
		privateKey, _ := crypto.ToECDSA(privateKeyBytes)
		fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
		fromAddr = fromAddress.Hex()
	}

	// Table header
	tableHeader := fmt.Sprintf("%-3s %-8s %-12s %-12s %-8s %-6s %-10s",
		"#", "Nonce", "From", "To", "Value", "Sent", "Confirmed")
	headerLine := frameHeaderStyle.Render(tableHeader)
	headerLineLen := len(stripAnsi(headerLine))
	headerPadding := m.width - headerLineLen
	if headerPadding < 0 {
		headerPadding = 0
	}
	lines[currentLine] = headerLine + strings.Repeat(" ", headerPadding)
	currentLine++

	// Table separator line
	separator := strings.Repeat("─", m.width)
	lines[currentLine] = separator
	currentLine++

	// Transaction table rows
	for i, tx := range m.preSignedTxs {
		nonce := tx.Nonce()
		value := tx.Value()
		to := tx.To().Hex()

		// Convert wei to ETH for display
		ethValue := new(big.Float).Quo(new(big.Float).SetInt(value), big.NewFloat(1e18))

		// Get status
		status := m.txStatuses[i]
		sentStatus := "No"
		confirmedStatus := "No"

		if status.Sent {
			sentStatus = "Yes"
		}
		if status.Confirmed {
			confirmedStatus = "✅"
		} else if status.Sent {
			confirmedStatus = "..."
		}

		// Current transaction indicator
		prefix := " "
		if i == m.focused {
			prefix = "►"
		}

		// Show sending status
		sendingIndicator := ""
		if m.sending && i == m.focused {
			sendingIndicator = " 📡"
		} else if status.Checking && !status.Confirmed {
			sendingIndicator = " ⏳"
		}

		rowContent := fmt.Sprintf("%s%-2d %-8d %-12s %-12s %-8s %-6s %-10s%s",
			prefix,
			i+1,
			nonce,
			selectorTruncateAddr(fromAddr),
			selectorTruncateAddr(to),
			ethValue.Text('f', 3),
			sentStatus,
			confirmedStatus,
			sendingIndicator)

		// Apply focus styling
		style := selectorTxStyle
		if i == m.focused {
			style = selectorFocusedStyle
		}

		styledRow := style.Render(rowContent)
		styledRowLen := len(stripAnsi(styledRow))
		rowPadding := m.width - styledRowLen
		if rowPadding < 0 {
			rowPadding = 0
		}
		lines[currentLine] = styledRow + strings.Repeat(" ", rowPadding)
		currentLine++
	}

	// Fill remaining space before action log
	for currentLine < m.height-10 {
		lines[currentLine] = strings.Repeat(" ", m.width)
		currentLine++
	}

	// Action Log header
	actionLogHeader := frameHeaderStyle.Render("Action Log")
	actionLogHeaderLen := len(stripAnsi(actionLogHeader))
	actionLogPadding := m.width - actionLogHeaderLen
	if actionLogPadding < 0 {
		actionLogPadding = 0
	}
	lines[currentLine] = actionLogHeader + strings.Repeat(" ", actionLogPadding)
	currentLine++

	// Action Log content
	historyLines := 5 // Show last 5 log entries
	startIdx := 0
	if len(m.history) > historyLines {
		startIdx = len(m.history) - historyLines
	}

	for i := 0; i < historyLines; i++ {
		if startIdx+i < len(m.history) {
			hist := m.history[startIdx+i]
			if len(hist) > m.width-2 {
				hist = hist[:m.width-5] + "..."
			}
			histContent := selectorTxStyle.Render("  " + hist)
			histContentLen := len(stripAnsi(histContent))
			histContentPadding := m.width - histContentLen
			if histContentPadding < 0 {
				histContentPadding = 0
			}
			lines[currentLine] = histContent + strings.Repeat(" ", histContentPadding)
		} else {
			lines[currentLine] = strings.Repeat(" ", m.width)
		}
		currentLine++
	}

	// Footer
	footer := footerStyle.Render("↑/↓:Navigate  Enter:Send  R:Refresh  Esc:Back  Q:Quit")
	footerLen := len(stripAnsi(footer))
	footerPadding := m.width - footerLen
	if footerPadding < 0 {
		footerPadding = 0
	}
	lines[m.height-1] = footer + strings.Repeat(" ", footerPadding)

	return strings.Join(lines, "\n")
}

func selectorTruncateAddr(addr string) string {
	if len(addr) > 12 {
		return addr[:6] + ".." + addr[len(addr)-4:]
	}
	return addr
}
