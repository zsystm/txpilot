package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("235")).
		Padding(0, 2).
		MarginBottom(1)
	
	labelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))
	
	okStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("42"))
	
	errStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196"))
	
	boxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		MarginTop(1).
		MarginBottom(1)
	
	inputStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("205"))
	
	highlightStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170"))
)

// ---------- Welcome ----------

type WelcomeModel struct{ done bool }

func NewWelcome() WelcomeModel { return WelcomeModel{} }

func (m WelcomeModel) Update(msg tea.Msg) (WelcomeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == keyNext {
			m.done = true
		}
	}
	return m, nil
}

func (m WelcomeModel) View() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		MarginBottom(2).
		Render("🚀 TxPilot")
	
	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		MarginBottom(1).
		Render("Multi-Chain Transaction Wizard")
	
	instructions := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250")).
		Bold(true).
		MarginTop(2).
		Render("Press Enter to start")
	
	content := lipgloss.JoinVertical(lipgloss.Center,
		title,
		subtitle,
		instructions,
	)
	
	return boxStyle.Render(content)
}

// ---------- Menu (Select Chain / Select Tx) ----------

type MenuModel struct {
	label string
	list  list.Model
}

type item string

func (i item) Title() string       { return string(i) }
func (i item) Description() string { return "" }
func (i item) FilterValue() string { return string(i) }

func NewMenu(label string, items []string) MenuModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("170")).
		BorderLeft(true).
		BorderForeground(lipgloss.Color("170")).
		BorderStyle(lipgloss.ThickBorder()).
		PaddingLeft(1)
	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		PaddingLeft(3).
		Foreground(lipgloss.Color("250"))
	
	l := list.New([]list.Item{}, delegate, 40, len(items)+2)
	l.SetShowHelp(false)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	
	m := MenuModel{label: label, list: l}
	if len(items) > 0 {
		m.SetItems(items)
	}
	return m
}

func (m *MenuModel) SetItems(items []string) {
	arr := make([]list.Item, len(items))
	for i, s := range items {
		arr[i] = item(s)
	}
	m.list.SetItems(arr)
}

func (m MenuModel) Selected() string {
	if it, ok := m.list.SelectedItem().(item); ok {
		return string(it)
	}
	return ""
}

func (m MenuModel) Update(msg tea.Msg) (MenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == keyNext {
			// Handle Enter key to select item
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m MenuModel) View() string {
	title := titleStyle.Render("📋 " + m.label)
	
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		MarginTop(1).
		Render("↑/↓ to navigate • Enter to select")
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		m.list.View(),
		hint,
	)
	
	return boxStyle.Render(content)
}

// ---------- PK Entry ----------

type PKModel struct {
	input textinput.Model
	done  bool
}

func NewPKModel() PKModel {
	ti := textinput.New()
	ti.Placeholder = "Enter hex private key (64 characters)"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	ti.Focus()
	return PKModel{input: ti}
}

func (m PKModel) Update(msg tea.Msg) (PKModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		s := msg.String()
		if s == keyNext {
			m.done = true
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m PKModel) View() string {
	title := titleStyle.Render("🔑 Enter Private Key")
	
	description := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		MarginBottom(2).
		Render("Enter your secp256k1 private key in hex format")
	
	warning := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true).
		MarginBottom(2).
		Render("⚠️  Your key will be hidden after this step")
	
	inputBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Render(m.input.View())
	
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		MarginTop(2).
		Render("Press Enter to continue")
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		description,
		warning,
		inputBox,
		hint,
	)
	
	return boxStyle.Render(content)
}

func (m PKModel) Value() string { return strings.TrimSpace(m.input.Value()) }

// ---------- Params Form ----------

type ParamsModel struct {
	fields       []textinput.Model
	labels       []string
	readOnly     []bool
	idx          int
	done         bool
	errorMessage string
}

func NewParamsModel() ParamsModel { return ParamsModel{} }

