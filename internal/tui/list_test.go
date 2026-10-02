package tui

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/abhiram-ar/mindful-yt/internal/deps"
	"github.com/abhiram-ar/mindful-yt/internal/store"
	"github.com/abhiram-ar/mindful-yt/internal/ytdlp"
)

var qKey = tea.KeyPressMsg{Code: 'q', Text: "q"}

// searching is a model whose search for query has passed the tool check, so
// yt-dlp is listing.
func searching(t *testing.T, app *App, query string) model {
	t.Helper()
	m := newModel(app, Options{URL: query})
	m = update(m, depsCheckedMsg{})
	if m.stage != stageListing {
		t.Fatalf("stage %v, problem %q, final %q", m.stage, m.problem, m.final)
	}
	return m
}

// testVideos are n search results, the first one "Me at the zoo".
func testVideos(n int, at time.Time) []ytdlp.ListEntry {
	videos := []ytdlp.ListEntry{{
		ID: testID, URL: "https://www.youtube.com/watch?v=" + testID, Title: "Me at the zoo", Channel: "jawed", UploaderID: "@jawed",
		Duration: 19, Views: 1_234_567, Timestamp: float64(at.Add(-21 * 24 * time.Hour).Unix()),
	}}
	for i := 1; i < n; i++ {
		id := fmt.Sprintf("video%06d", i)
		videos = append(videos, ytdlp.ListEntry{
			ID: id, URL: "https://www.youtube.com/watch?v=" + id, Title: fmt.Sprintf("Video number %d", i),
			Channel: "Someone", Duration: 600, Views: 1000,
		})
	}
	return videos
}

// listedWith answers m's search with videos.
func listedWith(m model, videos []ytdlp.ListEntry, at time.Time) model {
	return update(m, listedMsg{seq: m.listings, videos: videos, at: at})
}

func TestWordsAreSearchedAfterTheToolCheck(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{URL: "  lofi hip hop "})
	if m.stage != stageBusy || m.busyLabel != "Checking tools..." || m.link.Value() != "lofi hip hop" || m.watchURL != "" {
		t.Fatalf("stage %v %q, box %q, watch %q", m.stage, m.busyLabel, m.link.Value(), m.watchURL)
	}
	m = update(m, depsCheckedMsg{})
	if m.stage != stageListing || m.list.target != "ytsearch20:lofi hip hop" || m.cancel == nil {
		t.Fatalf("stage %v, target %q, cancel set %v", m.stage, m.list.target, m.cancel != nil)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, `Searching YouTube for "lofi hip hop"...`) {
		t.Errorf("no search line:\n%s", view)
	}
}

func TestEmptyInputAsksForALinkOrSearch(t *testing.T) {
	m := press(newModel(testApp(t, store.DefaultConfig, nil), Options{}), tea.KeyEnter)
	if m.stage != stageLink || !strings.Contains(m.problem, "search") || m.quitting {
		t.Fatalf("stage %v, problem %q, quitting %v", m.stage, m.problem, m.quitting)
	}
}

func TestSearchRefusedOnceTheDailyLimitIsUsedUp(t *testing.T) {
	cfg := store.DefaultConfig
	cfg.DailyLimit = 1
	m := newModel(testApp(t, cfg, []store.Entry{{Date: store.Today(), VideoID: "aaaaaaaaaaa"}}), Options{URL: "cats"})
	if !m.quitting || m.exitCode != 1 || !strings.Contains(m.final, "Daily limit reached (1/1)") {
		t.Fatalf("quitting %v, code %d, final %q", m.quitting, m.exitCode, m.final)
	}
}

func TestMissingToolsThenTheSearchRuns(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{URL: "cats"})
	m = update(m, depsCheckedMsg{missing: []deps.Dependency{{Name: "yt-dlp", Why: "it's the downloader itself"}}})
	if m.stage != stageDeps {
		t.Fatalf("stage %v, want the tools screen", m.stage)
	}
	if m = update(m, depsCheckedMsg{}); m.stage != stageListing {
		t.Fatalf("stage %v after the tools arrived, want the search", m.stage)
	}
}

