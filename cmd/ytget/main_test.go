package main

import "testing"

func TestParseQuality(t *testing.T) {
	for text, want := range map[string]int{"720": 720, "720p": 720, " 1080P ": 1080, "2160": 2160} {
		if got, err := parseQuality(text); err != nil || got != want {
			t.Errorf("%q: got %d, %v", text, got, err)
		}
	}
	for _, text := range []string{"", "hd", "1000", "0", "-720", "4k"} {
		if _, err := parseQuality(text); err == nil {
			t.Errorf("%q was accepted", text)
		}
	}
}
