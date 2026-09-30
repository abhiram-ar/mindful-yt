package ytdlp

import (
	"errors"
	"testing"
)

func TestParseProgress(t *testing.T) {
	p, ok := parseProgress("ytget-progress downloading 140 1048576 NA 4194304 2097152.5 2", []string{"299", "140"})
	want := Progress{Part: 2, Parts: 2, Done: 1048576, Total: 4194304, Speed: 2097152.5, ETA: 2}
	if !ok || p != want {
		t.Errorf("got %+v, %v", p, ok)
	}
	if _, ok := parseProgress("ytget-progress garbage", nil); ok {
		t.Error("garbage was accepted")
	}
}

func TestParseDoneKeepsSpacesInPath(t *testing.T) {
	h, path := parseDone(`ytget-done 1080 C:\Videos\Tom & Jerry [abc] 1080p.mp4`)
	if h != 1080 || path != `C:\Videos\Tom & Jerry [abc] 1080p.mp4` {
		t.Errorf("got %d, %q", h, path)
	}
}

func TestErrorFromPicksLastErrorLine(t *testing.T) {
	stderr := "WARNING: something\nERROR: first\nERROR: [youtube] abc: Video unavailable\n"
	if got := errorFrom(stderr, errors.New("exit 1")).Error(); got != "ERROR: [youtube] abc: Video unavailable" {
		t.Errorf("got %q", got)
	}
	if got := errorFrom("", errors.New("exit 1")).Error(); got != "yt-dlp failed: exit 1" {
		t.Errorf("got %q", got)
	}
}
