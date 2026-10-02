package human

import (
	"fmt"
	"testing"
	"time"
)

func TestBytes(t *testing.T) {
	for n, want := range map[float64]string{
		512: "512 B", 2048: "2 KB", 12.3 * (1 << 20): "12.3 MB", 1.5 * (1 << 30): "1.5 GB",
	} {
		if got := Bytes(n); got != want {
			t.Errorf("Bytes(%v) = %q, want %q", n, got, want)
		}
	}
}

func TestDuration(t *testing.T) {
	for s, want := range map[float64]string{0: "?", 19: "0:19", 634: "10:34", 3725: "1:02:05"} {
		if got := Duration(s); got != want {
			t.Errorf("Duration(%v) = %q, want %q", s, got, want)
		}
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[float64]string{
		0: "0", 7: "7", 999: "999",
		1000: "1K", 1050: "1K", 1299: "1.2K", 9999: "9.9K", 15800: "15K", 999999: "999K",
		1e6: "1M", 1.25e6: "1.2M", 12.3e6: "12M", 999.9e6: "999M",
		1e9: "1B", 2.5e9: "2.5B", 16.4e9: "16B",
	} {
		if got := Count(n); got != want {
			t.Errorf("Count(%v) = %q, want %q", n, got, want)
		}
	}
}

func TestAgoBoundaries(t *testing.T) {
	const day = 24 * time.Hour
	now := time.Date(2026, 10, 3, 6, 15, 42, 0, time.UTC)
	midnight := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	hour := time.Date(2026, 10, 3, 6, 0, 0, 0, time.UTC)
	minute := time.Date(2026, 10, 3, 6, 15, 0, 0, time.UTC)
	for _, c := range []struct {
		name   string
		t, now time.Time
		want   string
	}{
		{"13 years, rounded up to the next midnight", time.Date(2013, 10, 4, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC), "13 years ago"},
		{"just under 6.5 days", now.Add(-(6*day + 12*time.Hour - time.Second)), now, "6 days ago"},
		{"6.5 days", now.Add(-(6*day + 12*time.Hour)), now, "1 week ago"},
		{"27 days 15 hours", now.Add(-(27*day + 15*time.Hour)), now, "4 weeks ago"},
		{"30 days 15 hours", now.Add(-(30*day + 15*time.Hour)), now, "1 month ago"},
		{"333 days 15 hours", now.Add(-(333*day + 15*time.Hour)), now, "11 months ago"},
		{"364 days 15 hours", now.Add(-(364*day + 15*time.Hour)), now, "1 year ago"},
		{"15 hours, landing on midnight", midnight, midnight.Add(15 * time.Hour), "1 day ago"},
		{"13 hours, on the hour but not midnight", hour.Add(-13 * time.Hour), hour.Add(4 * time.Minute), "13 hours ago"},
		{"44 minutes, landing on the hour", hour, hour.Add(44 * time.Minute), "1 hour ago"},
		{"45 minutes, off the hour", now.Add(-45 * time.Minute), now, "45 minutes ago"},
		{"40 seconds, landing on the minute", minute, minute.Add(40 * time.Second), "1 minute ago"},
		{"40 seconds, off the minute", now.Add(-40 * time.Second), now, "40 seconds ago"},
		{"exactly a day, not rounded", now.Add(-day), now, "1 day ago"},
		{"in the future", now.Add(time.Minute), now, "1 second ago"},
	} {
		if got := Ago(c.t, c.now); got != c.want {
			t.Errorf("%s: Ago = %q, want %q", c.name, got, c.want)
		}
	}
}

// ytdlpDate is what yt-dlp makes of YouTube's "n units ago" at now
// (datetime_from_str with precision "auto"): now minus n units, rounded half
// up to the second, minute or hour for those units and to the day for days
// and longer. Months and years go back in calendar months, keeping the time
// of day and clamping the day to the month's length.
func ytdlpDate(now time.Time, n int, unit string) time.Time {
	round := func(t time.Time, u time.Duration) time.Time { return t.Add(u / 2).Truncate(u) }
	const day = 24 * time.Hour
	switch unit {
	case "second":
		return round(now.Add(-time.Duration(n)*time.Second), time.Second)
	case "minute":
		return round(now.Add(-time.Duration(n)*time.Minute), time.Minute)
	case "hour":
		return round(now.Add(-time.Duration(n)*time.Hour), time.Hour)
	case "day":
		return round(now.Add(-time.Duration(n)*day), day)
	case "week":
		return round(now.Add(-time.Duration(n)*7*day), day)
	}
	months := n
	if unit == "year" {
		months = 12 * n
	}
	m := int(now.Month()) - 1 - months
	year := now.Year() + m/12
	if m%12 < 0 {
		year--
	}
	month := time.Month((m%12+12)%12 + 1)
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	back := time.Date(year, month, min(now.Day(), lastDay),
		now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
	return round(back, day)
}

func TestAgoReadsBackYtdlpDates(t *testing.T) {
	counts := map[string]int{"second": 59, "minute": 59, "hour": 23, "day": 6, "week": 4, "month": 11, "year": 20}
	nows := []time.Time{
		time.Date(2026, 10, 3, 6, 15, 42, 0, time.UTC),
		time.Date(2026, 3, 31, 11, 59, 59, 0, time.UTC),
		time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC),
		time.Date(2027, 3, 15, 0, 0, 1, 0, time.UTC),
		time.Date(2028, 2, 29, 23, 59, 59, 0, time.UTC),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 31, 18, 30, 12, 345678000, time.UTC),
	}
	for _, now := range nows {
		for unit, most := range counts {
			for n := 1; n <= most; n++ {
				ts := ytdlpDate(now, n, unit)
				got := Ago(ts, now)
				if got == want(n, unit) || misreadAllowed(got, n, unit, ts, now) {
					continue
				}
				t.Errorf("now %s, %s: yt-dlp's date %s reads as %q", now.Format(time.RFC3339), want(n, unit), ts.Format(time.RFC3339), got)
			}
		}
	}
}

func want(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}

// misreadAllowed lists the only ways Ago may differ from what YouTube said:
//   - "1 month ago" from a February or other short month, under 28.5 days
//     back, reads as "4 weeks ago".
//   - Half a unit or more of seconds, minutes or hours whose rounded date
//     happens to land on the next unit's mark (a minute, an hour, midnight)
//     reads as 1 of that unit. It happens once in 60 or 24.
func misreadAllowed(got string, n int, unit string, ts, now time.Time) bool {
	on := func(u time.Duration) bool { return ts.Unix()%int64(u/time.Second) == 0 }
	switch {
	case unit == "month" && n == 1 && now.Sub(ts) < 28*24*time.Hour+12*time.Hour:
		return got == "4 weeks ago"
	case unit == "second" && n >= 30 && on(time.Minute):
		return got == "1 minute ago"
	case unit == "minute" && n >= 30 && on(time.Hour):
		return got == "1 hour ago"
	case unit == "hour" && n >= 12 && on(24*time.Hour):
		return got == "1 day ago"
	}
	return false
}
