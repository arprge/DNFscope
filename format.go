package main

import (
	"fmt"
	"time"
)

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.Format("2006-01-02")
}

// relDate keeps the installed column narrow and readable: the absolute date
// lives in the detail panel, the column answers "how long ago" at a glance.
func relDate(t, now time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	days := int(d.Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "1d ago"
	case days < 14:
		return fmt.Sprintf("%dd ago", days)
	case days < 60:
		return fmt.Sprintf("%dw ago", days/7)
	case days < 365:
		return fmt.Sprintf("%dmo ago", days/30)
	default:
		return fmt.Sprintf("%dy ago", days/365)
	}
}
