package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type CosmosSelectorModel struct {
	ready             bool
	focused           int
	preSignedCosmosTx []map[string]interface{}
	privateKey        string
	cosmosRPC         string
	width             int
	height            int
	sending           bool
	lastTxTime        time.Time
	history           []string
	txStatuses        map[int]CosmosTxStatus // Track status of each transaction
}

type CosmosTxStatus struct {
	Sent      bool
	Confirmed bool
	Hash      string
	Checking  bool // Whether we're currently checking this transaction
}

type CosmosTxSentMsg struct {
	Hash string
	Err  error
}

type CosmosTxConfirmedMsg struct {
	TxIndex   int // Which transaction index this result is for
	Hash      string
	Confirmed bool
	Err       error
}

type CosmosCheckReceiptTickMsg struct {
	TxIndex int // Which transaction to check
}

func NewCosmosSelectorModel() CosmosSelectorModel {
	return CosmosSelectorModel{
		preSignedCosmosTx: []map[string]interface{}{},
		focused:           0,
		width:             80,
		height:            24,
		history:           []string{},
		sending:           false,
		txStatuses:        make(map[int]CosmosTxStatus),
	}
}

func (m *CosmosSelectorModel) Init(pk, cosmosRPC string) {
	m.privateKey = pk
	m.cosmosRPC = cosmosRPC
	m.createPreSignedCosmosTransactions()
	m.ready = true
	m.addHistory("Cosmos transaction selector ready")
}

func (m *CosmosSelectorModel) createPreSignedCosmosTransactions() {
	randomTo, _ := GenerateRandomCosmosAddress("cosmos")

	preSignedTxs, err := CreatePreSignedCosmosTransactions(
		m.privateKey,
		m.cosmosRPC,
		randomTo,
		"1000uatom", // 1000 uatom
		10,          // Create 10 pre-signed transactions
	)

	if err != nil {
		m.addHistory(fmt.Sprintf("ERROR: Failed to create pre-signed Cosmos txs: %v", err))
		m.preSignedCosmosTx = []map[string]interface{}{}
	} else {
		m.preSignedCosmosTx = preSignedTxs
		m.focused = 0
		// Reset transaction statuses
		m.txStatuses = make(map[int]CosmosTxStatus)
		for i := range preSignedTxs {
			m.txStatuses[i] = CosmosTxStatus{Sent: false, Confirmed: false, Hash: "", Checking: false}
		}
		m.addHistory(fmt.Sprintf("Created %d pre-signed Cosmos transactions", len(preSignedTxs)))
	}
}

func (m *CosmosSelectorModel) addHistory(msg string) {
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, msg)
	m.history = append(m.history, entry)

	if len(m.history) > 20 {
		m.history = m.history[1:]
	}
}

func (m CosmosSelectorModel) Update(msg tea.Msg) (CosmosSelectorModel, tea.Cmd) {
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
			if m.focused < len(m.preSignedCosmosTx)-1 {
				m.focused++
			}

		case "enter":
			if len(m.preSignedCosmosTx) > 0 && m.focused < len(m.preSignedCosmosTx) {
				m.sending = true
				m.lastTxTime = time.Now()
				selectedTx := m.preSignedCosmosTx[m.focused]
				sequence := selectedTx["sequence"].(string)
				m.addHistory(fmt.Sprintf("Sending Cosmos transaction with sequence %s...", sequence))
				return m, m.sendSelectedCosmosTransaction()
			}

		case "r", "R":
			m.createPreSignedCosmosTransactions()
			m.addHistory("Pre-signed Cosmos transactions refreshed")

		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case CosmosTxSentMsg:
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
			m.addHistory(fmt.Sprintf("Cosmos transaction sent: %s", msg.Hash))
			m.addHistory("Waiting for confirmation...")

			// Start independent goroutine for this transaction
			txIndex := m.focused
			return m, m.startCosmosReceiptChecking(txIndex, msg.Hash)
		}

	case CosmosCheckReceiptTickMsg:
		// Check specific transaction and continue checking if not confirmed
		txIndex := msg.TxIndex
		if status, exists := m.txStatuses[txIndex]; exists && status.Checking && !status.Confirmed && status.Hash != "" {
			// Only check the receipt, don't schedule next tick yet
			return m, m.checkSpecificCosmosTransactionReceipt(txIndex)
		}
		// If transaction is already confirmed or not checking, stop the ticker

	case CosmosTxConfirmedMsg:
		// Handle confirmation result for specific transaction
		if status, exists := m.txStatuses[msg.TxIndex]; exists && msg.Hash != "" {
			if msg.Err != nil {
				// Log error and continue checking
				if status.Hash != "" {
					m.addHistory(fmt.Sprintf("Receipt check failed for tx %d: %v", msg.TxIndex+1, msg.Err))
				}
				// Schedule next check only if still checking
				if status.Checking && !status.Confirmed {
					return m, tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
						return CosmosCheckReceiptTickMsg{TxIndex: msg.TxIndex}
					})
				}
			} else if msg.Confirmed {
				// Mark as confirmed and stop checking
				status.Confirmed = true
				status.Checking = false
				m.txStatuses[msg.TxIndex] = status
				duration := time.Since(m.lastTxTime)
				m.addHistory(fmt.Sprintf("CONFIRMED tx %d: %s (%.2fs)", msg.TxIndex+1, msg.Hash, duration.Seconds()))
				// Don't schedule more checks for this transaction
			} else {
				// Not confirmed yet, schedule next check
				if status.Checking && !status.Confirmed {
					return m, tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
						return CosmosCheckReceiptTickMsg{TxIndex: msg.TxIndex}
					})
				}
			}
		}
		// Ignore empty results from already confirmed transactions
	}

	return m, nil
}