func TestResultsKeepYouTubesOrderWithTheirDetails(t *testing.T) {
	at := time.Date(2020, 1, 1, 6, 15, 42, 0, time.UTC) // far from today: "ago" counts from the list, not the clock
	m := searching(t, testApp(t, store.DefaultConfig, nil), "zoo")
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = listedWith(m, testVideos(3, at), at)
	if m.stage != stageResults || m.cancel != nil {
		t.Fatalf("stage %v, cancel still set %v", m.stage, m.cancel != nil)
	}
	lines := strings.Split(m.View().Content, "\n")
	heading, zoo, one, two := lineWith(lines, `Results for "zoo":`), lineWith(lines, "Me at the zoo"),
		lineWith(lines, "Video number 1"), lineWith(lines, "Video number 2")
	if heading < 0 || !(heading < zoo && zoo < one && one < two) {
		t.Fatalf("want the heading, then the videos in order:\n%s", ansi.Strip(m.View().Content))
	}
	if !strings.HasPrefix(ansi.Strip(lines[zoo]), "│ ") {
		t.Errorf("the cursor isn't on the first video: %q", ansi.Strip(lines[zoo]))
	}
	if got := rowText(lines[zoo+1]); got != "@jawed · 1.2M views · 3 weeks ago · 0:19" {
		t.Errorf("details line %q", got)
	}
	if got := rowText(lines[one+1]); got != "Someone · 1K views · 10:00" {
		t.Errorf("a video with no date: %q", got)
	}
	for _, line := range lines {
		if w := ansi.StringWidth(line); w > 99 {
			t.Errorf("line is %d columns in a 100-column terminal: %q", w, ansi.Strip(line))
		}
	}
}

func TestPickingAResultLooksItUp(t *testing.T) {
	at := time.Now()
	m := listedWith(searching(t, testApp(t, store.DefaultConfig, nil), "cats"), testVideos(3, at), at)
	m = press(m, tea.KeyDown)
	m = press(m, tea.KeyEnter)
	if m.stage != stageBusy || m.busyLabel != "Checking tools..." || m.videoID != "video000001" {
		t.Fatalf("stage %v %q, video %q", m.stage, m.busyLabel, m.videoID)
	}
	if m = update(m, depsCheckedMsg{}); m.stage != stageBusy || m.busyLabel != "Looking up the video..." {
		t.Fatalf("stage %v %q, want the lookup", m.stage, m.busyLabel)
	}
}

func TestPickingASavedResultOffersTheCopy(t *testing.T) {
	file := filepath.Join(t.TempDir(), "zoo.mp4")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := testApp(t, store.DefaultConfig, []store.Entry{{Date: "2026-01-01", VideoID: testID, Quality: 240, File: file}})
	at := time.Now()
	m := listedWith(searching(t, app, "zoo"), testVideos(3, at), at)
	if m = press(m, tea.KeyEnter); m.stage != stageSaved {
		t.Fatalf("stage %v, want the saved copy offered", m.stage)
	}
}

func TestEscOnResultsGoesBackWithTheQuery(t *testing.T) {
	at := time.Now()
	m := listedWith(searching(t, testApp(t, store.DefaultConfig, nil), "cats"), testVideos(3, at), at)
	m = update(m, escKey)
	if m.stage != stageLink || m.link.Value() != "cats" || !m.link.Focused() || m.problem != "" || m.quitting {
		t.Fatalf("stage %v, box %q focused %v, problem %q", m.stage, m.link.Value(), m.link.Focused(), m.problem)
	}
}

func TestQOnResultsQuits(t *testing.T) {
	at := time.Now()
	m := listedWith(searching(t, testApp(t, store.DefaultConfig, nil), "cats"), testVideos(3, at), at)
	if m = update(m, qKey); !m.quitting || m.exitCode != 0 || m.final != "" {
		t.Fatalf("quitting %v, code %d, final %q", m.quitting, m.exitCode, m.final)
	}
}

