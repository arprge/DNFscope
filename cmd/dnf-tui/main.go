package main

import (
	"fmt"
	"os"

	"dnf-tui/internal/app"
)

const usage = `usage: dnf-tui [--list] [--no-icons] [--icons]

  --list       print the default view as TSV and exit
  --no-icons   disable Nerd Font glyphs (toggle at runtime with i)
  --icons      force Nerd Font glyphs (default)
`

func main() {
	list := false
	icons := true

	for _, arg := range os.Args[1:] {
		switch arg {
		case "--list":
			list = true
		case "--icons":
			icons = true
		case "--no-icons":
			icons = false
		case "-h", "--help":
			fmt.Print(usage)
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown argument %q\n%s", arg, usage)
			os.Exit(2)
		}
	}

	if list {
		if err := app.RunList(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "dnf-tui:", err)
			os.Exit(1)
		}
		return
	}

	if err := app.RunTUI(icons); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
