package main

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zsystm/txpilot/internal/ui"
)

func main() {
	// Use the simple working version
	p := tea.NewProgram(ui.NewSimpleApp(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Println("TxPilot crashed:", err)
		os.Exit(1)
	}
}
