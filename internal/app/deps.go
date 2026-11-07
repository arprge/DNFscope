package app

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type dependencyMode int

const (
	dependencyNone dependencyMode = iota
	dependencyRequires
	dependencyRequiredBy
)

func (m dependencyMode) String() string {
	switch m {
	case dependencyRequires:
		return "requires"
	case dependencyRequiredBy:
		return "required by"
	default:
		return "none"
	}
}

type dependencyResult struct {
	requires   []string
	requiredBy []string
	err        error
}

func (r dependencyResult) lineCount(mode dependencyMode) int {
	switch mode {
	case dependencyRequires:
		return len(r.requires) + 1
	case dependencyRequiredBy:
		return len(r.requiredBy) + 1
	default:
		return 0
	}
}

type depsDoneMsg struct {
	key    string
	result dependencyResult
}

func packageKey(p Package) string {
	return p.Name + "." + p.Arch
}

// loadDependencies reads the selected package's requires and reverse
// dependencies. Querying rpm avoids dnf repoquery selecting an available,
// different version. Reverse dependencies cover direct name and architecture
// capability consumers, not every soname consumer.
func loadDependencies(p Package) tea.Cmd {
	return func() tea.Msg {
		key := packageKey(p)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, "rpm", "-q", "--requires", key).Output()
		if err != nil {
			return depsDoneMsg{key: key, result: dependencyResult{err: fmt.Errorf("rpm requirements: %w", err)}}
		}
		result := dependencyResult{requires: parseDependencies(string(out))}

		// Reverse dependencies are the installed packages that require this
		// package's name or architecture capability. rpm's whatrequires does
		// not match soname consumers through the package name, which is an
		// acceptable direct-dependency view and avoids fan-out queries.
		names := []string{p.Name}
		if p.Arch == "x86_64" || p.Arch == "i686" {
			names = append(names, p.Name+"(x86-64)")
		}
		reverse, _ := exec.CommandContext(ctx, "rpm", append([]string{"-q", "--whatrequires"}, names...)...).Output()
		result.requiredBy = parseRequiredBy(string(reverse), key)
		return depsDoneMsg{key: key, result: result}
	}
}

func parseRequiredBy(output, selfKey string) []string {
	seen := make(map[string]struct{})
	var dependents []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "no package requires ") {
			continue
		}
		key := nevraKey(line)
		if key == "" || key == selfKey {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		dependents = append(dependents, line)
	}
	sort.Strings(dependents)
	return dependents
}

func parseDependencies(output string) []string {
	seen := make(map[string]struct{})
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "rpmlib(") {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
	}
	sort.Slice(lines, func(i, j int) bool {
		ri, rj := dependencyRank(lines[i]), dependencyRank(lines[j])
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(lines[i]) < strings.ToLower(lines[j])
	})
	return lines
}

// Direct RPM requirements are more useful to people than implementation
// capabilities such as libfoo.so. Keep both, but present direct names first.
func dependencyRank(requirement string) int {
	requirement = strings.ToLower(requirement)
	switch {
	case strings.Contains(requirement, ".so"), strings.Contains(requirement, "()"):
		return 2
	case strings.HasPrefix(requirement, "/"), strings.HasPrefix(requirement, "pkgconfig("):
		return 1
	default:
		return 0
	}
}
