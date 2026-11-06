package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	libdnfDir = "/usr/lib/sysimage/libdnf5"

	// SHA-256 of an empty rpmdb, stored as rpmdb_version_begin by the very
	// first dnf5 transaction on a fresh install.
	emptyRPMDB = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// One rpm call yields metadata plus every file owned by every package.
	rpmQueryFormat = "\x01%{NAME}\t%{ARCH}\t%{VERSION}-%{RELEASE}\t%{SIZE}\t%{INSTALLTIME}\t%{SUMMARY}\t%{URL}\n[%{FILENAMES}\n]"
)

var (
	reasonLineRe  = regexp.MustCompile(`^"([^"]+)" = \{reason = "([^"]+)"\}$`)
	devNameRe     = regexp.MustCompile(`(?i)-(devel|static|headers|debuginfo|debugsource|docs?|langpacks?.*|libs)$`)
	devSummaryRe  = regexp.MustCompile(`(?i)^(development files|header files|libraries and headers|static libraries|documentation for|language packs)`)
	binPathRe     = regexp.MustCompile(`^/usr/(?:(?:s?bin|local/bin)/[^/]+|libexec/git-core/[^/]+)$`)
	nevraRe       = regexp.MustCompile(`^(.*)-(\d+):(.*)\.([^.]+)$`)
	baseDescRe    = regexp.MustCompile(`(?i)(--installroot|kiwi|anaconda|installer|livecd)`)
	rpmdbCookieRe = regexp.MustCompile(`rpmdb_cookie\s*=\s*"([^"]+)"`)
)

func isUserReason(r string) bool {
	return r == "User" || r == "External User"
}

// loadReasons reads the per-package install reason written by libdnf5.
// The file is a flat TOML table: "name.arch" = {reason = "User"}.
func loadReasons() (map[string]string, error) {
	f, err := os.Open(filepath.Join(libdnfDir, "packages.toml"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reasons := make(map[string]string, 4096)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := reasonLineRe.FindStringSubmatch(sc.Text()); m != nil {
			reasons[m[1]] = m[2]
		}
	}
	return reasons, sc.Err()
}

// fingerprint changes whenever the rpmdb or dnf5's stored reasons change, so a
// cached result can be trusted without re-scanning.
func fingerprint() (string, error) {
	h := sha256.New()
	cookie := ""
	if b, err := os.ReadFile(filepath.Join(libdnfDir, "system.toml")); err == nil {
		if m := rpmdbCookieRe.FindSubmatch(b); m != nil {
			cookie = string(m[1])
		}
	}
	fmt.Fprintf(h, "cookie=%s", cookie)
	for _, name := range []string{"packages.toml", "nevras.toml"} {
		st, err := os.Stat(filepath.Join(libdnfDir, name))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "|%s:%d:%d", name, st.Size(), st.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}

type histInfo struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	RPMDBBegin  string `json:"rpmdb_version_begin"`
	Packages    []struct {
		NEVRA string `json:"nevra"`
	} `json:"packages"`
}

// loadBaseNames returns the name.arch keys installed by the base image
// transactions. The lowest transaction is only trusted when it started from an
// empty rpmdb (i.e. the system installer); image builds add a few consecutive
// --installroot transactions after it.
func loadBaseNames(ctx context.Context) (map[string]struct{}, bool, error) {
	out, err := runCmd(ctx, "dnf", "history", "list", "--json")
	if err != nil {
		return nil, false, err
	}
	var list []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, false, err
	}
	if len(list) == 0 {
		return nil, false, nil
	}
	ids := make([]int, 0, len(list))
	for _, t := range list {
		ids = append(ids, t.ID)
	}
	sort.Ints(ids)

	first, err := historyInfo(ctx, ids[0])
	if err != nil {
		return nil, false, err
	}
	if first.RPMDBBegin != emptyRPMDB {
		return nil, false, nil // history was rotated; no reliable baseline
	}

	base := make(map[string]struct{}, 2048)
	addBaseNames(base, first)
	for i := 1; i < len(ids) && i <= 3 && ids[i] == ids[i-1]+1; i++ {
		info, err := historyInfo(ctx, ids[i])
		if err != nil {
			break
		}
		if !baseDescRe.MatchString(info.Description) {
			break
		}
		addBaseNames(base, info)
	}
	return base, true, nil
}

