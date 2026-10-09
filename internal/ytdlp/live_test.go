//go:build live

// Real network tests through the proxy:
//
//	go test -tags live -run Live -v ./internal/ytdlp
//
// They install mindful-yt's own yt-dlp, and Node.js if there's no JS runtime,
// on first run. The download test needs ffmpeg and skips without it.

package ytdlp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abhiram-ar/mindful-yt/internal/deps"
	"github.com/abhiram-ar/mindful-yt/internal/link"
	"github.com/abhiram-ar/mindful-yt/internal/proxy"
	"github.com/abhiram-ar/mindful-yt/internal/store"
	"github.com/abhiram-ar/mindful-yt/internal/ytdlp"
)

const zoo = "https://www.youtube.com/watch?v=jNQXAC9IVRw"
const gangnam = "https://www.youtube.com/watch?v=9bZkp7q19f0" // Korean title

func liveSetup(t *testing.T) (context.Context, string, *proxy.Proxy) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	_, tools, err := store.Dirs()
	if err != nil {
		t.Fatal(err)
	}
	exe := deps.YtdlpPath(tools)
	if _, err := os.Stat(exe); err != nil {
		t.Log("installing yt-dlp")
		if err := deps.InstallYtdlp(ctx, exe, nil); err != nil {
			t.Fatal(err)
		}
	}
	if deps.FindJSRuntime(tools) == "" {
		t.Log("installing Node.js")
		if err := deps.InstallNode(ctx, deps.NodePath(tools), nil); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("using %s and %s", exe, deps.FindJSRuntime(tools))
	p, err := proxy.Start(proxy.YouTubeLookup(proxy.NewResolver()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return ctx, exe, p
}

func TestLiveProbeAndDownload(t *testing.T) {
	ctx, exe, p := liveSetup(t)
	info, raw, err := ytdlp.Probe(ctx, exe, p.URL(), zoo)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%q by %s, %v", info.Title, info.ChannelName(), ytdlp.Qualities(info))
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("the download merges video and audio, which needs ffmpeg")
	}

	infoFile := filepath.Join(t.TempDir(), "info.json")
	os.WriteFile(infoFile, raw, 0o644)
	events := make(chan any, 64)
	go ytdlp.Download(ctx, exe, p.URL(), ytdlp.DownloadArgs(infoFile, t.TempDir(), 144), events)
	var result ytdlp.Result
	var selected []string
	updates := 0
	for ev := range events {
		switch ev := ev.(type) {
		case ytdlp.Selected:
			selected = ev.Formats
		case ytdlp.Progress:
			updates++
		case ytdlp.Result:
			result = ev
		}
	}
	t.Logf("formats %v", selected)
	if len(selected) == 0 {
		t.Error("no Selected event before the progress")
	}
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	stat, err := os.Stat(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("saved %s (%d bytes, %dp) after %d progress updates", filepath.Base(result.Path), stat.Size(), result.Height, updates)
	if result.Height != 144 || updates == 0 || !strings.HasSuffix(result.Path, " 144p.mp4") {
		t.Errorf("height %d, %d updates, path %q", result.Height, updates, result.Path)
	}
}

func TestLiveChallengeSolverWorks(t *testing.T) {
	ctx, exe, p := liveSetup(t)
	cmd := ytdlp.Command(ctx, exe, p.URL(), "-v", "--simulate", gangnam)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "js runtime") || strings.Contains(lower, "challenge") || strings.Contains(lower, "ejs") {
			t.Log(strings.TrimSpace(line))
		}
		if strings.Contains(lower, "warning") && strings.Contains(lower, "challenge") {
			t.Errorf("challenge warning: %s", line)
		}
	}
}

func TestLiveNonASCIITitleSurvives(t *testing.T) {
	ctx, exe, p := liveSetup(t)
	info, raw, err := ytdlp.Probe(ctx, exe, p.URL(), gangnam)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(info.Title, "강남스타일") {
		t.Errorf("title came back as %q", info.Title)
	}
	// The printed file path goes through the same pipe as a real download's.
	infoFile := filepath.Join(t.TempDir(), "info.json")
	os.WriteFile(infoFile, raw, 0o644)
	out, err := ytdlp.Command(ctx, exe, p.URL(), "--load-info-json", infoFile,
		"-o", "%(title).150B [%(id)s] %(height)sp.%(ext)s", "--print", "filename").Output()
	if err != nil {
		t.Fatal(err)
	}
	if name := strings.TrimSpace(string(out)); !strings.Contains(name, "강남스타일") {
		t.Errorf("printed filename came back as %q", name)
	}
}

