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

type dependencyResult struct {
	lines []string
	err   error
}

type depsDoneMsg struct {
	key    string
	result dependencyResult
}

func packageKey(p Package) string {
	return p.Name + "." + p.Arch
}

// loadDependencies reads the requirements of the installed package itself.
// Querying rpm avoids dnf repoquery selecting an available, different version.
func loadDependencies(key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, "rpm", "-q", "--requires", key).Output()
		if err != nil {
			return depsDoneMsg{key: key, result: dependencyResult{err: fmt.Errorf("rpm requirements: %w", err)}}
		}
		return depsDoneMsg{key: key, result: dependencyResult{lines: parseDependencies(string(out))}}
	}
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
