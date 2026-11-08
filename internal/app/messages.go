package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type scanDoneMsg struct {
	pkgs      []Package
	fp        string
	baseFound bool
	withRepos bool
	cached    bool
	err       error
}

type repoDoneMsg struct {
	pkgs  []Package
	repos map[string]string
	err   error
}

type spinnerMsg struct{}
type blinkMsg struct{}
type flashMsg struct{}
type copyDoneMsg struct {
	name string
	err  error
}
type clearNoticeMsg string

type pane int

const (
	paneList pane = iota
	paneDetails
)

type dragMode int

const (
	dragNone dragMode = iota
	dragSplitWidth
	dragStackHeight
	dragOverviewHeight
)

func spinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return spinnerMsg{} })
}

func blinkTick() tea.Cmd {
	return tea.Tick(530*time.Millisecond, func(time.Time) tea.Msg { return blinkMsg{} })
}

func flashTick() tea.Cmd {
	return tea.Tick(140*time.Millisecond, func(time.Time) tea.Msg { return flashMsg{} })
}