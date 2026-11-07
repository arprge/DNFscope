package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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
	rpmQueryFormat = "\x01%{NAME}\t%{ARCH}\t%{VERSION}-%{RELEASE}\t%{SIZE}\t%{INSTALLTIME}\t%{SUMMARY}\t%{URL}\n[%{FILENAMES}\t%{FILEMODES}\n]"
)

var (
	reasonLineRe = regexp.MustCompile(`^"([^"\\]+)"\s*=\s*\{\s*reason\s*=\s*"([^"\\]+)"\s*,?\s*\}\s*(?:#.*)?$`)
	devNameRe    = regexp.MustCompile(`(?i)-(devel|static|headers|debuginfo|debugsource|docs?|langpacks?.*|libs)$`)
	devSummaryRe = regexp.MustCompile(`(?i)^(development files|header files|libraries and headers|static libraries|documentation for|language packs)`)
	binPathRe    = regexp.MustCompile(`^/(?:usr/(?:local/)?)?s?bin/[^/]+$`)
	nevraRe      = regexp.MustCompile(`^([A-Za-z0-9+_.-]+)-(?:[0-9]+:)?([^-:]+)-([^-:]+)\.([^.]+)$`)
)

func isUserReason(r string) bool {
	return r == "User" || r == "External User"
}

// loadReasons reads the per-package install reason written by libdnf5.
// libdnf5 writes "name.arch" = {reason = "User"} under [packages].
func loadReasons() (map[string]string, error) {
	f, err := os.Open(filepath.Join(libdnfDir, "packages.toml"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseReasons(f)
}

// Parse libdnf5's generated TOML subset, not arbitrary TOML. Unsupported records
// must fail rather than silently turning user packages into dependencies.
func parseReasons(r io.Reader) (map[string]string, error) {
	reasons := make(map[string]string, 4096)
	sc := bufio.NewScanner(r)
	inPackages, foundPackages := false, false
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section := strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
			inPackages = section == "[packages]"
			foundPackages = foundPackages || inPackages
			continue
		}
		if !inPackages {
			continue
		}
		m := reasonLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("packages.toml line %d: unsupported package reason record", lineNo)
		}
		if _, exists := reasons[m[1]]; exists {
			return nil, fmt.Errorf("packages.toml line %d: duplicate package %q", lineNo, m[1])
		}
		reasons[m[1]] = m[2]
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if !foundPackages {
		return nil, fmt.Errorf("packages.toml: missing [packages] table")
	}
	return reasons, nil
}

// DNF's stored cookie does not change after a direct rpm transaction. Inspect
// the actual rpmdb too, and history (including SQLite's WAL) used by the baseline.
func fingerprint() (string, error) {
	out, err := runCmd(context.Background(), "rpm", "--eval", "%{_dbpath}")
	if err != nil {
		return "", err
	}
	dbDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dbDir) {
		return "", fmt.Errorf("rpm returned invalid database path %q", dbDir)
	}
	return fingerprintDirs(libdnfDir, dbDir)
}

