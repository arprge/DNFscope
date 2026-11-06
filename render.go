package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// span is one fixed-width cell of a rendered line.
type span struct {
	text  string
	style lipgloss.Style
	width int
	right bool
}

func cell(text string, style lipgloss.Style, w int) span {
	return span{text: text, style: style, width: w}
}

func rcell(text string, style lipgloss.Style, w int) span {
	return span{text: text, style: style, width: w, right: true}
}

func renderSpans(spans []span) string {
	var b strings.Builder
	for _, s := range spans {
		if s.width <= 0 {
			continue
		}
		t := ansi.Truncate(s.text, s.width, "…")
		pad := max(0, s.width-ansi.StringWidth(t))
		if s.right {
			b.WriteString(s.style.Render(strings.Repeat(" ", pad) + t))
			continue
		}
		b.WriteString(s.style.Render(t + strings.Repeat(" ", pad)))
	}
	return b.String()
}

// fitLine truncates to the given width and pads with the background colour so
// every row is exactly w cells.
func fitLine(s string, w int, bg lipgloss.Color) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += canvas(bg).Render(strings.Repeat(" ", pad))
	}
	return s
}

func centered(w int, msg string, fg, bg lipgloss.Color) string {
	msg = ansi.Truncate(msg, max(0, w-2), "…")
	pad := w - ansi.StringWidth(msg)
	left := pad / 2
	return canvas(bg).Render(strings.Repeat(" ", left)) +
		paint(fg, bg).Render(msg) +
		canvas(bg).Render(strings.Repeat(" ", pad-left))
}

// --- panels --------------------------------------------------------------

// panel draws a titled rounded box of exactly w x h cells. body lines are
// fitted to the inner width; missing lines are blank.
func panel(title, right string, w, h int, active bool, body []string) []string {
	bg := theme.base
	border := paint(theme.surface1, theme.mantle)
	titleStyle := paint(theme.subtext0, theme.mantle).Bold(true)
	if active {
		border = paint(theme.accent, theme.mantle)
		titleStyle = paint(theme.accent, theme.mantle).Bold(true)
	}
	inner := w - 2

	t := ansi.Truncate(title, max(0, inner-4), "…")
	tw := ansi.StringWidth(t)
	used := 3 + tw
	r := right
	rw := ansi.StringWidth(r)
	if r != "" && used+rw+3 > inner {
		r = ""
	}
	if r != "" {
		used += rw + 3
	}
	fill := max(0, inner-used)

	top := border.Render("┌─ ") + titleStyle.Render(t)
	if r == "" {
		top += border.Render(" " + strings.Repeat("─", fill) + "┐")
	} else {
		top += border.Render(" "+strings.Repeat("─", fill)+" ") +
			paint(theme.overlay1, theme.mantle).Render(r) + border.Render(" ─┐")
	}

	out := make([]string, 0, h)
	out = append(out, top)
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		out = append(out, border.Render("│")+fitLine(line, inner, bg)+border.Render("│"))
	}
	out = append(out, border.Render("└"+strings.Repeat("─", inner)+"┘"))
	return out
}

func joinCols(a, b []string) []string {
	n := max(len(a), len(b))
	out := make([]string, n)
	for i := range out {
		if i < len(a) {
			out[i] += a[i]
		}
		if i < len(b) {
			out[i] += b[i]
		}
	}
	return out
}

func blankLine(w int) string {
	return canvas(theme.mantle).Render(strings.Repeat(" ", w))
}

// --- header, search, tabs ------------------------------------------------

func (m model) statusText() (string, lipgloss.Style) {
	bg := theme.mantle
	switch {
	case m.err != nil:
		return "scan failed", paint(theme.red, bg)
	case m.loading:
		return spinnerFrames[m.spin] + " scanning rpmdb…", paint(theme.accent, bg)
	case m.flash:
		return fmt.Sprintf("%d user packages · updated", len(m.all)), paint(theme.accent, bg).Bold(true)
	default:
		return fmt.Sprintf("%d user packages", len(m.all)), paint(theme.overlay1, bg)
	}
}

func (m model) headerRow() string {
	l := m.lay
	bg := theme.mantle
	title := paint(theme.accent, bg).Bold(true).Render("DNFscope")
	tw := ansi.StringWidth(title)
	info, style := m.statusText()

	avail := l.termW - 2
	info = ansi.Truncate(info, max(0, avail-tw-1), "…")
	gap := max(1, avail-tw-ansi.StringWidth(info))
	line := canvas(bg).Render(" ") + title + canvas(bg).Render(strings.Repeat(" ", gap)) +
		style.Render(info) + canvas(bg).Render(" ")
	return fitLine(line, l.termW, bg)
}

