package app

import tea "github.com/charmbracelet/bubbletea"

func RunTUI(icons bool) error {
	_, err := tea.NewProgram(newModel(icons), tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