func TestEscCancelsASearchAndItsLateAnswerIsIgnored(t *testing.T) {
	at := time.Now()
	m := searching(t, testApp(t, store.DefaultConfig, nil), "cats")
	cancelled := false
	cancel := m.cancel
	m.cancel = func() { cancelled = true; cancel() }
	old := m.listings

	m = update(m, escKey)
	if !cancelled || m.stage != stageLink || m.cancel != nil {
		t.Fatalf("cancelled %v, stage %v", cancelled, m.stage)
	}
	if m = update(m, listedMsg{seq: old, videos: testVideos(3, at), at: at}); m.stage != stageLink {
		t.Fatalf("a late answer moved the screen to %v", m.stage)
	}

	m = press(m, tea.KeyEnter) // the same search again
	m = update(m, depsCheckedMsg{})
	if m.stage != stageListing || m.listings == old {
		t.Fatalf("stage %v, fetch %d", m.stage, m.listings)
	}
	if m = update(m, listedMsg{seq: old, videos: testVideos(3, at), at: at}); m.stage != stageListing {
		t.Fatalf("the first search's answer was taken for the second's: stage %v", m.stage)
	}
	if m = listedWith(m, testVideos(3, at), at); m.stage != stageResults {
		t.Fatalf("stage %v, want the results", m.stage)
	}
}

func TestFailedOrEmptySearchGoesBackToTheBox(t *testing.T) {
	app := testApp(t, store.DefaultConfig, nil)
	m := searching(t, app, "cats")
	m = update(m, listedMsg{seq: m.listings, err: fmt.Errorf("ERROR: HTTP Error 429: Too Many Requests")})
	if m.stage != stageLink || m.problem != "Couldn't search YouTube: ERROR: HTTP Error 429: Too Many Requests" ||
		m.link.Value() != "cats" || m.quitting {
		t.Fatalf("stage %v, problem %q, box %q", m.stage, m.problem, m.link.Value())
	}
	m = searching(t, app, "qwzxqwzx")
	if m = update(m, listedMsg{seq: m.listings}); m.stage != stageLink || m.problem != `No videos found for "qwzxqwzx".` {
		t.Fatalf("stage %v, problem %q", m.stage, m.problem)
	}
}

func TestResultsFitTheTerminal(t *testing.T) {
	at := time.Now()
	videos := testVideos(10, at)
	for i := range videos {
		videos[i].Title = strings.Repeat("강남스타일 a very long title ", 6)
	}
	query := strings.Repeat("a long search ", 8)
	for _, c := range []struct {
		width, height int
		perPage       int // 10: no pages
	}{
		{100, 40, 10}, {80, 36, 10}, {80, 35, 9}, {80, 30, 7}, {80, 24, 5}, {60, 14, 2},
		{30, 12, 1}, {40, 6, 1}, {0, 0, 5},
	} {
		m := searching(t, testApp(t, store.DefaultConfig, nil), query)
		if c.width > 0 {
			m = update(m, tea.WindowSizeMsg{Width: c.width, Height: c.height})
		}
		width, height := cmp.Or(c.width, 80), cmp.Or(c.height, 24)
		for _, line := range strings.Split(m.View().Content, "\n") { // the spinner
			if w := ansi.StringWidth(line); w > width-1 {
				t.Errorf("%dx%d: searching, a line is %d columns: %q", c.width, c.height, w, ansi.Strip(line))
			}
		}
		m = listedWith(m, videos, at)
		view := m.View().Content
		// Below 10 rows even one video and its page dots don't fit; it's shown anyway.
		if rows := strings.Count(view, "\n") + 1; rows > height && height >= 10 {
			t.Errorf("%dx%d: the frame is %d rows:\n%s", c.width, c.height, rows, ansi.Strip(view))
		}
		for _, line := range strings.Split(view, "\n") {
			if w := ansi.StringWidth(line); w > width-1 {
				t.Errorf("%dx%d: line is %d columns: %q", c.width, c.height, w, ansi.Strip(line))
			}
		}
		plain := ansi.Strip(view)
		shown := 0
		for _, line := range strings.Split(plain, "\n") {
			if strings.Contains(line, "강남스타일") {
				shown++
			}
		}
		if shown != c.perPage {
			t.Errorf("%dx%d: %d videos shown, want %d:\n%s", c.width, c.height, shown, c.perPage, plain)
		}
		paged := c.perPage < 10
		if strings.Contains(plain, "•") != paged || strings.Contains(plain, "←/→ page") != paged {
			t.Errorf("%dx%d: page dots and the page key should show only when paged:\n%s", c.width, c.height, plain)
		}
	}

	m := searching(t, testApp(t, store.DefaultConfig, nil), "cats")
	m = update(m, tea.WindowSizeMsg{Width: 60, Height: 14})
	m = listedWith(m, testVideos(10, at), at)
	for range 9 {
		m = press(m, tea.KeyDown)
	}
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if last := lineWith(lines, "Video number 9"); last < 0 || !strings.HasPrefix(lines[last], "│ ") {
		t.Errorf("the cursor's video isn't shown with the cursor:\n%s", strings.Join(lines, "\n"))
	}
	if lineWith(lines, "Me at the zoo") >= 0 {
		t.Errorf("the first video still shows at the end of the list:\n%s", strings.Join(lines, "\n"))
	}
}