func (m model) searchRow() string {
	l := m.lay
	bg := theme.surface0
	innerW := l.termW - 4

	var left string
	switch {
	case m.searching:
		left = paint(theme.accent, bg).Bold(true).Render("/  ") + paint(theme.text, bg).Render(m.query)
		caret := theme.overlay0
		if m.caretOn {
			caret = theme.accent
		}
		left += paint(caret, bg).Render("▏")
	case m.query != "":
		left = paint(theme.accent, bg).Bold(true).Render("/  ") + paint(theme.text, bg).Render(m.query)
	default:
		left = paint(theme.overlay1, bg).Render("/  Search packages, commands, summaries")
	}

	right := ""
	if m.query != "" {
		word := "matches"
		if len(m.visible) == 1 {
			word = "match"
		}
		right = fmt.Sprintf("%d %s", len(m.visible), word)
	}
	leftW, rightW := ansi.StringWidth(left), ansi.StringWidth(right)
	contentW := max(0, innerW)
	if rightW > 0 {
		left = ansi.Truncate(left, max(0, contentW-rightW-2), "…")
		leftW = ansi.StringWidth(left)
	}
	content := paint(theme.surface1, bg).Render(" ") + left
	if rightW > 0 {
		gap := max(1, contentW-1-leftW-rightW)
		content += canvas(bg).Render(strings.Repeat(" ", gap)) + paint(theme.subtext0, bg).Render(right)
	}
	content += paint(theme.surface1, bg).Render(strings.Repeat(" ", max(0, contentW-ansi.StringWidth(content))))
	return canvas(theme.mantle).Render(" ") + paint(theme.surface1, bg).Render("│") +
		content + paint(theme.surface1, bg).Render("│") + canvas(theme.mantle).Render(" ")
}

func (m model) tabsRow() string {
	l := m.lay
	bg := theme.mantle
	var segs []string
	for k := KindAll; k <= KindPkg; k++ {
		label := strings.ToUpper(k.label()[:1]) + k.label()[1:]
		if k == KindCLI {
			label = "CLI"
		}
		if l.icons && k != KindAll {
			label = kindIcon(k.label()) + " " + label
		}
		label = fmt.Sprintf(" %s %d ", label, m.counts[k])
		if k == m.kind {
			segs = append(segs, paint(theme.accent, bg).Render("▐")+
				paint(theme.crust, theme.accent).Bold(true).Render(label)+
				paint(theme.accent, bg).Render("▌"))
		} else {
			segs = append(segs, paint(theme.subtext0, bg).Render(" "+label+" "))
		}
	}
	left := canvas(bg).Render(" ") + strings.Join(segs, "")

	dir := "↓"
	if !m.sortDesc {
		dir = "↑"
	}
	right := paint(theme.overlay1, bg).Render("sort ") +
		paint(theme.text, bg).Render(m.sortMode.String()+" "+dir) +
		canvas(bg).Render(" ")
	for _, f := range []struct {
		key string
		on  bool
	}{{"all", m.filters.AllUser}, {"base", m.filters.IncludeBase}, {"dev", m.filters.ShowDev}} {
		st := paint(theme.overlay0, bg)
		if f.on {
			st = paint(theme.green, bg)
		}
		right = st.Render(f.key) + canvas(bg).Render(" ") + right
	}
	right = canvas(bg).Render(" ") + right

	gap := l.termW - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return fitLine(left, l.termW, bg)
	}
	return left + canvas(bg).Render(strings.Repeat(" ", gap)) + right
}

// --- list ----------------------------------------------------------------

func (m model) listPanel() []string {
	l := m.lay
	bg := theme.base
	inner := l.listInnerW

	var body []string
	if len(m.visible) == 0 {
		msg := "no packages match"
		if m.query != "" {
			msg = "no match for “" + m.query + "”"
		}
		for i := 0; i < l.bodyH; i++ {
			if i == l.bodyH/2 {
				body = append(body, centered(inner, msg, theme.overlay1, bg))
			} else {
				body = append(body, "")
			}
		}
	} else {
		for i := 0; i < l.bodyH; i++ {
			idx := m.offset + i
			if idx >= len(m.visible) {
				break
			}
			body = append(body, m.packageRow(idx))
		}
	}

	pos := ""
	if len(m.visible) > 0 {
		pos = fmt.Sprintf("%d/%d", m.cursor+1, len(m.visible))
	}
	return panel("Packages", pos, l.listW, l.listH, true, body)
}

