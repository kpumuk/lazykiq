package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kpumuk/lazykiq/internal/ui"
)

func main() {
	app := ui.New()
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running lazykiq: %v\n", err)
		os.Exit(1)
	}
}