func TestControlCharactersInTitlesStayOnOneLine(t *testing.T) {
	at := time.Now()
	videos := testVideos(2, at)
	m := searching(t, testApp(t, store.DefaultConfig, nil), "cats")
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	plain := strings.Count(listedWith(m, videos, at).View().Content, "\n")
	videos[0].Title, videos[0].Channel = "a\nb\x1b[31mc\td", "x\ny\x1b[32mz\tw"
	view := listedWith(m, videos, at).View().Content
	if strings.Contains(view, "\x1b[31m") || strings.Contains(view, "\x1b[32m") || strings.Contains(view, "\t") ||
		strings.Count(view, "\n") != plain {
		t.Errorf("a title's control characters reached the screen:\n%q", view)
	}
}

// rowText is a list row's text, without the cursor's bar or the indent.
func rowText(line string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ansi.Strip(line)), "│"))
}

// The list's own keys must not end the program behind mindful-yt's back:
// its quit and filter keys are off, and q and esc are mindful-yt's.
func TestTheListsOwnKeysAreOff(t *testing.T) {
	at := time.Now()
	m := listedWith(searching(t, testApp(t, store.DefaultConfig, nil), "cats"), testVideos(10, at), at)
	for _, k := range []tea.KeyPressMsg{
		{Code: '/', Text: "/"}, {Code: '?', Text: "?"}, {Code: 'c', Mod: tea.ModCtrl},
	} {
		next, cmd := m.Update(k)
		if got := next.(model); k.Code != 'c' && (got.stage != stageResults || cmd != nil) {
			t.Errorf("%v: stage %v, cmd %v", k, got.stage, cmd != nil)
		}
	}
	if m.results.FilteringEnabled() || m.results.ShowHelp() {
		t.Error("the list's filter or help is on")
	}
}

func TestPagingKeysMoveThroughTheList(t *testing.T) {
	at := time.Now()
	m := searching(t, testApp(t, store.DefaultConfig, nil), "cats")
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24}) // 5 a page
	m = listedWith(m, testVideos(10, at), at)
	if m = press(m, tea.KeyRight); m.results.Index() != 5 {
		t.Errorf("→ went to video %d, want 5", m.results.Index())
	}
	if m = press(m, tea.KeyUp); m.results.Index() != 4 {
		t.Errorf("↑ from the top of page 2 went to %d, want 4", m.results.Index())
	}
	m = press(m, tea.KeyEnter)
	if m.videoID != "video000004" {
		t.Errorf("picked %q", m.videoID)
	}
}

