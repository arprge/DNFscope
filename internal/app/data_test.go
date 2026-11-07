package app

import (
	"strings"
	"testing"
)

func parseOneRPM(t *testing.T, input string, reasons map[string]string, base map[string]struct{}, baseFound bool) []Package {
	t.Helper()

	pkgs, err := parseRPM(
		strings.NewReader(input),
		reasons,
		base,
		baseFound,
		func(string) bool { return true },
	)
	if err != nil {
		t.Fatal(err)
	}
	return pkgs
}

func TestParseRPMIncludesUntrackedNonBasePackage(t *testing.T) {
	pkgs := parseOneRPM(t,
		"\x01patch\tnoarch\t2.7.6-14.fc45\t1024\t1700000000\tGNU patch tool\t\thttps://example.invalid\n[/usr/bin/patch\t33188\n]",
		map[string]string{},
		map[string]struct{}{},
		true,
	)
	if len(pkgs) != 1 {
		t.Fatalf("parsed %d packages, want 1", len(pkgs))
	}
	if pkgs[0].Reason != "Untracked" || pkgs[0].FromBase {
		t.Fatalf("untracked package = %#v, want Reason Untracked and FromBase false", pkgs[0])
	}
}

func TestParseRPMExcludesUntrackedBaseOrUnknownBaselinePackages(t *testing.T) {
	for _, test := range []struct {
		name      string
		base      map[string]struct{}
		baseFound bool
	}{
		{name: "base package", base: map[string]struct{}{"patch.noarch": {}}, baseFound: true},
		{name: "unknown baseline", base: map[string]struct{}{}, baseFound: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkgs := parseOneRPM(t,
				"\x01patch\tnoarch\t2.7.6-14.fc45\t1024\t1700000000\tGNU patch tool\t\thttps://example.invalid\n[/usr/bin/patch\t33188\n]",
				map[string]string{},
				test.base,
				test.baseFound,
			)
			if len(pkgs) != 0 {
				t.Fatalf("parsed %d packages, want 0: %#v", len(pkgs), pkgs)
			}
		})
	}
}

func TestParseRPMExcludesTrackedDependencies(t *testing.T) {
	pkgs := parseOneRPM(t,
		"\x01libexample\tx86_64\t1.0-1.fc45\t1024\t1700000000\tA library\t\thttps://example.invalid\n[/usr/lib64/libexample.so\t33188\n]",
		map[string]string{"libexample.x86_64": "Dependency"},
		map[string]struct{}{},
		true,
	)
	if len(pkgs) != 0 {
		t.Fatalf("parsed %d packages, want 0: %#v", len(pkgs), pkgs)
	}
}
