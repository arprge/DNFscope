package app

// Filters hold the toggles exposed with a/b/d.
type Filters struct {
	AllUser     bool // show every user package, including fonts/themes/libs
	IncludeBase bool // show packages from the base image transactions
	ShowDev     bool // show -devel/-doc/-static noise
}

type SortMode int

const (
	SortDate SortMode = iota
	SortName
	SortSize
	SortType
)

func (s SortMode) String() string {
	switch s {
	case SortName:
		return "name"
	case SortSize:
		return "size"
	case SortType:
		return "type"
	default:
		return "date"
	}
}

func (s SortMode) next() SortMode {
	return (s + 1) % 4
}

// KindFilter is the type tab: all, apps, cli tools or everything else. The
// values double as indexes into model.counts.
type KindFilter int

const (
	KindAll KindFilter = iota
	KindApp
	KindCLI
	KindPkg
)

const kindCount = 4

func (k KindFilter) next(delta int) KindFilter {
	return KindFilter((int(k) + delta + kindCount) % kindCount)
}

func (k KindFilter) label() string {
	switch k {
	case KindApp:
		return "app"
	case KindCLI:
		return "cli"
	case KindPkg:
		return "pkg"
	default:
		return "all"
	}
}

func kindIndex(p Package) KindFilter {
	switch p.KindLabel() {
	case "app":
		return KindApp
	case "cli":
		return KindCLI
	default:
		return KindPkg
	}
}