func TestResizingRefitsTheList(t *testing.T) {
	at := time.Now()
	m := searching(t, testApp(t, store.DefaultConfig, nil), "cats")
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 14})
	m = listedWith(m, testVideos(10, at), at)
	if m.results.Paginator.PerPage != 2 {
		t.Fatalf("%d a page at 14 rows", m.results.Paginator.PerPage)
	}
	if m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40}); m.results.Paginator.TotalPages != 1 {
		t.Errorf("still %d pages at 40 rows", m.results.Paginator.TotalPages)
	}
	if rows := strings.Count(m.View().Content, "\n") + 1; rows > 40 {
		t.Errorf("%d rows at 40", rows)
	}
}

func TestANewListStartsAtTheTop(t *testing.T) {
	at := time.Now()
	m := listedWith(searching(t, testApp(t, store.DefaultConfig, nil), "cats"), testVideos(10, at), at)
	for range 5 {
		m = press(m, tea.KeyDown)
	}
	m = update(m, escKey)
	m = press(m, tea.KeyEnter)
	m = update(m, depsCheckedMsg{})
	if m = listedWith(m, testVideos(3, at), at); m.results.Index() != 0 {
		t.Fatalf("cursor %d on a new list of 3", m.results.Index())
	}
	if m = press(m, tea.KeyEnter); m.videoID != testID {
		t.Errorf("picked %q, want the first video", m.videoID)
	}
}

func TestCheckOnlyQuitsInsteadOfSearching(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{CheckOnly: true})
	if m = update(m, depsCheckedMsg{summary: "yt-dlp   2026.08.19"}); !m.quitting || m.exitCode != 0 || m.stage == stageListing {
		t.Fatalf("stage %v, quitting %v, code %d", m.stage, m.quitting, m.exitCode)
	}
}

// Esc on the spinner must stop yt-dlp, not just leave its answer unread.
func TestEscStopsTheSearch(t *testing.T) {
	stopped := make(chan struct{})
	listVideos = func(ctx context.Context, _, _, _ string, _ int) ([]ytdlp.ListEntry, time.Time, error) {
		<-ctx.Done()
		close(stopped)
		return nil, time.Time{}, ctx.Err()
	}
	t.Cleanup(func() { listVideos = ytdlp.List })

	next, cmd := newModel(testApp(t, store.DefaultConfig, nil), Options{URL: "cats"}).Update(depsCheckedMsg{})
	m := next.(model)
	if m.stage != stageListing || cmd == nil {
		t.Fatalf("stage %v", m.stage)
	}
	go cmd()
	m = update(m, escKey)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("esc left the search running")
	}
	if m.stage != stageLink {
		t.Errorf("stage %v after esc", m.stage)
	}
}

func TestRetryingAFailedSearchClearsTheError(t *testing.T) {
	m := searching(t, testApp(t, store.DefaultConfig, nil), "cats")
	m = update(m, listedMsg{seq: m.listings, err: fmt.Errorf("ERROR: HTTP Error 429: Too Many Requests")})
	m = press(m, tea.KeyEnter)
	if m = update(m, depsCheckedMsg{}); m.stage != stageListing || m.problem != "" {
		t.Fatalf("stage %v, problem %q", m.stage, m.problem)
	}
}

func TestACommandLineSearchIsWhatTheBoxShows(t *testing.T) {
	m := searching(t, testApp(t, store.DefaultConfig, nil), "lofi\nhip\x1b[31m hop")
	if strings.ContainsFunc(m.list.target, func(r rune) bool { return r < ' ' }) || m.list.target != "ytsearch20:"+m.link.Value() {
		t.Errorf("searched %q for a box showing %q", m.list.target, m.link.Value())
	}
	m = searching(t, testApp(t, store.DefaultConfig, nil), strings.Repeat("a", 600))
	if len(m.link.Value()) != 500 || m.list.target != "ytsearch20:"+m.link.Value() {
		t.Errorf("searched %d characters for a box showing %d", len(m.list.target)-len("ytsearch20:"), len(m.link.Value()))
	}
}

// A problem line goes once a link is taken, even if it was pasted, which
// doesn't count as typing.
func TestAnAcceptedLinkClearsTheProblem(t *testing.T) {
	m := press(newModel(testApp(t, store.DefaultConfig, nil), Options{}), tea.KeyEnter)
	if m.problem == "" {
		t.Fatal("no problem for an empty box")
	}
	m = update(m, tea.PasteMsg{Content: "https://youtu.be/" + testID})
	if m = press(m, tea.KeyEnter); m.stage != stageBusy || m.problem != "" {
		t.Fatalf("stage %v, problem %q", m.stage, m.problem)
	}
}