func (m model) packageRow(idx int) string {
	l := m.lay
	p := m.visible[idx]
	sel := idx == m.cursor
	inner := l.listInnerW

	bg := theme.base
	if sel {
		bg = theme.surface0
	}
	gutter, gutterStyle := " ", canvas(bg)
	if sel {
		gutter, gutterStyle = "▌", paint(theme.accent, bg)
	}
	nameStyle := paint(theme.text, bg)
	if p.FromBase {
		nameStyle = paint(theme.overlay0, bg)
	}
	if sel {
		nameStyle = nameStyle.Bold(true)
	}

	badge, badgeW := p.KindLabel(), 3
	if l.icons {
		badge, badgeW = kindIcon(p.KindLabel()), 1
	}

	dateW := 0
	if l.showDate {
		dateW = 9
	}
	// gutter 1 + badge + gap 1 + name + gap 2 + cmds + date + pad 1
	rest := inner - 1 - badgeW - 1 - 2 - dateW - 1
	nameW := clamp(rest*45/100, 6, 30)
	cmdW := rest - nameW
	if cmdW < 0 {
		nameW, cmdW = max(1, rest), 0
	}

	cmds := strings.Join(p.Commands, ", ")
	if cmds == "" && p.KindLabel() == "app" {
		cmds = p.Summary
	}

	spans := []span{
		cell(gutter, gutterStyle, 1),
		cell(badge, paint(kindColor(p.KindLabel()), bg), badgeW),
		cell(" ", canvas(bg), 1),
		cell(p.Name, nameStyle, nameW),
		cell("  ", canvas(bg), 2),
		cell(cmds, paint(theme.subtext0, bg), cmdW),
	}
	if dateW > 0 {
		spans = append(spans, rcell(relDate(p.InstallTime, time.Now()), paint(theme.overlay1, bg), dateW))
	}
	spans = append(spans, cell("", canvas(bg), 1))

	// Make the widths sum to exactly inner no matter how the arithmetic fell.
	sum := 0
	for _, s := range spans {
		sum += s.width
	}
	if sum < inner {
		spans = append(spans, cell("", canvas(bg), inner-sum))
	}
	return renderSpans(spans)
}

// --- details -------------------------------------------------------------

func kv(label, value string, valueStyle lipgloss.Style, w int) string {
	bg := theme.base
	return renderSpans([]span{
		cell(" ", canvas(bg), 1),
		cell(label, paint(theme.overlay0, bg), 10),
		cell(value, valueStyle, max(0, w-11)),
	})
}

func (m model) detailPanel(w, h int) []string {
	bg := theme.base
	inner := w - 2
	p, ok := m.selected()
	if !ok {
		return panel("Details", "", w, h, false, []string{centered(inner, "nothing selected", theme.overlay0, bg)})
	}

	accent := kindColor(p.KindLabel())
	head := " " + paint(theme.text, bg).Bold(true).Render(p.Name) +
		paint(theme.overlay1, bg).Render(" "+p.EVR)
	lines := []string{head}

	for _, ln := range strings.Split(ansi.Wrap(p.Summary, max(1, inner-2), ""), "\n") {
		lines = append(lines, " "+paint(theme.subtext1, bg).Render(ln))
		if len(lines) >= 4 {
			break
		}
	}
	lines = append(lines, "")

	text := paint(theme.subtext0, bg)
	kind := p.KindLabel()
	if p.FromBase {
		kind += " · base image"
	}
	lines = append(lines, kv("type", kind, paint(accent, bg), inner))
	cmd := "—"
	if len(p.Commands) > 0 {
		cmd = strings.Join(p.Commands, ", ")
	}
	lines = append(lines,
		kv("commands", cmd, text, inner),
		kv("size", humanSize(p.Size), paint(theme.peach, bg), inner),
		kv("installed", fmtDate(p.InstallTime)+" · "+relDate(p.InstallTime, time.Now()), text, inner),
		kv("reason", p.Reason+" · "+p.Arch, text, inner),
	)
	if p.URL != "" {
		lines = append(lines, kv("url", p.URL, paint(theme.sapphire, bg), inner))
	}
	return panel("Details", "", w, h, false, lines)
}

// --- overview ------------------------------------------------------------

func (m model) overviewPanel(w, h int) []string {
	bg := theme.base
	inner := w - 2
	total := max(1, m.counts[KindAll])

	barW := inner - 11
	var lines []string
	for k := KindApp; k <= KindPkg; k++ {
		n := m.counts[k]
		name := k.label()
		icon := ""
		if m.lay.icons {
			icon = kindIcon(name) + " "
		}
		label := " " + icon + name
		bar := ""
		if barW >= 4 {
			bar = smoothBar(n, total, barW, kindColor(name))
		}
		lines = append(lines, renderSpans([]span{
			cell(label, paint(kindColor(name), bg), 6),
			cell(" ", canvas(bg), 1),
		})+bar+" "+paint(theme.text, bg).Render(fmt.Sprintf("%3d", n)))
	}

	var size int64
	var latest Package
	for _, p := range m.visible {
		size += p.Size
		if latest.Name == "" || p.InstallTime.After(latest.InstallTime) {
			latest = p
		}
	}
	lines = append(lines, kv("size", humanSize(size)+" shown", paint(theme.peach, bg), inner))
	if latest.Name != "" {
		lines = append(lines, kv("latest", latest.Name+" · "+relDate(latest.InstallTime, time.Now()),
			paint(theme.subtext0, bg), inner))
	}
	return panel("Overview", "", w, h, false, lines)
}

