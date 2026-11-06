package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type scanDoneMsg struct {
	pkgs      []Package
	fp        string
	baseFound bool
	cached    bool
	err       error
}

type spinnerMsg struct{}
type blinkMsg struct{}
type flashMsg struct{}

func spinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return spinnerMsg{} })
}

func blinkTick() tea.Cmd {
	return tea.Tick(530*time.Millisecond, func(time.Time) tea.Msg { return blinkMsg{} })
}

func flashTick() tea.Cmd {
	return tea.Tick(140*time.Millisecond, func(time.Time) tea.Msg { return flashMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case spinnerMsg:
		if !m.loading {
			return m, nil
		}
		m.spin = (m.spin + 1) % len(spinnerFrames)
		return m, spinnerTick()

	case blinkMsg:
		if !m.searching {
			m.caretOn = false
			return m, nil
		}
		m.caretOn = !m.caretOn
		return m, blinkTick()

	case flashMsg:
		m.flash = false
		return m, nil

	case scanDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.cached {
			return m, nil
		}
		m.err = nil
		m.all = msg.pkgs
		m.fp = msg.fp
		m.baseFound = msg.baseFound
		m.rebuildRows()
		m.flash = true
		return m, flashTick()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.help {
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		default:
			m.help = false
			return m, nil
		}
	}

	if m.searching {
		return m.updateSearch(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "ctrl+u":
		m.move(-max(1, m.lay.bodyH/2))
	case "ctrl+d":
		m.move(max(1, m.lay.bodyH/2))
	case "pgup", "ctrl+b":
		m.move(-max(1, m.lay.bodyH))
	case "pgdown", "ctrl+f":
		m.move(max(1, m.lay.bodyH))
	case "home", "g":
		m.jump(0)
	case "end", "G":
		m.jump(len(m.visible) - 1)

	case "tab", "right", "l":
		m.kind = m.kind.next(1)
		m.rebuildRows()
	case "shift+tab", "left", "h":
		m.kind = m.kind.next(-1)
		m.rebuildRows()
	case "1", "2", "3", "4":
		m.kind = KindFilter(msg.String()[0] - '1')
		m.rebuildRows()

	case "/":
		m.searching = true
		m.caretOn = true
		m.query = ""
		m.rebuildRows()
		return m, blinkTick()

	case "esc":
		if m.query != "" {
			m.query = ""
			m.rebuildRows()
		}

	case "s":
		m.cycleSort()
		m.rebuildRows()
	case "S":
		m.sortDesc = !m.sortDesc
		m.rebuildRows()

	case "a":
		m.filters.AllUser = !m.filters.AllUser
		m.rebuildRows()
	case "b":
		m.filters.IncludeBase = !m.filters.IncludeBase
		m.rebuildRows()
	case "d":
		m.filters.ShowDev = !m.filters.ShowDev
		m.rebuildRows()

	case "i":
		m.icons = !m.icons
		m.resize(m.width, m.height)

	case "?":
		m.help = true
	}
	return m, nil
}

func (m model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.searching = false
		m.caretOn = false
		m.query = ""
		m.rebuildRows()
		return m, nil
	case "enter":
		m.searching = false
		m.caretOn = false
		return m, nil
	case "backspace", "ctrl+h":
		q := []rune(m.query)
		if len(q) > 0 {
			m.query = string(q[:len(q)-1])
			m.rebuildRows()
		}
		return m, nil
	case "ctrl+u":
		m.query = ""
		m.rebuildRows()
		return m, nil
	}

	switch msg.Type {
	case tea.KeyRunes:
		m.query += string(msg.Runes)
	case tea.KeySpace:
		m.query += " "
	default:
		return m, nil
	}
	m.rebuildRows()
	return m, nil
}
