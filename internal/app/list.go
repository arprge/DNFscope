package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// RunList prints the default view without starting the TUI; used for checks.
func RunList(w io.Writer) error {
	fp, fpErr := fingerprint()

	var pkgs []Package
	baseFound := false
	fromCache := false

	if cf, err := readCache(); err == nil && fpErr == nil && cf.Fingerprint == fp {
		pkgs = cf.Packages
		baseFound = cf.BaseFound
		fromCache = true
	} else {
		var err error
		pkgs, baseFound, err = buildPackages(context.Background())
		if err != nil {
			return err
		}
		if fpErr == nil {
			_ = writeCache(&cacheFile{
				Schema:      cacheSchema,
				Fingerprint: fp,
				BaseFound:   baseFound,
				Packages:    pkgs,
			})
		}
	}

	shown := 0
	for _, p := range pkgs {
		if p.FromBase || p.DevNoise || p.SystemComponent || p.TUI || (!p.HasDesktop && len(p.Commands) == 0) {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			p.KindLabel(), p.Name, strings.Join(p.Commands, ","), humanSize(p.Size), fmtDate(p.InstallTime), p.Summary)
		shown++
	}
	fmt.Fprintf(os.Stderr, "%d shown · %d user packages · base=%v · cache=%v\n", shown, len(pkgs), baseFound, fromCache)
	return nil
}
