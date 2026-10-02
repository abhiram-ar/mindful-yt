// Package human formats sizes, durations, counts and how long ago something
// was, for people to read.
package human

import (
	"fmt"
	"strconv"
	"time"
)

// Bytes formats a byte count, e.g. "12.3 MB".
func Bytes(n float64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", n/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", n/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", n/(1<<10))
	}
	return fmt.Sprintf("%.0f B", n)
}

// Duration formats seconds as m:ss or h:mm:ss, and "?" when unknown.
func Duration(seconds float64) string {
	if seconds <= 0 {
		return "?"
	}
	s := int(seconds)
	if h := s / 3600; h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// Count shortens a count the way YouTube does, cutting rather than rounding:
// 1,299 is "1.2K", 15,800 is "15K" and 999,999 is "999K".
func Count(n float64) string {
	c := int64(n)
	var size int64
	var suffix string
	switch {
	case c >= 1e9:
		size, suffix = 1e9, "B"
	case c >= 1e6:
		size, suffix = 1e6, "M"
	case c >= 1e3:
		size, suffix = 1e3, "K"
	default:
		return strconv.FormatInt(c, 10)
	}
	if tenths := c * 10 / size; tenths < 100 && tenths%10 != 0 {
		return fmt.Sprintf("%d.%d%s", tenths/10, tenths%10, suffix)
	}
	return fmt.Sprintf("%d%s", c/size, suffix)
}

// Ago says how long before now t was, in YouTube's words: "3 weeks ago".
//
// It reads back the approximate dates yt-dlp makes from YouTube's own "3
// weeks ago": now minus 3 weeks, rounded to the second, minute or hour for
// those units, and to midnight UTC for days and longer. So t can sit up to
// half a unit either side of the true count, and a count is rounded, not cut:
// "13 years ago" can come back as 12 years and 364 days. Each unit takes over
// half a unit early, and where two could apply, the mark the rounding left on
// t settles it ("1 hour ago" lands on the hour, "1 day ago" on midnight).
//
// now should be yt-dlp's clock when it made the date. A now that's off by δ
// still gives YouTube's words, except when yt-dlp's clock was within δ of a
// half-unit mark: then the count can be one off.
func Ago(t, now time.Time) string {
	const day = 24 * time.Hour
	const year = 31556952 * time.Second // 365.2425 days
	const month = year / 12
	d, at := now.Sub(t), t.Unix()
	on := func(unit time.Duration) bool { return at%int64(unit/time.Second) == 0 }
	switch {
	case d < 30*time.Second || d < time.Minute-time.Second/2 && !on(time.Minute):
		return ago(d, time.Second, "second")
	case d < 30*time.Minute || d < time.Hour-time.Minute/2 && !on(time.Hour):
		return ago(d, time.Minute, "minute")
	case d < 12*time.Hour || d < day-time.Hour/2 && !on(day):
		return ago(d, time.Hour, "hour")
	case d < 7*day-day/2:
		return ago(d, day, "day")
	case d < 4*7*day+day/2: // after 4 weeks, YouTube says "1 month"
		return ago(d, 7*day, "week")
	case d < 12*month-month/2:
		return ago(d, month, "month")
	}
	return ago(d, year, "year")
}

// ago rounds d to a whole number of units, at least 1.
func ago(d, unit time.Duration, name string) string {
	n := max(1, int64((d+unit/2)/unit))
	if n == 1 {
		return "1 " + name + " ago"
	}
	return fmt.Sprintf("%d %ss ago", n, name)
}
