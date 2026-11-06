package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
)

type fakeDNF struct {
	history map[int]string // transaction ID -> packages JSON
	infoIDs [][]string     // IDs requested by each "history info" call
}

func (f *fakeDNF) run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name != "dnf5" || len(args) == 0 || args[0] != "history" {
		return nil, fmt.Errorf("unexpected call: %s %v", name, args)
	}
	if args[1] == "list" {
		var parts []string
		for id := range f.history {
			parts = append(parts, fmt.Sprintf(`{"id":%d}`, id))
		}
		return []byte("[" + strings.Join(parts, ",") + "]"), nil
	}
	var ids []string
	var parts []string
	for _, a := range args[2:] {
		if a == "--json" {
			continue
		}
		var id int
		fmt.Sscan(a, &id)
		ids = append(ids, a)
		parts = append(parts, fmt.Sprintf(`{"id":%d,"packages":%s}`, id, f.history[id]))
	}
	slices.Sort(ids)
	f.infoIDs = append(f.infoIDs, ids)
	return []byte("[" + strings.Join(parts, ",") + "]"), nil
}

func TestLoadReposIncremental(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	f := &fakeDNF{history: map[int]string{
		1: `[{"nevra":"fd-find-0:9.0.0-1.fc45.x86_64","action":"Install","repository":"fedora"}]`,
		2: `[{"nevra":"fd-find-0:9.0.0-1.fc45.x86_64","action":"Replaced","repository":"@System"},
			{"nevra":"fd-find-0:9.1.0-1.fc45.x86_64","action":"Upgrade","repository":"updates"},
			{"nevra":"gum-0:0.1.0-1.fc45.x86_64","action":"Install","repository":"@commandline"}]`,
	}}

	repos, err := loadReposWith(context.Background(), f.run)
	if err != nil {
		t.Fatal(err)
	}
	if got := repos["fd-find.x86_64|9.1.0-1.fc45"]; got != "updates" {
		t.Fatalf("fd-find 9.1.0 repo = %q, want %q", got, "updates")
	}
	if got := repos["fd-find.x86_64|9.0.0-1.fc45"]; got != "fedora" {
		t.Fatalf("fd-find 9.0.0 repo = %q, want %q", got, "fedora")
	}
	if _, ok := repos["gum.x86_64|0.1.0-1.fc45"]; ok {
		t.Fatal("pseudo repository @commandline should not be recorded")
	}
	if !slices.Equal(f.infoIDs[0], []string{"1", "2"}) {
		t.Fatalf("first scan queried %v, want all transactions", f.infoIDs[0])
	}

	// A new transaction must be the only one fetched, and it overrides older data.
	f.history[3] = `[{"nevra":"fd-find-0:9.1.0-1.fc45.x86_64","action":"Install","repository":"copr:example"}]`
	repos, err = loadReposWith(context.Background(), f.run)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.infoIDs[1], []string{"3"}) {
		t.Fatalf("second scan queried %v, want only [3]", f.infoIDs[1])
	}
	if got := repos["fd-find.x86_64|9.1.0-1.fc45"]; got != "copr:example" {
		t.Fatalf("fd-find 9.1.0 repo = %q, want newest %q", got, "copr:example")
	}

	// Nothing new: no history info call at all.
	if _, err := loadReposWith(context.Background(), f.run); err != nil {
		t.Fatal(err)
	}
	if len(f.infoIDs) != 2 {
		t.Fatalf("unchanged history made %d info calls, want 2", len(f.infoIDs))
	}
}
