package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type page int

const (
	pageWelcome page = iota
	pageSelectChain
	pageSelectTx
	pageEnterPK
	pageEnterParams
	pageConfirm
	pageProgress
	pageResult
)

const (
	keyQuit = "ctrl+c"
	keyHome = "ctrl+x"
	keyBack = "esc"
	keyNext = "enter"
)

type App struct {
	page  page
	stack []page

	welcome  WelcomeModel
	chains   MenuModel
	txs      MenuModel
	pk       PKModel
	params   ParamsModel
	confirm  ConfirmModel
	progress ProgressModel
	result   ResultModel

	state SharedState
}

type SharedState struct {
	Chain  string
	TxType string
	PK     string
	Inputs map[string]string
	RPC    string

	TxHash string
	Err    error
}

func NewApp() App {
	m := App{
		page:     pageWelcome,
		stack:    []page{},
		state:    SharedState{Inputs: map[string]string{}},
		welcome:  NewWelcome(),
		chains:   NewMenu("Select chain", []string{"cosmos", "evm"}),
		txs:      NewMenu("Select tx type", nil),
		pk:       NewPKModel(),
		params:   NewParamsModel(),
		confirm:  NewConfirmModel(),
		progress: NewProgressModel(),
		result:   NewResultModel(),
	}
	return m
}

func (m *App) push(p page) {
	m.stack = append(m.stack, m.page)
	m.page = p
}

func (m *App) back() {
	if len(m.stack) > 0 {
		m.page = m.stack[len(m.stack)-1]
		m.stack = m.stack[:len(m.stack)-1]
	}
}

func (m *App) home() {
	*m = NewApp()
}

func (m App) Init() tea.Cmd { return nil }

func (m App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case keyQuit:
			return m, tea.Quit
		case keyHome:
			mm := m
			mm.home()
			return mm, nil
		case keyBack:
			mm := m
			mm.back()
			return mm, nil
		}
	}

	switch m.page {
	case pageWelcome:
		var cmd tea.Cmd
		m.welcome, cmd = m.welcome.Update(msg)
		if m.welcome.done {
			m.push(pageSelectChain)
		}
		return m, cmd

	case pageSelectChain:
		var cmd tea.Cmd
		m.chains, cmd = m.chains.Update(msg)
		// Check if Enter was pressed
		if kMsg, ok := msg.(tea.KeyMsg); ok && kMsg.String() == keyNext {
			if selected := m.chains.Selected(); selected != "" {
				m.state.Chain = selected
				if m.state.Chain == "cosmos" {
					m.txs.SetItems([]string{"bank/send"})
				} else {
					m.txs.SetItems([]string{"eth/transfer"})
				}
				m.push(pageSelectTx)
			}
		}
		return m, cmd

	case pageSelectTx:
		var cmd tea.Cmd
		m.txs, cmd = m.txs.Update(msg)
		// Check if Enter was pressed
		if kMsg, ok := msg.(tea.KeyMsg); ok && kMsg.String() == keyNext {
			if sel := m.txs.Selected(); sel != "" {
				m.state.TxType = sel
				m.push(pageEnterPK)
			}
		}
		return m, cmd

	case pageEnterPK:
		var cmd tea.Cmd
		m.pk, cmd = m.pk.Update(msg)
		if m.pk.done {
			m.state.PK = m.pk.Value()
			m.params.ResetFor(m.state.TxType, m.state)
			m.push(pageEnterParams)
		}
		return m, cmd

	case pageEnterParams:
		var cmd tea.Cmd
		m.params, cmd = m.params.Update(msg)
		if m.params.done {
			m.state.Inputs = m.params.Values()
			if rpc, ok := m.state.Inputs["node(rpc)"]; ok {
				m.state.RPC = rpc
				CacheRPC(m.state.Chain, rpc)
			} else if rpc, ok := m.state.Inputs["rpc_url"]; ok {
				m.state.RPC = rpc
				CacheRPC(m.state.Chain, rpc)
			}
			m.confirm.SetSummary(m.state)
			m.push(pageConfirm)
		}
		return m, cmd

	case pageConfirm:
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		if m.confirm.confirmed {
			m.push(pageProgress)
			return m, tea.Sequence(
				tea.Tick(0, func(t time.Time) tea.Msg { return StartSendMsg{} }),
			)
		}
		return m, cmd

	case pageProgress:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		switch msg.(type) {
		case StartSendMsg:
			return m, m.sendTxCmd()
		case SendResultMsg:
			res := msg.(SendResultMsg)
			m.state.TxHash = res.Hash
			m.state.Err = res.Err
			m.result.SetResult(m.state.TxHash, m.state.Err)
			m.push(pageResult)
			return m, nil
		}
		return m, cmd

	case pageResult:
		var cmd tea.Cmd
		m.result, cmd = m.result.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m App) View() string {
	header := HelpBar()
	var body string
	switch m.page {
	case pageWelcome:
		body = m.welcome.View()
	case pageSelectChain:
		body = m.chains.View()
	case pageSelectTx:
		body = m.txs.View()
	case pageEnterPK:
		body = m.pk.View()
	case pageEnterParams:
		body = m.params.View()
	case pageConfirm:
		body = m.confirm.View()
	case pageProgress:
		body = m.progress.View()
	case pageResult:
		body = m.result.View()
	}
	return header + "\n" + body
}

func (m App) sendTxCmd() tea.Cmd {
	state := m.state
	return func() tea.Msg {
		hash, err := txSend(state)
		return SendResultMsg{Hash: hash, Err: err}
	}
}