func (m *ParamsModel) ResetFor(txType string, state SharedState) {
	// Define form per tx type
	switch txType {
	case "bank/send":
		m.setFields([]string{"from_addr", "to_addr", "amount", "node(rpc)", "chain_id"})
	case "eth/transfer":
		m.setFields([]string{"from", "to", "value(wei)", "gas", "gas_price(wei)", "nonce(optional)", "rpc_url"})
	default:
		m.setFields([]string{"param1", "param2"})
	}
	
	// Auto-fill parameters
	cachedRPC := GetCachedRPC(state.Chain)
	autoFillInput := AutoFillInput{
		TxType: txType,
		PKHex:  state.PK,
		RPCURL: cachedRPC,
	}
	
	autoFilled, err := AutoFillParams(autoFillInput)
	if err != nil {
		m.errorMessage = "Auto-fill error: " + err.Error()
	} else {
		// Apply auto-filled values
		for i, label := range m.labels {
			if val, ok := autoFilled[label]; ok {
				m.fields[i].SetValue(val)
			}
		}
	}
	
	// Find first editable field and focus it
	for i := range m.fields {
		if !m.readOnly[i] {
			m.fields[i].Focus()
			m.idx = i
			break
		}
	}
	
	m.done = false
}

func (m *ParamsModel) setFields(labels []string) {
	m.labels = labels
	m.fields = make([]textinput.Model, len(labels))
	m.readOnly = make([]bool, len(labels))
	
	for i, lab := range labels {
		ti := textinput.New()
		ti.Placeholder = lab
		m.fields[i] = ti
		m.readOnly[i] = shouldMakeFieldReadOnly(lab)
	}
	m.idx = 0
}

func (m ParamsModel) Values() map[string]string {
	vals := map[string]string{}
	for i, lab := range m.labels {
		vals[lab] = m.fields[i].Value()
	}
	return vals
}

func (m ParamsModel) Update(msg tea.Msg) (ParamsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		s := msg.String()
		if s == keyNext {
			// Find next editable field
			nextIdx := m.findNextEditableField(m.idx)
			if nextIdx == -1 {
				// No more editable fields, we're done
				m.done = true
			} else if nextIdx != m.idx {
				// Move to next editable field
				m.fields[m.idx].Blur()
				m.idx = nextIdx
				m.fields[m.idx].Focus()
			}
			return m, nil
		}
	}
	
	// Only update the current focused field if it's editable
	var cmds []tea.Cmd
	if !m.readOnly[m.idx] {
		var cmd tea.Cmd
		m.fields[m.idx], cmd = m.fields[m.idx].Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m ParamsModel) findNextEditableField(currentIdx int) int {
	for i := currentIdx + 1; i < len(m.fields); i++ {
		if !m.readOnly[i] {
			return i
		}
	}
	return -1
}

func (m ParamsModel) View() string {
	title := titleStyle.Render("📝 Transaction Parameters")
	
	var fields []string
	
	if m.errorMessage != "" {
		errorBox := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("196")).
			Padding(0, 1).
			MarginBottom(2).
			Render(errStyle.Render("⚠️ " + m.errorMessage))
		fields = append(fields, errorBox)
	}
	
	for i, f := range m.fields {
		lab := m.labels[i]
		var fieldView string
		
		if m.readOnly[i] {
			val := f.Value()
			if val == "" {
				val = "<auto-generated>"
			}
			
			readOnlyBox := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("238")).
				Padding(0, 1).
				MarginBottom(1).
				Render(
					lipgloss.JoinVertical(lipgloss.Left,
						labelStyle.Render(lab+" 🔒"),
						highlightStyle.Render(val),
					),
				)
			fieldView = readOnlyBox
		} else {
			isActive := i == m.idx
			borderColor := "240"
			if isActive {
				borderColor = "86"
			}
			
			editableBox := lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color(borderColor)).
				Padding(0, 1).
				MarginBottom(1).
				Render(
					lipgloss.JoinVertical(lipgloss.Left,
						labelStyle.Render(lab),
						f.View(),
					),
				)
			fieldView = editableBox
		}
		
		fields = append(fields, fieldView)
	}
	
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		MarginTop(2).
		Render("Press Enter to advance • Final Enter to confirm")
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{title}, append(fields, hint)...)...,
	)
	
	return boxStyle.Render(content)
}

// ---------- Confirm ----------

type ConfirmModel struct {
	summary   string
	confirmed bool
}

func NewConfirmModel() ConfirmModel { return ConfirmModel{} }

