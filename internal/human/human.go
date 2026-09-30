// Package human formats sizes and durations for people to read.
package human

import "fmt"

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
