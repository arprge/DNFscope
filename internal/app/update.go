package app

import (
	"errors"
	"os/exec"
	"strings"
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

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

	case copyDoneMsg:
		if msg.err != nil {
			m.notice = "copy failed: " + msg.err.Error()
		} else {
			m.notice = "copied " + msg.name
		}
		notice := m.notice
		return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearNoticeMsg(notice) })

	case clearNoticeMsg:
		if m.notice == string(msg) {
			m.notice = ""
		}
		return m, nil

	case depsDoneMsg:
		if m.depsLoading == msg.key {
			m.depsLoading = ""
		}
		m.deps[msg.key] = msg.result
		return m, nil

	case scanDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.cached {
			// The cached list may predate the origin lookup, so start one now
			// instead of pretending every package has an unknown origin.
			if m.repoSource && !msg.withRepos {
				m.repoLoading = true
				m.repoLoaded = false
				m.repoFailed = false
				return m, loadRepoSources(m.all)
			}
			return m, nil
		}
		m.err = nil
		m.all = msg.pkgs
		m.fp = msg.fp
		m.baseFound = msg.baseFound
		m.repoLoaded = msg.withRepos
		m.repoLoading = false
		m.repoFailed = false
		m.rebuildRows()
		m.flash = true
		cmds := []tea.Cmd{flashTick()}
		if m.repoSource && !msg.withRepos {
			// The scan ran before origins were requested; fetch them now.
			m.repoLoading = true
			cmds = append(cmds, loadRepoSources(m.all))
		}
		return m, tea.Batch(cmds...)

	case repoDoneMsg:
		m.repoLoading = false
		if !m.repoSource {
			return m, nil
		}
		if msg.err != nil {
			m.repoLoaded = false
			m.repoFailed = true
			m.notice = "repo lookup failed"
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearNoticeMsg(m.notice) })
		}
		m.repoFailed = false
		m.repoLoaded = true
		for i := range msg.pkgs {
			p := &msg.pkgs[i]
			p.Repo = msg.repos[p.Name+"."+p.Arch+"|"+p.EVR]
		}
		m.all = msg.pkgs
		_ = writeCache(&cacheFile{Schema: cacheSchema, Fingerprint: m.fp, BaseFound: m.baseFound, Packages: m.all})
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) toggleDependencies(mode dependencyMode) (model, tea.Cmd) {
	p, ok := m.selected()
	if !ok {
		return m, nil
	}
	key := packageKey(p)
	if m.depsMode == mode {
		m.depsMode = dependencyNone
		m.depsOffset = 0
		return m, nil
	}
	m.depsMode = mode
	m.depsOffset = 0
	if _, cached := m.deps[key]; cached || m.depsLoading == key {
		return m, nil
	}
	m.depsLoading = key
	return m, loadDependencies(p)
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.help || m.searching {
		return m, nil
	}

	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 1
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -1
		}
		if m.depsMode != dependencyNone && m.focused == paneDetails {
			m.moveDependencies(delta)
		} else {
			m.move(delta)
		}
		return m, nil
	}

	if msg.Action == tea.MouseActionRelease {
		m.dragging = false
		m.dragMode = dragNone
		return m, nil
	}
	if msg.Action == tea.MouseActionMotion && m.dragging {
		switch m.dragMode {
		case dragSplitWidth:
			m.splitW = m.width - msg.X
		case dragStackHeight:
			m.stackDetailH = m.lay.avail - (msg.Y - m.contentStartY())
		case dragOverviewHeight:
			m.overviewH = m.lay.avail - (msg.Y - m.contentStartY())
		}
		m.resize(m.width, m.height)
		return m, nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	switch m.lay.mode {
	case modeSplit:
		boundaryY := m.contentStartY() + m.lay.detH
		if m.lay.ovH > 0 && msg.X >= m.lay.listW && abs(msg.Y-boundaryY) <= 1 {
			m.dragging = true
			m.dragMode = dragOverviewHeight
			return m, nil
		}
		if abs(msg.X-m.lay.listW) <= 1 {
			m.dragging = true
			m.dragMode = dragSplitWidth
			return m, nil
		}
		if msg.X < m.lay.listW {
			m.focused = paneList
		} else {
			m.focused = paneDetails
			return m.toggleDependencies(dependencyRequires)
		}
	case modeStack:
		boundaryY := m.contentStartY() + m.lay.listH
		if abs(msg.Y-boundaryY) <= 1 {
			m.dragging = true
			m.dragMode = dragStackHeight
			return m, nil
		}
		if msg.Y < boundaryY {
			m.focused = paneList
		} else {
			m.focused = paneDetails
			return m.toggleDependencies(dependencyRequires)
		}
	default:
		m.focused = paneList
	}
	return m, nil
}

