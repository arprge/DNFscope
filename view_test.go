package main

import (
	"errors"
	"strings"
	"testing"
	"time"

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
