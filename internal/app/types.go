package app

import "time"

// Package is one user-installed rpm reduced to what DNFscope needs.
type Package struct {
	Name            string    `json:"name"`
	Arch            string    `json:"arch"`
	EVR             string    `json:"evr"`
	Summary         string    `json:"summary"`
	URL             string    `json:"url"`
	Size            int64     `json:"size"`
	InstallTime     time.Time `json:"install_time"`
	Reason          string    `json:"reason"`
	Commands        []string  `json:"commands,omitempty"`
	HasDesktop      bool      `json:"has_desktop"`
	FromBase        bool      `json:"from_base"`
	DevNoise        bool      `json:"dev_noise"`
	SystemComponent bool      `json:"system_component"`
	TUI             bool      `json:"tui"`
}

func (p Package) KindLabel() string {
	switch {
	case p.HasDesktop:
		return "app"
	case len(p.Commands) > 0:
		return "cli"
	default:
		return "pkg"
	}
}

// Filters hold the toggles exposed with a/b/d.
type Filters struct {
	AllUser     bool // show every user package, including fonts/themes/libs
	IncludeBase bool // show packages from the base image transactions
	ShowDev     bool // show -devel/-doc/-static noise
	ShowTUI     bool // show terminal UI apps (tui-tools family)
}

type SortMode int

const (
	SortDate SortMode = iota
	SortName
	SortSize
	SortType
)

func (s SortMode) String() string {
	switch s {
	case SortName:
		return "name"
	case SortSize:
		return "size"
	case SortType:
		return "type"
	default:
		return "date"
	}
}

func (s SortMode) next() SortMode {
	return (s + 1) % 4
}

// KindFilter is the type tab: all, apps, cli tools or everything else. The
// values double as indexes into model.counts.
type KindFilter int

const (
	KindAll KindFilter = iota
	KindApp
	KindCLI
	KindPkg
)

func (k KindFilter) next(delta int) KindFilter {
	return KindFilter((int(k) + delta + 4) % 4)
}

func (k KindFilter) label() string {
	switch k {
	case KindApp:
		return "app"
	case KindCLI:
		return "cli"
	case KindPkg:
		return "pkg"
	default:
		return "all"
	}
}

func kindIndex(p Package) KindFilter {
	switch p.KindLabel() {
	case "app":
		return KindApp
	case "cli":
		return KindCLI
	default:
		return KindPkg
	}
}