func (m model) contentStartY() int {
	return 1 + boolInt(m.lay.searchRow) + boolInt(m.lay.tabsRow) + boolInt(m.lay.contentGap)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
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

	case "c":
		p, ok := m.selected()
		if !ok {
			m.notice = "nothing selected"
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearNoticeMsg(m.notice) })
		}
		return m, copyPackageName(p.Name)

	case "r":
		return m.toggleDependencies(dependencyRequires)

	case "R":
		return m.toggleDependencies(dependencyRequiredBy)

	case "up", "k":
		if m.depsMode != dependencyNone {
			m.moveDependencies(-1)
		} else {
			m.move(-1)
		}
	case "down", "j":
		if m.depsMode != dependencyNone {
			m.moveDependencies(1)
		} else {
			m.move(1)
		}
	case "ctrl+u":
		if m.depsMode != dependencyNone {
			m.moveDependencies(-max(1, m.lay.detH-2))
		} else {
			m.move(-max(1, m.lay.bodyH/2))
		}
	case "ctrl+d":
		if m.depsMode != dependencyNone {
			m.moveDependencies(max(1, m.lay.detH-2))
		} else {
			m.move(max(1, m.lay.bodyH/2))
		}
	case "pgup", "ctrl+b":
		if m.depsMode != dependencyNone {
			m.moveDependencies(-max(1, m.lay.detH-2))
		} else {
			m.move(-max(1, m.lay.bodyH))
		}
	case "pgdown", "ctrl+f":
		if m.depsMode != dependencyNone {
			m.moveDependencies(max(1, m.lay.detH-2))
		} else {
			m.move(max(1, m.lay.bodyH))
		}
	case "g":
		if m.depsMode != dependencyNone {
			m.jumpDependencies(false)
		} else {
			m.jump(0)
		}
	case "G":
		if m.depsMode != dependencyNone {
			m.jumpDependencies(true)
		} else {
			m.jump(len(m.visible) - 1)
		}

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

	case "o":
		m.repoSource = !m.repoSource
		m.repoFailed = false
		if !m.repoSource {
			m.repoLoading = false
			return m, nil
		}
		m.repoLoaded = false
		m.repoLoading = true
		if len(m.all) > 0 {
			return m, loadRepoSources(m.all)
		}
		// A scan is still running; scanDoneMsg starts the lookup afterwards.
		return m, nil

	case "i":
		m.icons = !m.icons
		m.resize(m.width, m.height)

	case "?":
		m.help = true
	}
	return m, nil
}

func copyPackageName(name string) tea.Cmd {
	return func() tea.Msg {
		for _, candidate := range []struct {
			binary string
			args   []string
		}{{"wl-copy", nil}, {"xclip", []string{"-selection", "clipboard"}}, {"xsel", []string{"--clipboard", "--input"}}, {"pbcopy", nil}} {
			path, err := exec.LookPath(candidate.binary)
			if err != nil {
				continue
			}
			cmd := exec.Command(path, candidate.args...)
			cmd.Stdin = strings.NewReader(name)
			if err := cmd.Run(); err == nil {
				return copyDoneMsg{name: name}
			}
		}
		return copyDoneMsg{name: name, err: errors.New("no working clipboard utility found (wl-copy, xclip, xsel, pbcopy)")}
	}
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
