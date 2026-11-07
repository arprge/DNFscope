package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

type repoQueryCall struct {
	name string
	args []string
}

type fakeRepoQuery struct {
	out   string
	err   error
	calls []repoQueryCall
}

func (f *fakeRepoQuery) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, repoQueryCall{name: name, args: append([]string(nil), args...)})
	if f.err != nil {
		return nil, f.err
	}
	return []byte(f.out), nil
}

func TestLoadInstalledRepos(t *testing.T) {
	f := &fakeRepoQuery{out: "" +
		"fd-find.x86_64|9.1.0-1.fc45|updates\n" +
		"gum.x86_64|0.1.0-1.fc45|@commandline\n" +
		"systemd.x86_64|257-1.fc45|@System\n" +
		"local.x86_64|1-1|\n" +
		"code.x86_64|1.145.2-1793945328.el8|code\n"}

	repos, err := loadInstalledReposWith(context.Background(), f.run)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"fd-find.x86_64|9.1.0-1.fc45":        "updates",
		"code.x86_64|1.145.2-1793945328.el8": "code",
	}
	for key, repo := range want {
		if got := repos[key]; got != repo {
			t.Fatalf("%s repo = %q, want %q", key, got, repo)
		}
	}
	for _, key := range []string{"gum.x86_64|0.1.0-1.fc45", "systemd.x86_64|257-1.fc45", "local.x86_64|1-1"} {
		if _, ok := repos[key]; ok {
			t.Fatalf("pseudo or empty origin %q should not be recorded", key)
		}
	}
	if len(repos) != len(want) {
		t.Fatalf("parsed %d origins, want %d: %v", len(repos), len(want), repos)
	}

	if len(f.calls) != 1 {
		t.Fatalf("made %d calls, want 1", len(f.calls))
	}
	call := f.calls[0]
	if call.name != "dnf5" || !slices.Equal(call.args, []string{"repoquery", "--installed", "--queryformat", repoQueryFormat}) {
		t.Fatalf("unexpected call: %s %v", call.name, call.args)
	}
}

func TestLoadInstalledReposError(t *testing.T) {
	wantErr := errors.New("dnf5 exploded")
	f := &fakeRepoQuery{err: wantErr}

	if _, err := loadInstalledReposWith(context.Background(), f.run); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestMatchesRepository(t *testing.T) {
	p := Package{Name: "example", Summary: "an example package", Repo: "copr:copr.fedorainfracloud.org:user:project"}
	if !matches(p, "fedora") {
		t.Fatal("search should match the origin repository")
	}
	p.Repo = ""
	if matches(p, "fedora") {
		t.Fatal("empty origin must not match any repository name")
	}
}

func TestRepoQueryFormatMatchesRPMEVR(t *testing.T) {
	// The key must stay in the name.arch|VERSION-RELEASE shape produced by the
	// rpm scan, otherwise origins silently never match.
	for _, tag := range []string{"%{name}.%{arch}|", "|%{version}-%{release}|", "|%{from_repo}\n"} {
		if !strings.Contains(repoQueryFormat, tag) {
			t.Fatalf("repoQueryFormat %q is missing %q", repoQueryFormat, tag)
		}
	}
	if strings.Contains(repoQueryFormat, "epoch") {
		t.Fatal("repoQueryFormat must omit the epoch to match rpm's EVR")
	}
}