func historyInfo(ctx context.Context, id int) (histInfo, error) {
	out, err := runCmd(ctx, "dnf", "history", "info", strconv.Itoa(id), "--json")
	if err != nil {
		return histInfo{}, err
	}
	var infos []histInfo
	if err := json.Unmarshal(out, &infos); err != nil {
		return histInfo{}, err
	}
	if len(infos) == 0 {
		return histInfo{}, fmt.Errorf("dnf history info %d: empty result", id)
	}
	return infos[0], nil
}

func addBaseNames(base map[string]struct{}, info histInfo) {
	for _, p := range info.Packages {
		base[nevraKey(p.NEVRA)] = struct{}{}
	}
}

func nevraKey(nevra string) string {
	if m := nevraRe.FindStringSubmatch(nevra); m != nil {
		return m[1] + "." + m[4]
	}
	return nevra
}

// buildPackages scans the rpmdb once and keeps only packages the user asked for.
func buildPackages(ctx context.Context) ([]Package, bool, error) {
	reasons, err := loadReasons()
	if err != nil {
		return nil, false, fmt.Errorf("read packages.toml: %w", err)
	}

	base, baseFound, err := loadBaseNames(ctx)
	if err != nil {
		base = map[string]struct{}{}
		baseFound = false
	}

	pkgs, err := scanRPM(ctx, reasons, base)
	if err != nil {
		return nil, baseFound, err
	}

	for i := range pkgs {
		p := &pkgs[i]
		if len(p.Commands) > 1 {
			sort.Strings(p.Commands)
			uniq := p.Commands[:1]
			for _, c := range p.Commands[1:] {
				if c != uniq[len(uniq)-1] {
					uniq = append(uniq, c)
				}
			}
			p.Commands = uniq
		}
		p.DevNoise = devNameRe.MatchString(p.Name) || devSummaryRe.MatchString(p.Summary)
		p.SystemComponent = isSystemComponent(p.Name)
	}

	sort.SliceStable(pkgs, func(i, j int) bool { return pkgs[i].InstallTime.After(pkgs[j].InstallTime) })
	return pkgs, baseFound, nil
}

// systemComponents are user-marked packages that ship desktop files but are
// really desktop plumbing. They are hidden from the default view and appear
// again with the "a" (all user) toggle.
var systemComponents = map[string]struct{}{
	"sddm":                       {},
	"qt6ct":                      {},
	"darkly":                     {},
	"jemalloc":                   {},
	"quickshell":                 {},
	"polkit-kde":                 {},
	"plasma-browser-integration": {},
	"kf6-kconfig":                {},
}

func isSystemComponent(name string) bool {
	_, ok := systemComponents[name]
	return ok
}

func scanRPM(ctx context.Context, reasons map[string]string, base map[string]struct{}) ([]Package, error) {
	cctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, "rpm", "-qa", "--qf", rpmQueryFormat)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start rpm: %w", err)
	}

	out := make([]Package, 0, 256)
	idx := -1 // index of the package currently receiving files
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()

		if strings.HasPrefix(line, "/") {
			if idx >= 0 {
				addFile(&out[idx], line)
			}
			continue
		}
		if !strings.HasPrefix(line, "\x01") {
			continue // multi-line summary leftover or other noise
		}

		f := strings.SplitN(strings.TrimPrefix(line, "\x01"), "\t", 7)
		if len(f) != 7 {
			idx = -1
			continue
		}
		key := f[0] + "." + f[1]
		if !isUserReason(reasons[key]) {
			idx = -1
			continue
		}
		size, _ := strconv.ParseInt(f[3], 10, 64)
		installed, _ := strconv.ParseInt(f[4], 10, 64)
		out = append(out, Package{
			Name:        f[0],
			Arch:        f[1],
			EVR:         f[2],
			Size:        size,
			InstallTime: time.Unix(installed, 0),
			Summary:     f[5],
			URL:         f[6],
			Reason:      reasons[key],
			FromBase:    isBase(base, key),
		})
		idx = len(out) - 1
	}
	scanErr := sc.Err()
	waitErr := cmd.Wait()
	if scanErr != nil {
		return nil, scanErr
	}
	if waitErr != nil {
		return nil, fmt.Errorf("rpm -qa: %w", waitErr)
	}
	return out, nil
}

func isBase(base map[string]struct{}, key string) bool {
	_, ok := base[key]
	return ok
}

func addFile(p *Package, path string) {
	if !p.HasDesktop && strings.HasSuffix(path, ".desktop") && strings.Contains(path, "/share/applications/") {
		p.HasDesktop = true
	}
	if binPathRe.MatchString(path) {
		p.Commands = append(p.Commands, filepath.Base(path))
	}
}
