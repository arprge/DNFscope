package app

import (
	"context"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	all     []Package
	visible []Package

	width, height int
	lay           layout
	focused       pane
	dragging      bool
	splitW        int
	stackDetailH  int
	icons         bool
	repoSource    bool
	repoLoading   bool

	cursor int
	offset int

	query    string
	filters  Filters
	sortMode SortMode
	sortDesc bool
	kind     KindFilter
	counts   [kindCount]int // per type, before the type tab is applied

	searching bool
	help      bool
	loading   bool
	baseFound bool
	fp        string
	err       error

	spin    int
	caretOn bool
	flash   bool
	notice  string

	showDeps    bool
	depsLoading string
	depsOffset  int
	deps        map[string]dependencyResult
}

// baseModel builds a model with no cache access; tests use it directly.
func baseModel(icons bool) model {
	m := model{
		icons:    icons,
		sortMode: SortDate,
		sortDesc: true,
		loading:  true,
		deps:     make(map[string]dependencyResult),
	}
	m.resize(100, 30)
	return m
}

func newModel(icons bool) model {
	m := baseModel(icons)

	// Render whatever is cached immediately; freshness is checked in Init.
	if cf, err := readCache(); err == nil && cf.Fingerprint != "" {
		m.all = cf.Packages
		m.fp = cf.Fingerprint
		m.baseFound = cf.BaseFound
		m.loading = false
	}
	m.rebuildRows()
	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{refreshCmd(m.fp, len(m.all) > 0, m.repoSource)}
	if m.loading {
		cmds = append(cmds, spinnerTick())
	}
	return tea.Batch(cmds...)
}

// refreshCmd validates the cache fingerprint and re-scans only when needed.
func refreshCmd(oldFP string, hasData, includeRepos bool) tea.Cmd {
	return func() tea.Msg {
		fp, err := fingerprint()
		if err != nil {
			return scanDoneMsg{err: err}
		}
		if hasData && fp == oldFP {
			return scanDoneMsg{fp: fp, cached: true}
		}

		pkgs, baseFound, err := buildPackages(context.Background(), includeRepos)
		if err != nil {
			return scanDoneMsg{err: err, fp: fp}
		}
		_ = writeCache(&cacheFile{
			Schema:      cacheSchema,
			Fingerprint: fp,
			BaseFound:   baseFound,
			Packages:    pkgs,
		})
		return scanDoneMsg{pkgs: pkgs, fp: fp, baseFound: baseFound}
	}
}

func loadRepoSources(pkgs []Package) tea.Cmd {
	return func() tea.Msg {
		repos, err := loadRepos(context.Background())
		return repoDoneMsg{pkgs: pkgs, repos: repos, err: err}
	}
}

func (m *model) resize(width, height int) {
	m.width, m.height = width, height
	m.lay = computeLayout(width, height, m.icons, m.splitW, m.stackDetailH)
	if m.lay.mode == modeSplit {
		m.splitW = m.lay.detW
	} else if m.lay.mode == modeStack {
		m.stackDetailH = m.lay.detH
	}
	m.clampScroll()
}

func (m *model) rebuildRows() {
	q := strings.ToLower(strings.TrimSpace(m.query))
	m.visible = m.visible[:0]
	m.counts = [kindCount]int{}
	for _, p := range m.all {
		if !m.filters.IncludeBase && p.FromBase {
			continue
		}
		if !m.filters.ShowDev && p.DevNoise {
			continue
		}

		if !m.filters.AllUser && p.SystemComponent {
			continue
		}
		if !m.filters.AllUser && !p.HasDesktop && len(p.Commands) == 0 {
			continue
		}
		if q != "" && !matches(p, q) {
			continue
		}
		k := kindIndex(p)
		m.counts[KindAll]++
		m.counts[k]++
		if m.kind != KindAll && m.kind != k {
			continue
		}
		m.visible = append(m.visible, p)
	}

	sort.SliceStable(m.visible, func(i, j int) bool {
		return m.less(m.visible[i], m.visible[j])
	})

	m.cursor = 0
	m.offset = 0
	m.clampScroll()
}

func (m *model) clampScroll() {
	n := len(m.visible)
	if n == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}

	body := m.lay.bodyH
	if body < 1 {
		body = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+body {
		m.offset = m.cursor - body + 1
	}

	maxOffset := n - body
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *model) move(delta int) {
	m.cursor += delta
	m.clampScroll()
}

func (m *model) moveDependencies(delta int) {
	p, ok := m.selected()
	if !ok {
		return
	}
	result, ok := m.deps[packageKey(p)]
	if !ok {
		return
	}
	page := max(1, m.lay.detH-2)
	maxOffset := max(0, len(result.lines)-page)
	m.depsOffset = clamp(m.depsOffset+delta, 0, maxOffset)
}

func (m *model) jumpDependencies(end bool) {
	if !end {
		m.depsOffset = 0
		return
	}
	p, ok := m.selected()
	if !ok {
		return
	}
	result, ok := m.deps[packageKey(p)]
	if ok {
		m.depsOffset = max(0, len(result.lines)-max(1, m.lay.detH-2))
	}
}

func (m *model) jump(index int) {
	m.cursor = index
	m.clampScroll()
}

func (m model) selected() (Package, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return Package{}, false
	}
	return m.visible[m.cursor], true
}

func (m model) less(a, b Package) bool {
	var c int
	switch m.sortMode {
	case SortName:
		c = strings.Compare(a.Name, b.Name)
	case SortSize:
		switch {
		case a.Size < b.Size:
			c = -1
		case a.Size > b.Size:
			c = 1
		}
	case SortType:
		c = strings.Compare(a.KindLabel(), b.KindLabel())
		if c == 0 {
			c = strings.Compare(a.Name, b.Name)
		}
	default:
		c = a.InstallTime.Compare(b.InstallTime)
	}
	if m.sortDesc {
		return c > 0
	}
	return c < 0
}

func (m *model) cycleSort() {
	m.sortMode = m.sortMode.next()
	m.sortDesc = m.sortMode == SortDate
}

func matches(p Package, q string) bool {
	if strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.Summary), q) {
		return true
	}
	for _, c := range p.Commands {
		if strings.Contains(strings.ToLower(c), q) {
			return true
		}
	}
	return false
}