func fingerprintDirs(stateDir, dbDir string) (string, error) {
	h := sha256.New()
	for _, name := range []string{"system.toml", "packages.toml", "nevras.toml"} {
		b, err := os.ReadFile(filepath.Join(stateDir, name))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s:%d:", name, len(b))
		h.Write(b)
	}
	paths := []string{
		filepath.Join(stateDir, "transaction_history.sqlite"),
		filepath.Join(stateDir, "transaction_history.sqlite-wal"),
	}
	dbFound := false
	for _, name := range []string{"rpmdb.sqlite", "Packages", "data.mdb"} {
		path := filepath.Join(dbDir, name)
		if _, err := os.Stat(path); err == nil {
			dbFound = true
		} else if !os.IsNotExist(err) {
			return "", err
		}
		paths = append(paths, path)
	}
	if !dbFound {
		return "", fmt.Errorf("no supported RPM database in %s", dbDir)
	}
	paths = append(paths, filepath.Join(dbDir, "rpmdb.sqlite-wal"))
	for _, path := range paths {
		st, err := os.Stat(path)
		if os.IsNotExist(err) {
			fmt.Fprintf(h, "|%s:missing", path)
			continue
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "|%s:%d:%d", path, st.Size(), st.ModTime().UnixNano())
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
	Status      string `json:"status"`
	Packages    []struct {
		NEVRA  string `json:"nevra"`
		Action string `json:"action"`
	} `json:"packages"`
}

// loadBaseNames returns the name.arch keys installed by the base image
// transactions. The lowest transaction is only trusted when it started from an
// empty rpmdb (i.e. the system installer); image builds add a few consecutive
// installroot or KIWI-configured transactions after it.
func loadBaseNames(ctx context.Context) (map[string]struct{}, bool, error) {
	return loadBaseNamesWith(ctx, runCmd)
}

type commandRunner func(context.Context, string, ...string) ([]byte, error)

func loadBaseNamesWith(ctx context.Context, run commandRunner) (map[string]struct{}, bool, error) {
	out, err := run(ctx, "dnf5", "history", "list", "--json")
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

	first, err := historyInfoWith(ctx, ids[0], run)
	if err != nil {
		return nil, false, err
	}
	if first.RPMDBBegin != emptyRPMDB || first.Status != "Ok" {
		return nil, false, nil // history was rotated; no reliable baseline
	}

	base := make(map[string]struct{}, 2048)
	addBaseNames(base, first)
	for i := 1; i < len(ids) && i <= 3 && ids[i] == ids[i-1]+1; i++ {
		info, err := historyInfoWith(ctx, ids[i], run)
		if err != nil {
			return nil, false, err
		}
		if !isImageBuild(info.Description) {
			break
		}
		if info.Status != "Ok" {
			return nil, false, fmt.Errorf("image transaction %d has status %q", info.ID, info.Status)
		}
		addBaseNames(base, info)
	}
	return base, true, nil
}

func historyInfo(ctx context.Context, id int) (histInfo, error) {
	return historyInfoWith(ctx, id, runCmd)
}

func historyInfoWith(ctx context.Context, id int, run commandRunner) (histInfo, error) {
	out, err := run(ctx, "dnf5", "history", "info", strconv.Itoa(id), "--json")
	if err != nil {
		return histInfo{}, err
	}
	var infos []histInfo
	if err := json.Unmarshal(out, &infos); err != nil {
		return histInfo{}, err
	}
	if len(infos) != 1 || infos[0].ID != id {
		return histInfo{}, fmt.Errorf("dnf5 history info %d: unexpected result", id)
	}
	return infos[0], nil
}

// Match image-builder options, not package names such as anaconda or installer.
func isImageBuild(description string) bool {
	args := strings.Fields(description)
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "--installroot=") && len(arg) > len("--installroot=") {
			return true
		}
		if arg == "--installroot" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			return true
		}
		config := ""
		if strings.HasPrefix(arg, "--config=") {
			config = strings.TrimPrefix(arg, "--config=")
		} else if (arg == "--config" || arg == "-c") && i+1 < len(args) {
			config = args[i+1]
		}
		if filepath.Base(strings.Trim(config, "\"'")) == "kiwi_dnf5.config" {
			return true
		}
	}
	return false
}

func addBaseNames(base map[string]struct{}, info histInfo) {
	// Process outgoing records first: upgrades can contain Replaced and Upgrade
	// entries with the same name.arch, in either order.
	for _, p := range info.Packages {
		if p.Action == "Remove" || p.Action == "Replaced" {
			delete(base, nevraKey(p.NEVRA))
		}
	}
	for _, p := range info.Packages {
		switch p.Action {
		case "Install", "Upgrade", "Downgrade", "Reinstall":
			if key := nevraKey(p.NEVRA); key != "" {
				base[key] = struct{}{}
			}
		}
	}
}

// repoQueryFormat yields the same name.arch|VERSION-RELEASE key that the rpm
// scan uses, plus the repository the installed build originally came from.
// %{version}-%{release} deliberately omits the epoch, matching rpm's EVR here.
const repoQueryFormat = "%{name}.%{arch}|%{version}-%{release}|%{from_repo}\n"

// loadInstalledRepos maps name.arch|EVR to the repository that installed that
// build. libdnf5 stores the origin per installed package, so one repoquery call
// replaces walking the entire transaction history. This is also more complete:
// packages whose history was rotated away still report their origin.
func loadInstalledRepos(ctx context.Context) (map[string]string, error) {
	return loadInstalledReposWith(ctx, runCmd)
}

func loadInstalledReposWith(ctx context.Context, run commandRunner) (map[string]string, error) {
	out, err := run(ctx, "dnf5", "repoquery", "--installed", "--queryformat", repoQueryFormat)
	if err != nil {
		return nil, err
	}
	repos := make(map[string]string, 4096)
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		sep := strings.LastIndexByte(line, '|')
		if sep <= 0 {
			continue // package ids and repoids never contain a bare '|'
		}
		repo := line[sep+1:]
		if repo == "" || strings.HasPrefix(repo, "@") {
			continue // @System/@commandline are not real origins
		}
		repos[line[:sep]] = repo
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return repos, nil
}

func nevraKey(nevra string) string {
	if m := nevraRe.FindStringSubmatch(nevra); m != nil {
		return m[1] + "." + m[4]
	}
	return ""
}

