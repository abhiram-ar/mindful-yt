package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/abhiram-ar/mindful-yt/internal/store"
	"github.com/abhiram-ar/mindful-yt/internal/ytdlp"
)

const testID = "jNQXAC9IVRw"

func testApp(t *testing.T, cfg store.Config, entries []store.Entry) *App {
	t.Helper()
	dir := t.TempDir()
	return &App{
		Ctx: context.Background(), Store: store.Store{Dir: dir}, Config: cfg, Entries: entries,
		Tools: filepath.Join(dir, "tools"), ProxyURL: "http://mindful-yt:x@127.0.0.1:1",
	}
}

// update feeds msg to the model and returns the new model.
func update(m model, msg tea.Msg) model {
	next, _ := m.Update(msg)
	return next.(model)
}

func TestFinishedStreamStaysGreenAndTheNextAppearsBelow(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{})
	m.stage, m.taskbar = stageDownloading, true
	m.info = ytdlp.VideoInfo{Title: "Big Buck Bunny", Formats: []ytdlp.Format{
		{ID: "137", VCodec: "avc1.640028", ACodec: "none", Width: 1920, Height: 1080, Filesize: 100 << 20},
		{ID: "140", VCodec: "none", ACodec: "mp4a.40.2", Ext: "m4a", Filesize: 4 << 20},
	}}
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(m, ytdlp.Selected{Formats: []string{"137", "140"}})
	if view := m.View().Content; !strings.Contains(view, "Starting the download") || strings.Contains(view, "Total") {
		t.Errorf("before any progress, want only a starting line:\n%s", view)
	}
	m = update(m, ytdlp.Progress{Part: 1, Parts: 2, FormatID: "137", Done: 50 << 20, Total: 100 << 20})

	lines := strings.Split(m.View().Content, "\n")
	video, audio := lineWith(lines, "Video 1080p"), lineWith(lines, "Audio")
	if video < 0 || audio >= 0 {
		t.Fatalf("while the video downloads, want only the video row:\n%s", m.View().Content)
	}
	if strings.Contains(lines[video], "38;5;42") {
		t.Error("the unfinished video bar is already green")
	}

	m = update(m, ytdlp.Progress{Part: 1, Parts: 2, FormatID: "137", Done: 100 << 20, Total: 100 << 20, Finished: true})
	m = update(m, ytdlp.Progress{Part: 2, Parts: 2, FormatID: "140", Done: 1 << 20, Total: 4 << 20})
	lines = strings.Split(m.View().Content, "\n")
	video, audio = lineWith(lines, "Video 1080p"), lineWith(lines, "Audio")
	if video < 0 || audio < 0 || video > audio {
		t.Fatalf("want the video row, then audio below it:\n%s", m.View().Content)
	}
	if lineWith(lines, "Total") >= 0 {
		t.Errorf("there should be no total row:\n%s", m.View().Content)
	}
	if !strings.Contains(lines[video], "38;5;42") || !strings.Contains(lines[video], "100%") {
		t.Errorf("the finished video row isn't a full green bar: %q", lines[video])
	}
	if strings.Contains(lines[audio], "38;5;42") {
		t.Error("the audio bar is green while it's still downloading")
	}
	for _, line := range lines {
		if w := ansi.StringWidth(line); w > 99 {
			t.Errorf("line is %d columns in a 100-column terminal: %q", w, line)
		}
	}
	if pb := m.View().ProgressBar; pb == nil || pb.Value != 97 { // 101 of 104 MiB
		t.Errorf("taskbar progress: %+v", pb)
	}
}

func TestNarrowTerminalLinesStayInside(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{})
	m.stage = stageDownloading
	m.info = ytdlp.VideoInfo{Title: "A title much longer than thirty columns", Formats: []ytdlp.Format{
		{ID: "18", VCodec: "avc1", ACodec: "mp4a.40.2", Width: 640, Height: 360, Filesize: 20 << 20},
	}}
	m = update(m, tea.WindowSizeMsg{Width: 30, Height: 20})
	m = update(m, ytdlp.Selected{Formats: []string{"18"}})
	m = update(m, ytdlp.Progress{Part: 1, Parts: 1, FormatID: "18", Done: 5 << 20, Total: 20 << 20, Speed: 3 << 20, ETA: 5})
	for _, line := range strings.Split(m.View().Content, "\n") {
		if w := ansi.StringWidth(line); w > 29 {
			t.Errorf("line is %d columns in a 30-column terminal: %q", w, ansi.Strip(line))
		}
	}
}

func TestTaskbarProgressOnlyInTerminalsThatDrawIt(t *testing.T) {
	env := func(pairs ...string) func(string) string {
		vars := map[string]string{}
		for i := 0; i+1 < len(pairs); i += 2 {
			vars[pairs[i]] = pairs[i+1]
		}
		return func(k string) string { return vars[k] }
	}
	for _, c := range []struct {
		name string
		env  func(string) string
		want bool
	}{
		{"Windows Terminal", env("WT_SESSION", "0f1c9e1a"), true},
		{"ConEmu", env("ConEmuANSI", "ON"), true},
		{"Ghostty 1.2", env("TERM_PROGRAM", "ghostty", "TERM_PROGRAM_VERSION", "1.2.0"), true},
		{"old Ghostty", env("TERM_PROGRAM", "ghostty", "TERM_PROGRAM_VERSION", "1.1.3"), false},
		{"iTerm2 3.6.6", env("TERM_PROGRAM", "iTerm.app", "TERM_PROGRAM_VERSION", "3.6.6"), true},
		{"iTerm2 3.6.10", env("TERM_PROGRAM", "iTerm.app", "TERM_PROGRAM_VERSION", "3.6.10"), true},
		{"iTerm2 3.5 (notification spam)", env("TERM_PROGRAM", "iTerm.app", "TERM_PROGRAM_VERSION", "3.5.14"), false},
		{"VS Code", env("TERM_PROGRAM", "vscode"), false},
		{"unknown", env(), false},
	} {
		if got := termProgress(c.env); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func lineWith(lines []string, text string) int {
	for i, line := range lines {
		if strings.Contains(ansi.Strip(line), text) {
			return i
		}
	}
	return -1
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

func TestHelpLineListsEachScreensKeys(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{})
	for _, c := range []struct {
		stage stage
		want  string
	}{
		{stageLink, "enter continue · esc quit"},
		{stageListing, "esc back"},
		{stageResults, "↑/↓ choose · enter select · esc back · q quit"},
		{stagePick, "↑/↓ choose · enter select · esc quit"},
		{stageDeps, "enter install · esc quit"},
		{stageReason, "enter start download · esc quit"},
		{stageDownloading, "esc cancel"},
		{stageDone, "enter play · o open folder · q quit"},
	} {
		m.stage = c.stage
		m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, c.want) {
			t.Errorf("stage %v: no %q in\n%s", c.stage, c.want, view)
		}
		m = update(m, tea.WindowSizeMsg{Width: 30, Height: 30})
		help := ansi.Strip(m.View().Content)
		help = help[strings.LastIndex(strings.TrimSuffix(help, "\n"), "\n")+1:]
		if w := ansi.StringWidth(strings.TrimSuffix(help, "\n")); w > 29 || !strings.HasPrefix(help, c.want[:3]) {
			t.Errorf("stage %v: help line %q is %d columns in a 30-column terminal", c.stage, help, w)
		}
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
