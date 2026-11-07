package app

const (
	minWidth  = 30
	minHeight = 8
	splitMinW = 72
)

type layoutMode int

const (
	modeList  layoutMode = iota // list only, one-line detail strip
	modeStack                   // list above details
	modeSplit                   // list beside details and overview
)

// layout is the resolved geometry for one terminal size. Everything that
// renders reads from here.
//
// Rows top to bottom:
//
//	header      1
//	search      0 or 1  (height >= 14)
//	tabs        0 or 1  (height >= 9)
//	body        avail
//	footer      1
type layout struct {
	termW, termH int
	icons        bool

	mode       layoutMode
	tabsRow    bool
	searchRow  bool
	contentGap bool
	avail      int

	listW, listH, listInnerW, bodyH int
	detW, detH                      int
	ovH                             int
	strip                           bool
	showDate                        bool
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func computeLayout(w, h int, icons bool, splitW, stackDetailH, overviewH int) layout {
	if w < minWidth {
		w = minWidth
	}
	if h < minHeight {
		h = minHeight
	}
	l := layout{termW: w, termH: h, icons: icons}
	l.tabsRow = h >= 9
	l.searchRow = h >= 14
	l.contentGap = h >= 5
	l.avail = h - 2 - boolInt(l.tabsRow) - boolInt(l.searchRow) - boolInt(l.contentGap)

	switch {
	case w >= splitMinW && l.avail >= 10:
		l.mode = modeSplit
		if splitW == 0 {
			splitW = w * 40 / 100
		}
		l.detW = clamp(splitW, 32, min(56, w-40))
		l.listW = w - l.detW
		l.listH = l.avail
		l.detH = l.avail
		if l.avail >= 18 {
			if overviewH == 0 {
				overviewH = 7
			}
			l.ovH = clamp(overviewH, 4, l.avail-6)
			l.detH = l.avail - l.ovH
		}
	case l.avail >= 14:
		l.mode = modeStack
		l.detW = w
		if stackDetailH == 0 {
			stackDetailH = 6
			if l.avail >= 18 {
				stackDetailH = 8
			}
		}
		l.detH = clamp(stackDetailH, 6, l.avail-6)
		l.listW = w
		l.listH = l.avail - l.detH
	default:
		l.mode = modeList
		l.listW = w
		l.strip = l.avail >= 5
		l.listH = l.avail - boolInt(l.strip)
	}

	l.listInnerW = l.listW - 2
	l.bodyH = max(1, l.listH-2)
	l.showDate = l.listInnerW >= 40
	return l
}

func clamp(v, lo, hi int) int {
	return max(lo, min(hi, v))
}