func TestDetailsLeaveOutWhatYouTubeDidntSay(t *testing.T) {
	at := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		v    ytdlp.ListEntry
		want string
	}{
		{ytdlp.ListEntry{Channel: "jawed", UploaderID: "@jawed", Views: 2, Timestamp: float64(at.Add(-48 * time.Hour).Unix()), Duration: 19},
			"@jawed · 2 views · 2 days ago · 0:19"},
		{ytdlp.ListEntry{Channel: "Lofi Girl", Views: 2}, "Lofi Girl · 2 views"},
		{ytdlp.ListEntry{Channel: "Tiny", Views: 1}, "Tiny · 1 view"},
		{ytdlp.ListEntry{Uploader: "shortcat", Views: 0, Duration: 0}, "shortcat"},
		{ytdlp.ListEntry{Views: 15800}, "15K views"},
		{ytdlp.ListEntry{}, ""},
	} {
		if got := details(c.v, at, true); got != c.want {
			t.Errorf("%+v: %q, want %q", c.v, got, c.want)
		}
	}
	m := searching(t, testApp(t, store.DefaultConfig, nil), "cats")
	m = listedWith(m, []ytdlp.ListEntry{{ID: testID, URL: "https://youtu.be/" + testID}}, at)
	if !strings.Contains(ansi.Strip(m.View().Content), "│ Untitled video") {
		t.Errorf("no stand-in title:\n%s", ansi.Strip(m.View().Content))
	}
}

func TestSearchResultsFromTheConfig(t *testing.T) {
	asked := 0
	listVideos = func(_ context.Context, _, _, _ string, n int) ([]ytdlp.ListEntry, time.Time, error) {
		asked = n
		return nil, time.Time{}, nil
	}
	t.Cleanup(func() { listVideos = ytdlp.List })

	cfg := store.DefaultConfig
	cfg.SearchResults = 5
	next, cmd := newModel(testApp(t, cfg, nil), Options{URL: "cats"}).Update(depsCheckedMsg{})
	if m := next.(model); m.list.target != "ytsearch10:cats" || cmd == nil {
		t.Fatalf("target %q", m.list.target)
	}
	cmd()
	if asked != 5 {
		t.Errorf("kept %d videos, want the config's 5", asked)
	}
}

func TestAHandleListsTheChannelsNewestVideos(t *testing.T) {
	at := time.Date(2020, 1, 1, 6, 15, 42, 0, time.UTC)
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{URL: " @jawed "})
	if m.stage != stageBusy || m.busyLabel != "Checking tools..." || m.link.Value() != "@jawed" {
		t.Fatalf("stage %v %q, box %q", m.stage, m.busyLabel, m.link.Value())
	}
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(m, depsCheckedMsg{})
	if m.stage != stageListing || m.list.target != "https://www.youtube.com/@jawed/videos" {
		t.Fatalf("stage %v, target %q", m.stage, m.list.target)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Listing @jawed's videos...") {
		t.Errorf("no listing line:\n%s", view)
	}
	m = listedWith(m, testVideos(3, at), at)
	lines := strings.Split(m.View().Content, "\n")
	if heading := lineWith(lines, "Newest videos from @jawed:"); heading < 0 {
		t.Fatalf("no heading:\n%s", ansi.Strip(m.View().Content))
	}
	// Every video is the channel's own, so the rows don't name it.
	zoo := lineWith(lines, "Me at the zoo")
	if got := rowText(lines[zoo+1]); got != "1.2M views · 3 weeks ago · 0:19" {
		t.Errorf("details line %q", got)
	}
	if m = update(m, escKey); m.stage != stageLink || m.link.Value() != "@jawed" {
		t.Errorf("esc: stage %v, box %q", m.stage, m.link.Value())
	}
}