func (m *ConfirmModel) SetSummary(s SharedState) {
	lines := []string{
		"Chain: " + s.Chain,
		"Tx: " + s.TxType,
		"PK: <hidden>",
	}
	for k, v := range s.Inputs {
		lines = append(lines, fmt.Sprintf("%s: %s", k, v))
	}
	m.summary = strings.Join(lines, "\n")
}

func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == keyNext {
			m.confirmed = true
		}
	}
	return m, nil
}

func (m ConfirmModel) View() string {
	title := titleStyle.Render("✅ Confirm Transaction")
	
	summaryBox := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("86")).
		Padding(1, 2).
		MarginTop(2).
		MarginBottom(2).
		Render(m.summary)
	
	warning := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Bold(true).
		MarginBottom(2).
		Render("⚠️  This action cannot be undone")
	
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true).
		Render("Press Enter to send transaction")
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		summaryBox,
		warning,
		hint,
	)
	
	return boxStyle.Render(content)
}

// ---------- Progress ----------

type StartSendMsg struct{}

type SendResultMsg struct {
	Hash string
	Err  error
}

type ProgressModel struct{ bar progress.Model }

func NewProgressModel() ProgressModel {
	p := progress.New(progress.WithDefaultGradient())
	p.Width = 40
	return ProgressModel{bar: p}
}

func (m ProgressModel) Update(msg tea.Msg) (ProgressModel, tea.Cmd) {
	var cmd tea.Cmd
	var progressModel tea.Model
	progressModel, cmd = m.bar.Update(msg)
	m.bar = progressModel.(progress.Model)
	return m, cmd
}

func (m ProgressModel) View() string {
	title := titleStyle.Render("⏳ Sending Transaction...")
	
	spinner := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true).
		MarginTop(2).
		MarginBottom(2).
		Render("Broadcasting to network...")
	
	progressBar := lipgloss.NewStyle().
		Padding(1, 0).
		Render(m.bar.ViewAs(0.6))
	
	content := lipgloss.JoinVertical(lipgloss.Center,
		title,
		spinner,
		progressBar,
	)
	
	return boxStyle.Render(content)
}

// ---------- Result ----------

type ResultModel struct {
	hash string
	err  error
}

func NewResultModel() ResultModel { return ResultModel{} }

func (m *ResultModel) SetResult(hash string, err error) { m.hash, m.err = hash, err }

func (m ResultModel) Update(msg tea.Msg) (ResultModel, tea.Cmd) { return m, nil }

func (m ResultModel) View() string {
	var title, icon, message, details string
	var messageStyle lipgloss.Style
	
	if m.err != nil {
		title = titleStyle.Render("❌ Transaction Failed")
		icon = "🚫"
		message = "Error occurred"
		details = m.err.Error()
		messageStyle = errStyle
	} else {
		title = titleStyle.Render("🎉 Transaction Successful!")
		icon = "✅"
		message = "Transaction Hash"
		details = m.hash
		messageStyle = okStyle
	}
	
	resultBox := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(func() lipgloss.Color {
			if m.err != nil {
				return lipgloss.Color("196")
			}
			return lipgloss.Color("42")
		}()).
		Padding(1, 2).
		MarginTop(2).
		MarginBottom(2).
		Render(
			lipgloss.JoinVertical(lipgloss.Center,
				lipgloss.NewStyle().
					Bold(true).
					Render(icon+" "+message),
				messageStyle.Render(details),
			),
		)
	
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Render(fmt.Sprintf("Press %s to send another transaction", keyHome))
	
	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		resultBox,
		hint,
	)
	
	return boxStyle.Render(content)
}

// ---------- Helpers ----------

func HelpBar() string {
	logo := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("235")).
		Padding(0, 1).
		Render("🚀 TxPilot")
	
	keys := []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("Ctrl+C") + " Quit",
		lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("Ctrl+X") + " Home",
		lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("Esc") + " Back",
		lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("Enter") + " Next",
	}
	
	helpText := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render(strings.Join(keys, " • "))
	
	bar := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Padding(0, 2).
		Width(80).
		Render(logo + lipgloss.PlaceHorizontal(60, lipgloss.Right, helpText))
	
	return bar
}
