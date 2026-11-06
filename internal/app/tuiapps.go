package app

import "strings"

// isTUI identifies packages from the tui-tools family by their package name or
// one of the family's executables.
func isTUI(name string, commands []string) bool {
	if strings.HasPrefix(name, "tui-") {
		return true
	}
	for _, command := range commands {
		if strings.HasPrefix(command, "tui-") {
			return true
		}
	}
	return false
}
