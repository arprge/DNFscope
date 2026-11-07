package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func testPackages() []Package {
	base := time.Now().AddDate(0, -5, -3)
	names := []string{
		"code", "fzf", "eza", "fd-find", "lazygit", "starship", "kitty", "mpv",
		"zen-browser", "yazi", "zsh", "zoxide", "gum", "matugen", "ffmpeg",
		"steam", "wtype", "7zip-standalone", "aajohan-comfortaa-fonts", "dms",
	}
	pkgs := make([]Package, 0, len(names))
	for i, n := range names {
		pkg := Package{
			Name:        n,
			Arch:        "x86_64",
			EVR:         "1.0.0-1.fc45",
			Summary:     "A fairly long summary that should be truncated instead of wrapping in narrow terminals.",
			URL:         "https://example.test/" + n,
			Size:        int64(i+1) * 1024 * 1024,
			InstallTime: base.AddDate(0, 0, i),
			Reason:      "User",
		}
		switch i % 3 {
		case 0:
			pkg.HasDesktop = true
		case 1:
			pkg.Commands = []string{n, n + "-helper", n + "-ctl"}
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func viewAt(t *testing.T, icons bool, w, h int, mutate func(*model)) []string {
	t.Helper()
	m := baseModel(icons)
	m.loading = false
	m.all = testPackages()
	m.resize(w, h)
	m.rebuildRows()
	if mutate != nil {
		mutate(&m)
	}
	m.resize(w, h) // mutation may have changed icons
	return strings.Split(m.View(), "\n")
}

func TestViewFitsTerminal(t *testing.T) {
	sizes := [][2]int{
		{120, 30}, {100, 28}, {80, 24}, {64, 20}, {50, 14}, {40, 12}, {30, 8},
	}
	for _, icons := range []bool{true, false} {
		for _, sz := range sizes {
			w, h := sz[0], sz[1]
			lines := viewAt(t, icons, w, h, nil)
			if len(lines) != h {
				t.Fatalf("%dx%d icons=%v: got %d lines, want %d", w, h, icons, len(lines), h)
			}
			for i, ln := range lines {
				if got := ansi.StringWidth(ln); got != w {
					t.Fatalf("%dx%d icons=%v: line %d width=%d want %d\n%q", w, h, icons, i, got, w, ln)
				}
			}
		}
	}
}

func TestViewFitsTerminalStates(t *testing.T) {
	sizes := [][2]int{{120, 30}, {80, 24}, {50, 14}, {30, 8}}
	states := map[string]func(*model){
		"help":    func(m *model) { m.help = true },
		"search":  func(m *model) { m.searching = true; m.query = "zzz"; m.rebuildRows() },
		"empty":   func(m *model) { m.query = "no-such-package"; m.rebuildRows() },
		"loading": func(m *model) { m.loading = true },
		"flash":   func(m *model) { m.flash = true },
		"error":   func(m *model) { m.err = errors.New("boom") },
		"requirements": func(m *model) {
			m.depsMode = dependencyRequires
			p, _ := m.selected()
			m.deps[packageKey(p)] = dependencyResult{requires: []string{"glibc >= 2.38", "libxkbcommon.so.0()(64bit)"}, requiredBy: []string{"other-1.0-1.fc45.x86_64"}}
		},
	}
	for name, mutate := range states {
		for _, sz := range sizes {
			w, h := sz[0], sz[1]
			lines := viewAt(t, true, w, h, mutate)
			if len(lines) != h {
				t.Fatalf("%s %dx%d: got %d lines, want %d", name, w, h, len(lines), h)
			}
			for i, ln := range lines {
				if got := ansi.StringWidth(ln); got != w {
					t.Fatalf("%s %dx%d: line %d width=%d want %d\n%q", name, w, h, i, got, w, ln)
				}
			}
		}
	}
}

func TestDetailPanelShowsInstallRepository(t *testing.T) {
	for _, test := range []struct {
		name    string
		repo    string
		loading bool
		loaded  bool
		failed  bool
		want    string
	}{
		{name: "known", repo: "fedora", loaded: true, want: "fedora"},
		{name: "unknown", loaded: true, want: "unknown"},
		{name: "loading", loading: true, want: "loading…"},
		{name: "not loaded yet", want: "loading…"},
		{name: "lookup failed", failed: true, want: "lookup failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := baseModel(false)
			m.loading = false
			m.repoSource = true
			m.repoLoading = test.loading
			m.repoLoaded = test.loaded
			m.repoFailed = test.failed
			m.all = []Package{{Name: "example", Arch: "x86_64", EVR: "1.0", Repo: test.repo, Commands: []string{"example"}}}
			m.rebuildRows()

			lines := strings.Join(m.detailPanel(80, 20, false), "\n")
			plain := ansi.Strip(lines)
			if !strings.Contains(plain, "installed from") || !strings.Contains(plain, test.want) {
				t.Fatalf("detail panel does not show install source %q:\n%s", test.want, plain)
			}
			if test.want != "unknown" && strings.Contains(plain, "installed from") && strings.Contains(plain, "unknown") {
				t.Fatalf("detail panel falsely reports an unknown origin:\n%s", plain)
			}
		})
	}
}

func TestDetailPanelShowsDependencySections(t *testing.T) {
	for _, mode := range []dependencyMode{dependencyRequires, dependencyRequiredBy} {
		t.Run(mode.String(), func(t *testing.T) {
			m := baseModel(false)
			m.loading = false
			m.depsMode = mode
			m.all = []Package{{Name: "example", Arch: "x86_64", EVR: "1.0", Commands: []string{"example"}}}
			m.rebuildRows()
			m.deps["example.x86_64"] = dependencyResult{
				requires:   []string{"glibc >= 2.38"},
				requiredBy: []string{"other-1.0-1.fc45.x86_64"},
			}

			plain := ansi.Strip(strings.Join(m.detailPanel(80, 20, false), "\n"))
			want := []string{"Details", "requires", "glibc >= 2.38"}
			hidden := []string{"required by", "other-1.0-1.fc45.x86_64"}
			if mode == dependencyRequiredBy {
				want = []string{"Details", "required by", "other-1.0-1.fc45.x86_64"}
				hidden = []string{"requires", "glibc >= 2.38"}
			}
			for _, value := range want {
				if !strings.Contains(plain, value) {
					t.Fatalf("detail panel does not contain %q:\n%s", value, plain)
				}
			}
			for _, value := range hidden {
				if strings.Contains(plain, value) {
					t.Fatalf("detail panel unexpectedly contains inactive section %q:\n%s", value, plain)
				}
			}
		})
	}
}

func TestShiftRSwapsDependencySections(t *testing.T) {
	m := baseModel(false)
	m.loading = false
	m.all = []Package{{Name: "example", Arch: "x86_64", EVR: "1.0", Commands: []string{"example"}}}
	m.rebuildRows()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = next.(model)
	if m.depsMode != dependencyRequiredBy {
		t.Fatalf("shift+r mode=%v, want required by", m.depsMode)
	}

	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(model)
	if m.depsMode != dependencyRequires {
		t.Fatalf("r after required by mode=%v, want requires", m.depsMode)
	}

	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = next.(model)
	if m.depsMode != dependencyRequiredBy {
		t.Fatalf("shift+r after requires mode=%v, want required by", m.depsMode)
	}

	next, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	m = next.(model)
	if m.depsMode != dependencyNone {
		t.Fatalf("second shift+r mode=%v, want none", m.depsMode)
	}
}

func TestRepoOriginLoadsAfterPendingScan(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := baseModel(false)
	m.loading = true

	// Toggling origins before the initial scan finishes must be remembered.
	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = next.(model)
	if !m.repoSource || !m.repoLoading || m.repoLoaded {
		t.Fatalf("pending toggle: source=%v loading=%v loaded=%v", m.repoSource, m.repoLoading, m.repoLoaded)
	}

	pkgs := []Package{{Name: "fd-find", Arch: "x86_64", EVR: "9.1.0-1.fc45", Commands: []string{"fd"}}}
	next, cmd := m.Update(scanDoneMsg{pkgs: pkgs, fp: "fp"})
	m = next.(model)
	if cmd == nil {
		t.Fatal("scan completion should start the pending origin lookup")
	}
	if !m.repoLoading || m.repoLoaded {
		t.Fatalf("after scan: loading=%v loaded=%v", m.repoLoading, m.repoLoaded)
	}

	next, _ = m.Update(repoDoneMsg{pkgs: m.all, repos: map[string]string{"fd-find.x86_64|9.1.0-1.fc45": "updates"}})
	m = next.(model)
	if !m.repoLoaded || m.repoLoading || m.repoFailed {
		t.Fatalf("after lookup: loading=%v loaded=%v failed=%v", m.repoLoading, m.repoLoaded, m.repoFailed)
	}
	if got := m.all[0].Repo; got != "updates" {
		t.Fatalf("repo = %q, want updates", got)
	}
}

func TestScrollWindow(t *testing.T) {
	m := baseModel(true)
	m.loading = false
	m.all = testPackages()
	m.resize(100, 16)
	m.rebuildRows()

	m.jump(len(m.visible) - 1)
	if got := m.offset + m.lay.bodyH; got <= m.cursor {
		t.Fatalf("cursor %d outside window offset=%d body=%d", m.cursor, m.offset, m.lay.bodyH)
	}
	if m.offset != len(m.visible)-m.lay.bodyH {
		t.Fatalf("offset=%d want %d", m.offset, len(m.visible)-m.lay.bodyH)
	}

	m.jump(0)
	if m.offset != 0 {
		t.Fatalf("offset=%d after jump top", m.offset)
	}
}

func TestMouseFocusAndResize(t *testing.T) {
	m := baseModel(false)
	m.loading = false
	m.all = testPackages()
	m.rebuildRows()

	if m.lay.mode != modeSplit {
		t.Fatalf("initial mode=%v, want split", m.lay.mode)
	}
	applyMouse := func(msg tea.MouseMsg) {
		updated, _ := m.Update(msg)
		m = updated.(model)
	}
	oldDetailW := m.lay.detW
	applyMouse(tea.MouseMsg{X: m.lay.listW + 2, Y: m.contentStartY(), Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.focused != paneDetails {
		t.Fatal("clicking the detail pane did not focus it")
	}
	applyMouse(tea.MouseMsg{X: m.lay.listW, Y: m.contentStartY(), Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	applyMouse(tea.MouseMsg{X: m.lay.listW - 8, Y: m.contentStartY(), Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	applyMouse(tea.MouseMsg{X: m.lay.listW - 8, Y: m.contentStartY(), Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if m.lay.detW != oldDetailW+8 {
		t.Fatalf("dragged detail width=%d, want %d", m.lay.detW, oldDetailW+8)
	}
	oldDetailH := m.lay.detH
	oldOverviewH := m.lay.ovH
	boundaryY := m.contentStartY() + m.lay.detH
	applyMouse(tea.MouseMsg{X: m.lay.listW + 2, Y: boundaryY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	applyMouse(tea.MouseMsg{X: m.lay.listW + 2, Y: boundaryY + 3, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	applyMouse(tea.MouseMsg{X: m.lay.listW + 2, Y: boundaryY + 3, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if m.lay.detH != oldDetailH+3 || m.lay.ovH != oldOverviewH-3 {
		t.Fatalf("dragged detail/overview heights=%d/%d, want %d/%d", m.lay.detH, m.lay.ovH, oldDetailH+3, oldOverviewH-3)
	}

	m.resize(60, 20)
	if m.lay.mode != modeStack {
		t.Fatalf("stack test mode=%v, want stack", m.lay.mode)
	}
	oldStackDetailH := m.lay.detH
	stackBoundaryY := m.contentStartY() + m.lay.listH
	applyMouse(tea.MouseMsg{X: 10, Y: stackBoundaryY, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	applyMouse(tea.MouseMsg{X: 10, Y: stackBoundaryY - 2, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft})
	applyMouse(tea.MouseMsg{X: 10, Y: stackBoundaryY - 2, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if m.lay.detH != oldStackDetailH+2 {
		t.Fatalf("dragged detail height=%d, want %d", m.lay.detH, oldStackDetailH+2)
	}
}

func TestRelDate(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		days int
		want string
	}{
		{0, "today"}, {1, "1d ago"}, {4, "4d ago"}, {9, "9d ago"},
		{20, "2w ago"}, {40, "5w ago"}, {200, "6mo ago"}, {400, "1y ago"},
	}
	for _, c := range cases {
		got := relDate(now.AddDate(0, 0, -c.days), now)
		if got != c.want {
			t.Errorf("%d days: got %q want %q", c.days, got, c.want)
		}
	}
}

func TestParseDependencies(t *testing.T) {
	got := parseDependencies("glibc >= 2.38\nrpmlib(CompressedFileNames) <= 3.0.4-1\naardvark.so.1()(64bit)\nglibc >= 2.38\n")
	want := []string{"glibc >= 2.38", "aardvark.so.1()(64bit)"}
	if !slices.Equal(got, want) {
		t.Fatalf("parseDependencies() = %q, want %q", got, want)
	}
}

func TestParseRequiredBy(t *testing.T) {
	got := parseRequiredBy("no package requires example\nexample-1.0-1.fc45.x86_64\nconsumer-1.0-1.fc45.x86_64\nconsumer-1.0-1.fc45.x86_64\n", "example.x86_64")
	want := []string{"consumer-1.0-1.fc45.x86_64"}
	if !slices.Equal(got, want) {
		t.Fatalf("parseRequiredBy() = %q, want %q", got, want)
	}
}

func TestDependencyScroll(t *testing.T) {
	m := baseModel(false)
	m.loading = false
	m.all = []Package{{Name: "example", Arch: "x86_64", Commands: []string{"example"}}}
	m.rebuildRows()
	m.depsMode = dependencyRequires
	m.lay.detH = 4
	m.deps["example.x86_64"] = dependencyResult{requires: []string{"one", "two", "three", "four"}}

	m.moveDependencies(1)
	if m.depsOffset != 1 {
		t.Fatalf("dependencies offset=%d, want 1", m.depsOffset)
	}
	m.jumpDependencies(true)
	if m.depsOffset != 3 {
		t.Fatalf("end offset=%d, want 3", m.depsOffset)
	}
}

func TestCopyPackageName(t *testing.T) {
	dir := t.TempDir()
	clipboard := filepath.Join(dir, "clipboard")
	tool := filepath.Join(dir, "wl-copy")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n/bin/cat > \"$COPY_OUTPUT\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("COPY_OUTPUT", clipboard)

	msg := copyPackageName("example-package")()
	result, ok := msg.(copyDoneMsg)
	if !ok || result.err != nil {
		t.Fatalf("copyPackageName() = %#v", msg)
	}
	got, err := os.ReadFile(clipboard)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "example-package" {
		t.Fatalf("clipboard got %q", got)
	}
}

func TestKindTabs(t *testing.T) {
	m := baseModel(true)
	m.loading = false
	m.all = testPackages()
	m.rebuildRows()
	all := len(m.visible)
	if m.counts[KindAll] != all || m.counts[KindApp]+m.counts[KindCLI]+m.counts[KindPkg] != all {
		t.Fatalf("counts %v do not add up to %d", m.counts, all)
	}
	m.kind = KindApp
	m.rebuildRows()
	if len(m.visible) != m.counts[KindApp] || m.counts[KindAll] != all {
		t.Fatalf("app tab: visible=%d counts=%v", len(m.visible), m.counts)
	}
	for _, p := range m.visible {
		if p.KindLabel() != "app" {
			t.Fatalf("non-app %s in app tab", p.Name)
		}
	}
	if KindPkg.next(1) != KindAll || KindAll.next(-1) != KindPkg {
		t.Fatal("tab wrap broken")
	}
}
