package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/store"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/ytdlp"
)

const testID = "jNQXAC9IVRw"

func testApp(t *testing.T, cfg store.Config, entries []store.Entry) *App {
	t.Helper()
	dir := t.TempDir()
	return &App{
		Ctx: context.Background(), Store: store.Store{Dir: dir}, Config: cfg, Entries: entries,
		Ytdlp: filepath.Join(dir, "yt-dlp.exe"), ProxyURL: "http://ytget:x@127.0.0.1:1",
	}
}

func press(m model, key rune) model {
	next, _ := m.Update(tea.KeyPressMsg{Code: key})
	return next.(model)
}

func TestBadLinkStaysOnTheLinkScreen(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{URL: "https://www.youtube.com/@someone"})
	if m.stage != stageLink || !strings.Contains(m.problem, "single video") || m.quitting {
		t.Fatalf("stage %v, problem %q", m.stage, m.problem)
	}
}

func TestDailyLimitStopsBeforeAnythingElse(t *testing.T) {
	cfg := store.DefaultConfig
	cfg.DailyLimit = 1
	m := newModel(testApp(t, cfg, []store.Entry{{Date: store.Today(), VideoID: "aaaaaaaaaaa"}}),
		Options{URL: "https://youtu.be/" + testID})
	if !m.quitting || m.exitCode != 1 || !strings.Contains(m.final, "Daily limit reached (1/1)") {
		t.Fatalf("quitting %v, code %d, final %q", m.quitting, m.exitCode, m.final)
	}
}

func TestLookupResultOpensPickerAtConfiguredDefault(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{URL: "https://youtu.be/" + testID})
	if m.stage != stageBusy {
		t.Fatalf("stage %v, want the tool check", m.stage)
	}
	video := func(h float64) ytdlp.Format {
		return ytdlp.Format{VCodec: "avc1", ACodec: "none", Width: h * 16 / 9, Height: h}
	}
	info := ytdlp.VideoInfo{Title: "T", Formats: []ytdlp.Format{video(2160), video(1080), video(720)}}
	next, _ := m.Update(probeDoneMsg{info: info})
	m = next.(model)
	if m.stage != stagePick || m.qualities[m.cursor].Res != 1080 {
		t.Fatalf("stage %v, cursor on %v", m.stage, m.qualities[m.cursor])
	}
	m = press(m, tea.KeyDown)
	m = press(m, tea.KeyEnter)
	if m.stage != stageReason || m.quality != 720 {
		t.Fatalf("stage %v, quality %d", m.stage, m.quality)
	}
}

func TestShortReasonIsRefused(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{})
	m.stage = stageReason
	m.reason.SetValue("meh")
	m = press(m, tea.KeyEnter)
	if m.stage != stageReason || !strings.Contains(m.problem, "at least 10") {
		t.Fatalf("stage %v, problem %q", m.stage, m.problem)
	}
}

func TestFinishedDownloadIsLogged(t *testing.T) {
	app := testApp(t, store.DefaultConfig, nil)
	m := newModel(app, Options{})
	m.videoID, m.quality, m.reasonText = testID, 1080, "a friend sent it"
	m.info = ytdlp.VideoInfo{Title: "Me at the zoo", Channel: "jawed", Duration: 19}
	next, _ := m.Update(ytdlp.Result{Path: `C:\Videos\zoo.mp4`, Height: 240})
	m = next.(model)
	if m.stage != stageDone {
		t.Fatalf("stage %v, final %q", m.stage, m.final)
	}
	entries, _ := app.Store.ReadHistory()
	if len(entries) != 1 || entries[0].Reason != "a friend sent it" || entries[0].Quality != 1080 ||
		entries[0].Height != 240 || entries[0].Date != store.Today() {
		t.Fatalf("history: %+v", entries)
	}
	if m.usedToday() != 1 {
		t.Errorf("today's count wasn't updated")
	}
}