func TestAHandleWithMoreWordsIsASearch(t *testing.T) {
	m := searching(t, testApp(t, store.DefaultConfig, nil), "@jawed zoo")
	if m.list.target != "ytsearch20:@jawed zoo" {
		t.Errorf("target %q", m.list.target)
	}
}

func TestAChannelIsRefusedOnceTheDailyLimitIsUsedUp(t *testing.T) {
	cfg := store.DefaultConfig
	cfg.DailyLimit = 1
	m := newModel(testApp(t, cfg, []store.Entry{{Date: store.Today(), VideoID: "aaaaaaaaaaa"}}), Options{URL: "@jawed"})
	if !m.quitting || m.exitCode != 1 || !strings.Contains(m.final, "Daily limit reached (1/1)") {
		t.Fatalf("quitting %v, code %d, final %q", m.quitting, m.exitCode, m.final)
	}
}

func TestAChannelThatCantBeListedGoesBackToTheBox(t *testing.T) {
	m := newModel(testApp(t, store.DefaultConfig, nil), Options{URL: "@nosuchchannel123"})
	m = update(m, depsCheckedMsg{})
	// What yt-dlp said for a handle that doesn't exist, live.
	m = update(m, listedMsg{seq: m.listings, err: fmt.Errorf("ERROR: [youtube:tab] @nosuchchannel123/videos: " +
		"Unable to download API page: HTTP Error 404: Not Found (caused by <HTTPError 404: Not Found>)")})
	if m.stage != stageLink || m.problem != "There's no channel @nosuchchannel123 on YouTube." ||
		m.link.Value() != "@nosuchchannel123" {
		t.Fatalf("stage %v, problem %q, box %q", m.stage, m.problem, m.link.Value())
	}
	m = press(m, tea.KeyEnter)
	m = update(m, depsCheckedMsg{})
	m = update(m, listedMsg{seq: m.listings, err: fmt.Errorf("ERROR: HTTP Error 429: Too Many Requests")})
	if want := "Couldn't list @nosuchchannel123's videos: ERROR: HTTP Error 429: Too Many Requests"; m.problem != want {
		t.Errorf("other errors: problem %q", m.problem)
	}
	m = newModel(testApp(t, store.DefaultConfig, nil), Options{URL: "@quietchannel"})
	m = update(m, depsCheckedMsg{})
	if m = update(m, listedMsg{seq: m.listings}); m.problem != "@quietchannel has no videos to list." {
		t.Errorf("problem %q", m.problem)
	}
}

func TestAChannelListKeepsTheConfigsCount(t *testing.T) {
	var asked int
	var target string
	listVideos = func(_ context.Context, _, _, tgt string, n int) ([]ytdlp.ListEntry, time.Time, error) {
		asked, target = n, tgt
		return nil, time.Time{}, nil
	}
	t.Cleanup(func() { listVideos = ytdlp.List })
	_, cmd := newModel(testApp(t, store.DefaultConfig, nil), Options{URL: "@jawed"}).Update(depsCheckedMsg{})
	cmd()
	if asked != 15 || target != "https://www.youtube.com/@jawed/videos" {
		t.Errorf("listed %q, keeping %d", target, asked)
	}
}

// A long yt-dlp error wraps rather than being cut off at the edge.
func TestLongProblemsWrap(t *testing.T) {
	m := searching(t, testApp(t, store.DefaultConfig, nil), "cats")
	m = update(m, tea.WindowSizeMsg{Width: 40, Height: 24})
	long := "ERROR: [youtube:search] cats: Unable to download API page: HTTP Error 503: Service Unavailable"
	m = update(m, listedMsg{seq: m.listings, err: fmt.Errorf("%s", long)})
	view := ansi.Strip(m.View().Content)
	for _, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 39 {
			t.Errorf("line is %d columns in a 40-column terminal: %q", w, line)
		}
	}
	if !strings.Contains(strings.Join(strings.Fields(view), " "), "HTTP Error 503: Service Unavailable") {
		t.Errorf("the end of the error is missing:\n%s", view)
	}
}