func TestLiveSearch(t *testing.T) {
	ctx, exe, p := liveSetup(t)
	start := time.Now()
	videos, at, err := ytdlp.List(ctx, exe, p.URL(), ytdlp.Search("me at the zoo", 10), 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d videos in %v; yt-dlp's clock %v behind ours", len(videos), time.Since(start).Round(time.Millisecond),
		time.Since(at).Round(time.Millisecond))
	if d := time.Since(at); d < 0 || d > time.Minute {
		t.Errorf("the list was made at %v, which isn't yt-dlp's epoch", at)
	}
	if len(videos) == 0 || len(videos) > 10 {
		t.Fatalf("got %d videos", len(videos))
	}
	seen, dated, zoo := map[string]bool{}, 0, -1
	for i, v := range videos {
		t.Logf("%2d. %s | %s | %.0f views | ts %.0f | %.0fs | %s", i+1, v.Title, v.ChannelName(), v.Views, v.Timestamp, v.Duration, v.URL)
		if _, _, err := link.Canonical(v.URL); err != nil || v.Title == "" || seen[v.ID] {
			t.Errorf("not a single, new, titled video: %+v", v)
		}
		if v.LiveStatus == "is_live" || v.LiveStatus == "is_upcoming" {
			t.Errorf("a live or upcoming video was kept: %+v", v)
		}
		seen[v.ID] = true
		if v.Timestamp > 0 {
			dated++
			// Older than a couple of days, yt-dlp rounds the date to midnight
			// UTC, which human.Ago relies on.
			if time.Since(time.Unix(int64(v.Timestamp), 0)) > 48*time.Hour && int64(v.Timestamp)%86400 != 0 {
				t.Errorf("%s: timestamp %.0f isn't midnight UTC", v.ID, v.Timestamp)
			}
		}
		if v.ID == "jNQXAC9IVRw" {
			zoo = i
		}
	}
	if zoo < 0 {
		t.Fatal("Me at the zoo isn't in its own search")
	}
	if v := videos[zoo]; v.ChannelName() != "jawed" || v.Handle() != "@jawed" || v.Views <= 0 || v.Timestamp <= 0 {
		t.Errorf("Me at the zoo, at %d: %+v", zoo+1, v)
	}
	if dated*2 <= len(videos) {
		t.Errorf("only %d of %d videos have a date: approximate_date didn't take", dated, len(videos))
	}
}

func TestLiveSearchNonASCII(t *testing.T) {
	ctx, exe, p := liveSetup(t)
	videos, _, err := ytdlp.List(ctx, exe, p.URL(), ytdlp.Search("강남스타일", 10), 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range videos {
		if strings.Contains(v.Title, "강남스타일") || v.ID == "9bZkp7q19f0" {
			return
		}
	}
	t.Errorf("no result for the Korean query matches it: %+v", videos)
}

func TestLiveChannel(t *testing.T) {
	ctx, exe, p := liveSetup(t)
	start := time.Now()
	videos, _, err := ytdlp.List(ctx, exe, p.URL(), ytdlp.Uploads("@jawed"), 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d videos in %v", len(videos), time.Since(start).Round(time.Millisecond))
	zoo := false
	for i, v := range videos {
		t.Logf("%2d. %s | %.0f views | ts %.0f | %.0fs | %s", i+1, v.Title, v.Views, v.Timestamp, v.Duration, v.URL)
		if _, _, err := link.Canonical(v.URL); err != nil || v.Title == "" {
			t.Errorf("not a single, titled video: %+v", v)
		}
		zoo = zoo || v.ID == "jNQXAC9IVRw"
	}
	if len(videos) == 0 || len(videos) > 10 || !zoo {
		t.Errorf("got %d videos, Me at the zoo among them: %v", len(videos), zoo)
	}
}

// A big channel must stop at n: -I keeps yt-dlp from paging through it all.
func TestLiveBigChannelStopsAtN(t *testing.T) {
	ctx, exe, p := liveSetup(t)
	start := time.Now()
	videos, _, err := ytdlp.List(ctx, exe, p.URL(), ytdlp.Uploads("@LofiGirl"), 10)
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d videos in %v", len(videos), took.Round(time.Millisecond))
	var last float64
	for i, v := range videos {
		t.Logf("%2d. %s | %.0f views | ts %.0f", i+1, v.Title, v.Views, v.Timestamp)
		if i > 0 && v.Timestamp > 0 && last > 0 && v.Timestamp > last {
			t.Errorf("%d is newer than the one before it: not newest first", i+1)
		}
		if v.Timestamp > 0 {
			last = v.Timestamp
		}
	}
	if len(videos) != 10 || took > time.Minute {
		t.Errorf("got %d videos in %v", len(videos), took)
	}
}

func TestLiveChannelThatDoesntExist(t *testing.T) {
	ctx, exe, p := liveSetup(t)
	_, _, err := ytdlp.List(ctx, exe, p.URL(), ytdlp.Uploads("@zz9nochannelhere7qx"), 10)
	t.Logf("error: %v", err)
	if err == nil {
		t.Error("a channel that doesn't exist was listed")
	}
}
