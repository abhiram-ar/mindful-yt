// Package ytdlp runs yt-dlp: looking a video up, turning its formats into
// resolution choices, and downloading while reading its progress.
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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/platform"
)

// BaseArgs are the options every yt-dlp run gets.
func BaseArgs(proxyURL string) []string {
	return []string{
		"--ignore-config",
		"--proxy", proxyURL,
		"--js-runtimes", "node",
		"--no-playlist",
		"--encoding", "utf-8",
	}
}

// Command prepares a yt-dlp run that stops, along with everything it
// started, when ctx is cancelled.
func Command(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1")
	// yt-dlp.exe starts a Python child, which starts node and ffmpeg: stop them all.
	cmd.Cancel = func() error { return platform.KillTree(cmd.Process.Pid) }
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// Probe asks yt-dlp about a video without downloading it. The raw JSON comes
// back too, so the download can reuse it (--load-info-json) instead of
// asking YouTube again.
func Probe(ctx context.Context, path, proxyURL, watchURL string) (VideoInfo, []byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := Command(ctx, path, append(BaseArgs(proxyURL), "-J", watchURL)...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return VideoInfo{}, nil, errorFrom(stderr.String(), err)
	}
	var info VideoInfo
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		return VideoInfo{}, nil, fmt.Errorf("couldn't read yt-dlp's answer: %w", err)
	}
	return info, stdout.Bytes(), nil
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
func DownloadArgs(proxyURL, infoJSON, outputDir string, res int) []string {
	return append(BaseArgs(proxyURL),
		"--load-info-json", infoJSON,
		"-S", fmt.Sprintf("res:%d,vcodec:h264,acodec:m4a", res),
		"--merge-output-format", "mp4",
		"-P", outputDir,
		// Height in the name, so copies at different resolutions don't collide.
		"-o", "%(title).150B [%(id)s] %(height)sp.%(ext)s",
		// Machine-readable progress and results. --print implies quiet and
		// simulate, so switch the download and its progress back on.
		"--no-simulate", "--progress", "--newline",
		"--progress-template", "download:ytget-progress %(progress.status)s %(info.format_id)s "+
			"%(progress.downloaded_bytes)s %(progress.total_bytes)s "+
			"%(progress.total_bytes_estimate)s %(progress.speed)s %(progress.eta)s",
		"--print", "before_dl:ytget-formats %(format_id)s",
		"--print", "after_move:ytget-done %(height)s %(filepath)s",
	)
}

// Progress is a download's state, per part (video and audio come separately).
type Progress struct {
	Part, Parts int
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

// Download runs yt-dlp and sends Progress values, then one Result, on events
// before closing it.
func Download(ctx context.Context, path string, args []string, events chan<- any) {
	defer close(events)
	var result Result
	defer func() {
		select {
		case events <- result:
		case <-ctx.Done():
		}
	}()

	var stderr bytes.Buffer
	cmd := Command(ctx, path, args...)
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		result.Err = err
		return
	}
	if err := cmd.Start(); err != nil {
		result.Err = err
		return
	}

	var formats []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		switch {
		case strings.HasPrefix(line, "ytget-formats "):
			formats = strings.Split(strings.TrimPrefix(line, "ytget-formats "), "+")
		case strings.HasPrefix(line, "ytget-progress "):
			if p, ok := parseProgress(line, formats); ok {
				select { // drop an update rather than stall yt-dlp if the screen is behind
				case events <- p:
				default:
				}
			}
		case strings.HasPrefix(line, "ytget-done "):
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
	f := strings.Fields(strings.TrimPrefix(line, "ytget-progress "))
	if len(f) != 7 {
		return Progress{}, false
	}
	p := Progress{Part: 1, Parts: max(1, len(formats)), Finished: f[0] == "finished"}
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
	rest := strings.TrimPrefix(line, "ytget-done ")
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
