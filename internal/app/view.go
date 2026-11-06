package app

import "strings"

func (m model) View() string {
	l := m.lay
	parts := make([]string, 0, l.termH)
	parts = append(parts, m.headerRow())

	if m.help {
		parts = append(parts, m.helpPanel(l.termH-2)...)
		parts = append(parts, m.footerRow())
		return strings.Join(parts, "\n")
	}

	if l.searchRow {
		parts = append(parts, m.searchRow())
	}
	if l.tabsRow {
		parts = append(parts, m.tabsRow())
	}
	if l.contentGap {
		parts = append(parts, blankLine(l.termW))
	}

	list := m.listPanel(m.focused == paneList)
	switch l.mode {
	case modeSplit:
		right := m.detailPanel(l.detW, l.detH, m.focused == paneDetails)
		if l.ovH > 0 {
			right = append(right, m.overviewPanel(l.detW, l.ovH)...)
		}
		parts = append(parts, joinCols(list, right)...)
	case modeStack:
		parts = append(parts, list...)
		parts = append(parts, m.detailPanel(l.detW, l.detH, m.focused == paneDetails)...)
	default:
		parts = append(parts, list...)
		if l.strip {
			parts = append(parts, m.stripRow())
		}
	}

	parts = append(parts, m.footerRow())
	return strings.Join(parts, "\n")
}
