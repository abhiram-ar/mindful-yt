//go:build live

// Real network tests through the proxy:
//
//	go test -tags live -run Live -v ./internal/ytdlp
//
// They install ytget's own yt-dlp.exe on first run.

package ytdlp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/deps"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/proxy"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/store"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/ytdlp"
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
	exe := filepath.Join(tools, "yt-dlp.exe")
	if _, err := os.Stat(exe); err != nil {
		t.Log("installing yt-dlp.exe")
		if err := deps.InstallYtdlp(ctx, exe, nil); err != nil {
			t.Fatal(err)
		}
	}
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

	infoFile := filepath.Join(t.TempDir(), "info.json")
	os.WriteFile(infoFile, raw, 0o644)
	events := make(chan any, 64)
	go ytdlp.Download(ctx, exe, ytdlp.DownloadArgs(p.URL(), infoFile, t.TempDir(), 144), events)
	var result ytdlp.Result
	updates := 0
	for ev := range events {
		switch ev := ev.(type) {
		case ytdlp.Progress:
			updates++
		case ytdlp.Result:
			result = ev
		}
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
	cmd := ytdlp.Command(ctx, exe, append(ytdlp.BaseArgs(p.URL()), "-v", "--simulate", gangnam)...)
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
	args := append(ytdlp.BaseArgs(p.URL()), "--load-info-json", infoFile,
		"-o", "%(title).150B [%(id)s] %(height)sp.%(ext)s", "--print", "filename")
	out, err := ytdlp.Command(ctx, exe, args...).Output()
	if err != nil {
		t.Fatal(err)
	}
	if name := strings.TrimSpace(string(out)); !strings.Contains(name, "강남스타일") {
		t.Errorf("printed filename came back as %q", name)
	}
}
