package main

import tea "github.com/charmbracelet/bubbletea"

func runTUI(icons bool) error {
	_, err := tea.NewProgram(newModel(icons), tea.WithAltScreen()).Run()
	return err
}