func (m CosmosSelectorModel) sendSelectedCosmosTransaction() tea.Cmd {
	return func() tea.Msg {
		if m.focused >= len(m.preSignedCosmosTx) {
			return CosmosTxSentMsg{Hash: "", Err: fmt.Errorf("invalid transaction index")}
		}

		txData := m.preSignedCosmosTx[m.focused]
		hash, err := SendPreSignedCosmosTransaction(m.cosmosRPC, m.privateKey, txData)
		return CosmosTxSentMsg{Hash: hash, Err: err}
	}
}

func (m CosmosSelectorModel) startCosmosReceiptChecking(txIndex int, txHash string) tea.Cmd {
	// Start independent ticker for this transaction
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return CosmosCheckReceiptTickMsg{TxIndex: txIndex}
	})
}

func (m CosmosSelectorModel) checkSpecificCosmosTransactionReceipt(txIndex int) tea.Cmd {
	return func() tea.Msg {
		if status, exists := m.txStatuses[txIndex]; exists && status.Hash != "" && !status.Confirmed {
			// Only check if transaction hasn't been confirmed yet
			confirmed, err := GetCosmosTransactionReceipt(m.cosmosRPC, status.Hash)
			return CosmosTxConfirmedMsg{
				TxIndex:   txIndex,
				Hash:      status.Hash,
				Confirmed: confirmed,
				Err:       err,
			}
		}
		// Don't check if already confirmed or transaction not found
		return CosmosTxConfirmedMsg{TxIndex: txIndex, Hash: "", Confirmed: false, Err: nil}
	}
}

func (m CosmosSelectorModel) View() string {
	if !m.ready {
		return "Initializing Cosmos transaction selector..."
	}

	lines := make([]string, m.height)
	currentLine := 0

	// Header
	header := selectorHeaderStyle.Render("TxPilot - Cosmos Pre-Signed Transaction Selector")
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

	// Get from address for display - already available in transaction data

	// Table header
	tableHeader := fmt.Sprintf("%-3s %-8s %-15s %-15s %-12s %-6s %-10s",
		"#", "Sequence", "From", "To", "Amount", "Sent", "Confirmed")
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
	for i, txData := range m.preSignedCosmosTx {
		sequence := txData["sequence"].(string)
		from := txData["from"].(string)
		to := txData["to"].(string)
		amount := txData["amount"].(string)

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

		rowContent := fmt.Sprintf("%s%-2d %-8s %-15s %-15s %-12s %-6s %-10s%s",
			prefix,
			i+1,
			sequence,
			cosmosAddrTruncate(from),
			cosmosAddrTruncate(to),
			amount,
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

func cosmosAddrTruncate(addr string) string {
	if len(addr) > 15 {
		return addr[:6] + ".." + addr[len(addr)-6:]
	}
	return addr
}