// buildPackages scans the rpmdb once and keeps only packages the user asked
// for. withRepos reports whether the origin lookup ran successfully, so callers
// can distinguish "not loaded yet" from a package with no known origin.
func buildPackages(ctx context.Context, includeRepos bool) ([]Package, bool, bool, error) {
	reasons, err := loadReasons()
	if err != nil {
		return nil, false, false, fmt.Errorf("read packages.toml: %w", err)
	}

	base, baseFound, err := loadBaseNames(ctx)
	if err != nil {
		base = map[string]struct{}{}
		baseFound = false
	}

	if includeRepos {
		// The repo lookup is independent of the rpm scan, so overlap them. A
		// failed lookup only hides the repo row.
		type repoResult struct {
			repos map[string]string
			ok    bool
		}
		reposCh := make(chan repoResult, 1)
		go func() {
			repos, err := loadInstalledRepos(ctx)
			reposCh <- repoResult{repos: repos, ok: err == nil}
		}()

		pkgs, err := scanRPM(ctx, reasons, base, baseFound)
		res := <-reposCh
		if err != nil {
			return nil, baseFound, false, err
		}
		return finishPackages(pkgs, res.repos), baseFound, res.ok, nil
	}

	pkgs, err := scanRPM(ctx, reasons, base, baseFound)
	if err != nil {
		return nil, baseFound, false, err
	}
	return finishPackages(pkgs, nil), baseFound, false, nil
}

func finishPackages(pkgs []Package, repos map[string]string) []Package {
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
		p.Repo = repos[p.Name+"."+p.Arch+"|"+p.EVR]
		p.DevNoise = devNameRe.MatchString(p.Name) || devSummaryRe.MatchString(p.Summary)
		p.SystemComponent = isSystemComponent(p.Name)

	}

	sort.SliceStable(pkgs, func(i, j int) bool { return pkgs[i].InstallTime.After(pkgs[j].InstallTime) })
	return pkgs
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

func scanRPM(ctx context.Context, reasons map[string]string, base map[string]struct{}, baseFound bool) ([]Package, error) {
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

	out, scanErr := parseRPM(stdout, reasons, base, baseFound, executableFile)
	// A parser error may leave rpm blocked writing to its pipe.
	if scanErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if scanErr != nil {
		return nil, scanErr
	}
	if waitErr != nil {
		return nil, fmt.Errorf("rpm -qa: %w", waitErr)
	}
	return out, nil
}

func parseRPM(r io.Reader, reasons map[string]string, base map[string]struct{}, baseFound bool, executable func(string) bool) ([]Package, error) {
	out := make([]Package, 0, 256)
	idx := -1 // index of the package currently receiving files
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()

		if strings.HasPrefix(line, "/") {
			if idx >= 0 {
				path, modeText, ok := strings.Cut(line, "\t")
				mode, err := strconv.ParseUint(modeText, 10, 32)
				if !ok || err != nil {
					return nil, fmt.Errorf("rpm: invalid file mode record %q", line)
				}
				addFile(&out[idx], path, mode, executable)
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
		reason, tracked := reasons[key]
		// An installed row absent from packages.toml can be an explicitly
		// installed package that libdnf5 never recorded. Recover it without an
		// extra query, but only when the baseline proves it is not part of the
		// image; RPM metadata alone cannot tell this from an ordinary base
		// package here.
		if !tracked && baseFound && !isBase(base, key) && f[0] != "gpg-pubkey" {
			reason = "Untracked"
		}
		if !isUserReason(reason) && reason != "Untracked" {
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
			Reason:      reason,
			FromBase:    isBase(base, key),
		})
		idx = len(out) - 1
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func isBase(base map[string]struct{}, key string) bool {
	_, ok := base[key]
	return ok
}

func executableFile(path string) bool {
	st, err := os.Stat(path) // follow symlinks, including targets owned by other RPMs
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
}

func addFile(p *Package, path string, mode uint64, executable func(string) bool) {
	if !p.HasDesktop && strings.HasSuffix(path, ".desktop") && strings.Contains(path, "/share/applications/") {
		p.HasDesktop = true
	}
	if !binPathRe.MatchString(path) {
		return
	}
	const fileType = 0o170000
	regularExecutable := mode&fileType == 0o100000 && mode&0o111 != 0
	// Alternatives-managed %ghost files can be recorded as non-executable
	// regular files even though their installed paths are executable symlinks.
	checkInstalled := mode&fileType == 0o120000 || mode&fileType == 0o100000 && !regularExecutable
	installedExecutable := checkInstalled && executable(path)
	if regularExecutable || installedExecutable {
		p.Commands = append(p.Commands, filepath.Base(path))
	}
}