// --- strip (list-only layout) -------------------------------------------

func (m model) stripRow() string {
	l := m.lay
	bg := theme.mantle
	p, ok := m.selected()
	if !ok {
		return blankLine(l.termW)
	}
	line := " " + paint(theme.text, bg).Bold(true).Render(p.Name) +
		paint(theme.overlay1, bg).Render(" "+humanSize(p.Size)+" · ") +
		paint(theme.subtext1, bg).Render(p.Summary)
	return fitLine(line, l.termW, bg)
}

// --- footer --------------------------------------------------------------

type hint struct {
	key  string
	desc string
	on   bool
}

func (m model) footerRow() string {
	l := m.lay
	bg := theme.mantle

	var hints []hint
	switch {
	case m.searching:
		hints = []hint{{"enter", "keep", false}, {"esc", "clear", false}, {"ctrl+u", "wipe", false}}
	case m.help:
		hints = []hint{{"any key", "close", false}, {"q", "quit", false}}
	default:
		// Ordered by usefulness: narrow terminals lose trailing hints.
		hints = []hint{
			{"/", "search", false},
			{"tab", "type", false},
			{"?", "help", false},
			{"s", "sort", false},
			{"a", "all", m.filters.AllUser},
			{"b", "base", m.filters.IncludeBase},
			{"d", "dev", m.filters.ShowDev},
			{"i", "icons", m.icons},
			{"q", "quit", false},
		}
	}

	avail := l.termW - 2
	var b strings.Builder
	used := 0
	for _, h := range hints {
		keyStyle := paint(theme.text, bg).Bold(true)
		if h.on {
			keyStyle = paint(theme.green, bg).Bold(true)
		}
		seg := keyStyle.Render(h.key) + paint(theme.overlay1, bg).Render(" "+h.desc)
		w := ansi.StringWidth(seg)
		if used > 0 {
			w += 2
		}
		if used+w > avail {
			break
		}
		if used > 0 {
			b.WriteString(canvas(bg).Render("  "))
		}
		b.WriteString(seg)
		used += w
	}
	return fitLine(" "+b.String(), l.termW, bg)
}

// --- help ----------------------------------------------------------------

func (m model) helpPanel(h int) []string {
	l := m.lay
	bg := theme.base
	desc := func(s string) string { return paint(theme.overlay1, bg).Render(s) }
	row := func(k, d string) string {
		return " " + renderSpans([]span{cell(k, paint(theme.text, bg).Bold(true), 12)}) + desc(d)
	}

	lines := []string{
		row("↑ ↓  j k", "move"),
		row("g  G", "top / bottom"),
		row("ctrl+u/d", "half page"),
		row("tab  ← →", "next / previous type"),
		row("1 2 3 4", "all / app / cli / pkg"),
		row("/", "search · esc clears"),
		row("s  S", "cycle sort · reverse"),
		row("a", "include non-tool packages"),
		row("b", "include base image"),
		row("d", "include -devel / -doc"),
		row("i", "toggle icons"),
		row("q", "quit"),
	}
	if l.icons {
		lines = append(lines, "",
			" "+paint(theme.overlay0, bg).Render("types       ")+
				paint(theme.app, bg).Render(iconApp)+desc(" app   ")+
				paint(theme.cli, bg).Render(iconCLI)+desc(" cli   ")+
				paint(theme.pkg, bg).Render(iconPkg)+desc(" pkg"))
	}
	return panel("Keys", "", l.termW, h, true, lines)
}

// smoothBar draws a groove with an eighth-block fill, so the bar length has
// 8x the resolution of whole cells.
func smoothBar(n, total, w int, c lipgloss.Color) string {
	track := theme.mantle
	units := n * w * 8 / max(1, total)
	if n > 0 && units == 0 {
		units = 1
	}
	full, rem := units/8, units%8
	if full > w {
		full, rem = w, 0
	}
	var b strings.Builder
	b.WriteString(paint(c, track).Render(strings.Repeat("█", full)))
	used := full
	if rem > 0 && used < w {
		b.WriteString(paint(c, track).Render(string([]rune(" ▏▎▍▌▋▊▉")[rem])))
		used++
	}
	b.WriteString(canvas(track).Render(strings.Repeat(" ", w-used)))
	return b.String()
}
