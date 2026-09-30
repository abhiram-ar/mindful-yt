// Command ytget downloads one YouTube video at a time, even with YouTube
// blocked in the hosts file. yt-dlp does the downloading through a private
// local proxy that looks YouTube up over DNS-over-HTTPS, so browsers and
// every other app stay blocked.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"

	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/deps"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/human"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/proxy"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/store"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/tui"
)

var qualities = []int{144, 240, 360, 480, 720, 1080, 1440, 2160}

const usage = `ytget: download one YouTube video from a link, even with YouTube blocked in
the hosts file. Single videos only, with a daily limit.

Usage:
  ytget [link] [-q RES] [-r REASON]
  ytget --history | --check | --update

  link           a single YouTube video; asked for if left out
  -q, --quality  resolution: 144, 240, 360, 480, 720, 1080, 1440 or 2160
                 (skips the picker; you get the best up to that)
  -r, --reason   why you're watching it (skips the question)
  --history      today's count and recent downloads
  --check        check for yt-dlp, Node.js and ffmpeg, and offer to install
                 whatever is missing
  --update       update yt-dlp (fixes most sudden breakages)

Settings and history: %s
yt-dlp:               %s
`

func main() { os.Exit(run(os.Args[1:])) }

func run(argv []string) int {
	dataDir, toolsDir, err := store.Dirs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	st := store.Store{Dir: dataDir}
	ytdlpPath := filepath.Join(toolsDir, "yt-dlp.exe")

	var opts tui.Options
	var qualityText string
	var showHistory, update bool
	fs := flag.NewFlagSet("ytget", flag.ContinueOnError)
	fs.StringVar(&qualityText, "q", "", "")
	fs.StringVar(&qualityText, "quality", "", "")
	fs.StringVar(&opts.Reason, "r", "", "")
	fs.StringVar(&opts.Reason, "reason", "", "")
	fs.BoolVar(&showHistory, "history", false, "")
	fs.BoolVar(&update, "update", false, "")
	fs.BoolVar(&opts.CheckOnly, "check", false, "")
	fs.Usage = func() { fmt.Fprintf(os.Stderr, usage, st.Dir, ytdlpPath) }

	// Accept flags before or after the link: ytget <link> -q 720.
	var positional []string
	for {
		if err := fs.Parse(argv); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if argv = fs.Args(); len(argv) == 0 {
			break
		}
		positional, argv = append(positional, argv[0]), argv[1:]
	}
	if len(positional) > 1 {
		fmt.Fprintln(os.Stderr, "One link at a time. (In PowerShell, put quotes around links.)")
		return 2
	}
	if len(positional) == 1 {
		opts.URL = positional[0]
	}
	if qualityText != "" {
		if opts.Quality, err = parseQuality(qualityText); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}

	cfg, err := st.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	entries, err := st.ReadHistory()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	switch {
	case showHistory:
		printHistory(cfg, entries)
		return 0
	case update:
		return runUpdate(ytdlpPath)
	}
	if opts.Reason != "" && utf8.RuneCountInString(strings.TrimSpace(opts.Reason)) < cfg.MinReasonLength {
		fmt.Fprintf(os.Stderr, "Give a real reason (at least %d characters).\n", cfg.MinReasonLength)
		return 2
	}
	if !term.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stderr, "ytget needs an interactive terminal.")
		return 2
	}

	p, err := proxy.Start(proxy.YouTubeLookup(proxy.NewResolver()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't start the local proxy:", err)
		return 1
	}
	defer p.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	code, err := tui.Run(&tui.App{
		Ctx: ctx, Store: st, Config: cfg, Entries: entries, Ytdlp: ytdlpPath, ProxyURL: p.URL(),
	}, opts)
	cancel() // stops a download that's still running
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

func parseQuality(text string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(text)), "p"))
	if err != nil || !slices.Contains(qualities, n) {
		labels := make([]string, len(qualities))
		for i, q := range qualities {
			labels[i] = fmt.Sprintf("%dp", q)
		}
		return 0, fmt.Errorf("-q: pick one of %s", strings.Join(labels, ", "))
	}
	return n, nil
}

func printHistory(cfg store.Config, entries []store.Entry) {
	fmt.Printf("Today: %d/%d downloads\n", store.DownloadsOn(entries, store.Today()), cfg.DailyLimit)
	for _, e := range entries[max(0, len(entries)-20):] {
		details := human.Duration(e.Duration)
		if e.Height > 0 {
			details += fmt.Sprintf(", %dp", e.Height)
		}
		fmt.Printf("\n%s  %s  (%s)\n            why: %s\n", e.Date, e.Title, details, e.Reason)
	}
}

func runUpdate(ytdlpPath string) int {
	if _, err := os.Stat(ytdlpPath); err == nil {
		cmd := exec.Command(ytdlpPath, "-U")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if cmd.Run() != nil {
			return 1
		}
		return 0
	}
	fmt.Println("Downloading yt-dlp from GitHub...")
	lastPercent := -1
	err := deps.InstallYtdlp(context.Background(), ytdlpPath, func(done, total int64) {
		if total > 0 {
			if percent := int(done * 100 / total); percent != lastPercent {
				lastPercent = percent
				fmt.Printf("\r  %3d%%  %s", percent, human.Bytes(float64(total)))
			}
		}
	})
	fmt.Println()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("Installed", ytdlpPath)
	return 0
}
