// Package ytdlp runs yt-dlp: listing videos (a search), looking one up,
// turning its formats into resolution choices, and downloading while reading
// its progress.
package ytdlp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/abhiram-ar/mindful-yt/internal/platform"
)

// baseArgs are the options every yt-dlp run gets.
func baseArgs() []string {
	return []string{
		"--ignore-config",
		"--config-locations", "-", // the proxy, from stdin; see Command
		"--js-runtimes", "node",
		"--no-playlist",
		"--encoding", "utf-8",
	}
}

// running counts the yt-dlp processes started and not yet waited for.
var running atomic.Int32

// Command prepares a yt-dlp run through the proxy at proxyURL, with the
// options every run gets. It stops, along with everything it started, when
// ctx is cancelled.
func Command(ctx context.Context, path, proxyURL string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, append(baseArgs(), args...)...)
	// The proxy URL holds this run's password, and other local users can read
	// a process's arguments on Linux and macOS. So it goes in on stdin, which
	// "--config-locations -" makes yt-dlp read as a config file.
	cmd.Stdin = strings.NewReader("--proxy " + proxyURL + "\n")
	// PYTHONUTF8 keeps non-ASCII titles intact in piped output. yt-dlp's folder
	// goes first on its PATH so it finds the Deno mindful-yt may have put there.
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1",
		"PATH="+filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	// yt-dlp starts a Python child, which starts the JS runtime and ffmpeg: stop them all.
	platform.NewProcessGroup(cmd)
	cmd.Cancel = func() error { return platform.KillTree(cmd.Process.Pid) }
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// Wait blocks until every yt-dlp that List, Probe or Download started has exited,
// or until timeout. Call it after cancelling their context, so mindful-yt doesn't
// exit while one is still being stopped.
func Wait(timeout time.Duration) {
	for deadline := time.Now().Add(timeout); running.Load() > 0 && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
}

// Probe asks yt-dlp about a video without downloading it. The raw JSON comes
// back too, so the download can reuse it (--load-info-json) instead of
// asking YouTube again.
func Probe(ctx context.Context, path, proxyURL, watchURL string) (VideoInfo, []byte, error) {
	out, err := output(ctx, path, proxyURL, "-J", watchURL)
	if err != nil {
		return VideoInfo{}, nil, err
	}
	var info VideoInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return VideoInfo{}, nil, fmt.Errorf("couldn't read yt-dlp's answer: %w", err)
	}
	return info, out, nil
}

// output runs yt-dlp and returns what it printed, or its error. It counts as
// running until yt-dlp has exited, so Wait covers it.
func output(ctx context.Context, path, proxyURL string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := Command(ctx, path, proxyURL, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	running.Add(1)
	err := cmd.Run()
	running.Add(-1)
	if err != nil {
		return nil, errorFrom(stderr.String(), err)
	}
	return stdout.Bytes(), nil
}

// errorFrom turns yt-dlp's stderr into a short message: its last ERROR line.
func errorFrom(stderr string, err error) error {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); strings.HasPrefix(line, "ERROR:") {
			return errors.New(line)
		}
	}
	return fmt.Errorf("yt-dlp failed: %w", err)
}

// DownloadArgs downloads the probed video (saved as infoJSON) at up to res.
func DownloadArgs(infoJSON, outputDir string, res int) []string {
	return []string{
		"--load-info-json", infoJSON,
		"-S", fmt.Sprintf("res:%d,vcodec:h264,acodec:m4a", res),
		"--merge-output-format", "mp4",
		"-P", outputDir,
		// Height in the name, so copies at different resolutions don't collide.
		"-o", "%(title).150B [%(id)s] %(height)sp.%(ext)s",
		// Machine-readable progress and results. --print implies quiet and
		// simulate, so switch the download and its progress back on. Five
		// progress lines a second is plenty for the screen.
		"--no-simulate", "--progress", "--newline", "--progress-delta", "0.2",
		"--progress-template", "download:mindful-yt-progress %(progress.status)s %(info.format_id)s " +
			"%(progress.downloaded_bytes)s %(progress.total_bytes)s " +
			"%(progress.total_bytes_estimate)s %(progress.speed)s %(progress.eta)s",
		"--print", "before_dl:mindful-yt-formats %(format_id)s",
		"--print", "after_move:mindful-yt-done %(height)s %(filepath)s",
	}
}

// Selected names the formats yt-dlp picked, in the order it downloads them
// (video before audio). It comes before any Progress.
type Selected struct{ Formats []string }

// Progress is a download's state, per part (video and audio come separately).
type Progress struct {
	Part, Parts int
	FormatID    string
	Done, Total float64 // bytes of the current part
	Speed, ETA  float64 // bytes per second, seconds
	Finished    bool    // the current part is complete
}

// Result is how a download ended.
type Result struct {
	Path   string
	Height int
	Err    error
}

// Download runs yt-dlp through the proxy at proxyURL and sends a Selected,
// Progress values, then one Result on events before closing it.
func Download(ctx context.Context, path, proxyURL string, args []string, events chan<- any) {
	defer close(events)
	send := func(ev any) {
		select {
		case events <- ev:
		case <-ctx.Done():
		}
	}
	var result Result
	defer func() { send(result) }()

	var stderr bytes.Buffer
	cmd := Command(ctx, path, proxyURL, args...)
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		result.Err = err
		return
	}
	running.Add(1)
	defer running.Add(-1)
	if err := cmd.Start(); err != nil {
		result.Err = err
		return
	}

	var formats []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "mindful-yt-formats "):
			formats = strings.Split(strings.TrimPrefix(line, "mindful-yt-formats "), "+")
			send(Selected{Formats: formats})
		case strings.HasPrefix(line, "mindful-yt-progress "):
			p, ok := parseProgress(line, formats)
			switch {
			case !ok:
			case p.Finished:
				send(p) // the screen needs this one to turn the bar green
			default:
				select { // drop an update rather than stall yt-dlp if the screen is behind
				case events <- p:
				default:
				}
			}
		case strings.HasPrefix(line, "mindful-yt-done "):
			result.Height, result.Path = parseDone(line)
		}
	}

	switch err := cmd.Wait(); {
	case ctx.Err() != nil:
		result.Err = ctx.Err()
	case err != nil:
		result.Err = errorFrom(stderr.String(), err)
	case result.Path == "":
		result.Err = errors.New("yt-dlp finished without saying where it saved the video")
	}
}

func parseProgress(line string, formats []string) (Progress, bool) {
	// status format_id downloaded total estimate speed eta
	f := strings.Fields(strings.TrimPrefix(line, "mindful-yt-progress "))
	if len(f) != 7 {
		return Progress{}, false
	}
	p := Progress{Part: 1, Parts: max(1, len(formats)), FormatID: f[1], Finished: f[0] == "finished"}
	if i := slices.Index(formats, f[1]); i >= 0 {
		p.Part = i + 1
	}
	p.Done, p.Total = number(f[2]), number(f[3])
	if p.Total == 0 {
		p.Total = number(f[4])
	}
	p.Speed, p.ETA = number(f[5]), number(f[6])
	return p, true
}

func parseDone(line string) (height int, path string) {
	rest := strings.TrimPrefix(line, "mindful-yt-done ")
	h, path, _ := strings.Cut(rest, " ")
	height, _ = strconv.Atoi(h)
	return height, path
}

// number reads one of yt-dlp's template values, which are "NA" when unknown.
func number(s string) float64 {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n
}
