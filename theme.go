package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// palette is one colour scheme. Surfaces step from crust (deepest) through
// mantle (page) and base (panels) to surface2 (most raised), which gives the
// layout its depth without gradients. accent marks focus; app, cli and pkg
// colour the type badges so each scheme keeps its own type colours.
type palette struct {
	name                                lipgloss.Color
	crust, mantle, base                 lipgloss.Color
	surface0, surface1, surface2        lipgloss.Color
	overlay0, overlay1                  lipgloss.Color
	subtext0, subtext1, text            lipgloss.Color
	accent, sapphire, green, peach, red lipgloss.Color
	app, cli, pkg                       lipgloss.Color
}

var gruvbox = palette{
	name:     "gruvbox",
	crust:    "#141617",
	mantle:   "#1d2021",
	base:     "#282828",
	surface0: "#3c3836",
	surface1: "#504945",
	surface2: "#665c54",
	overlay0: "#7c6f64",
	overlay1: "#928374",
	subtext0: "#a89984",
	subtext1: "#bdae93",
	text:     "#ebdbb2",
	accent:   "#fabd2f",
	sapphire: "#83a598",
	green:    "#b8bb26",
	peach:    "#fe8019",
	red:      "#fb4934",
	app:      "#d3869b",
	cli:      "#8ec07c",
	pkg:      "#a89984",
}

var monokai = palette{
	name:     "monokai",
	crust:    "#141510",
	mantle:   "#1d1e19",
	base:     "#272822",
	surface0: "#3e3d32",
	surface1: "#49483e",
	surface2: "#5b5a4d",
	overlay0: "#75715e",
	overlay1: "#90908a",
	subtext0: "#a8a89f",
	subtext1: "#cfcfc2",
	text:     "#f8f8f2",
	accent:   "#66d9ef",
	sapphire: "#66d9ef",
	green:    "#a6e22e",
	peach:    "#fd971f",
	red:      "#f92672",
	app:      "#ae81ff",
	cli:      "#a6e22e",
	pkg:      "#a8a89f",
}

var mocha = palette{
	name:     "mocha",
	crust:    "#11111b",
	mantle:   "#181825",
	base:     "#1e1e2e",
	surface0: "#313244",
	surface1: "#45475a",
	surface2: "#585b70",
	overlay0: "#6c7086",
	overlay1: "#7f849c",
	subtext0: "#a6adc8",
	subtext1: "#bac2de",
	text:     "#cdd6f4",
	accent:   "#b4befe",
	sapphire: "#74c7ec",
	green:    "#a6e3a1",
	peach:    "#fab387",
	red:      "#f38ba8",
	app:      "#cba6f7",
	cli:      "#94e2d5",
	pkg:      "#7f849c",
}

// flavors are the selectable colour schemes. The first one is the default.
var flavors = []palette{gruvbox, monokai, mocha}

var (
	theme    = flavors[0]
	themeIdx = 0
)

func setTheme(i int) {
	n := len(flavors)
	themeIdx = ((i % n) + n) % n
	theme = flavors[themeIdx]
}

func themeIndex(name string) int {
	for i, f := range flavors {
		if f.name == name {
			return i
		}
	}
	return -1
}

func themeNames() string {
	names := make([]string, len(flavors))
	for i, f := range flavors {
		names[i] = string(f.name)
	}
	return strings.Join(names, ", ")
}

// Nerd Font glyphs, all in the private use area and single width in patched
// fonts. Toggled at runtime with i.
const (
	iconApp    = "\uf2d0" // window-maximize
	iconCLI    = "\uf120" // terminal
	iconPkg    = "\uf187" // archive
	iconSearch = "\uf002" // search
	iconDate   = "\uf133" // calendar
	iconSize   = "\uf0a0" // hdd
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func paint(fg, bg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(fg).Background(bg)
}

func canvas(bg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Background(bg)
}

func kindColor(kind string) lipgloss.Color {
	switch kind {
	case "app":
		return theme.app
	case "cli":
		return theme.cli
	default:
		return theme.pkg
	}
}

func kindIcon(kind string) string {
	switch kind {
	case "app":
		return iconApp
	case "cli":
		return iconCLI
	default:
		return iconPkg
	}
}
