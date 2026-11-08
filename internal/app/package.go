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
	Repo            string    `json:"repo,omitempty"`
	FromBase        bool      `json:"from_base"`
	DevNoise        bool      `json:"dev_noise"`
	SystemComponent bool      `json:"system_component"`
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