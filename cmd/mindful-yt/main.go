// Command mindful-yt downloads one YouTube video at a time, even with YouTube
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
	"os/signal"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/term"

	"github.com/abhiram-ar/mindful-yt/internal/deps"
	"github.com/abhiram-ar/mindful-yt/internal/human"
	"github.com/abhiram-ar/mindful-yt/internal/proxy"
	"github.com/abhiram-ar/mindful-yt/internal/store"
	"github.com/abhiram-ar/mindful-yt/internal/tui"
	"github.com/abhiram-ar/mindful-yt/internal/ytdlp"
)

var qualities = []int{144, 240, 360, 480, 720, 1080, 1440, 2160}

// version is set by release builds (-X main.version=...).
var version = "dev"

// currentVersion is the release version, or the module version for a
// "go install ...@v1.2.3" build.
func currentVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

const usage = `mindful-yt: download one YouTube video from a link, even with YouTube blocked in
the hosts file. Single videos only, with a daily limit.

Usage:
  mindful-yt [link] [-q RES] [-r REASON]
  mindful-yt --history | --check | --update | --version
  mindful-yt lock-me-in

  link           a single YouTube video; asked for if left out
  -q, --quality  resolution: 144, 240, 360, 480, 720, 1080, 1440 or 2160
                 (skips the picker; you get the best up to that)
  -r, --reason   why you're watching it (skips the question)
  --history      today's count and recent downloads
  --check        check for yt-dlp, a JavaScript runtime (Deno or Node.js) and
                 ffmpeg, and offer to install whatever is missing
  --update       update yt-dlp (fixes most sudden breakages)
  --version      print mindful-yt's version
  lock-me-in     block YouTube in this machine's hosts file (mindful-yt
                 still downloads)

Settings and history: %s
yt-dlp:               %s
`

func main() { os.Exit(run(os.Args[1:])) }

func run(argv []string) int {
	if len(argv) > 0 && argv[0] == "lock-me-in" {
		return lockMeIn(argv[1:])
	}
	var opts tui.Options
	var qualityText string
	var showHistory, update, showVersion bool
	fs := flag.NewFlagSet("mindful-yt", flag.ContinueOnError)
	fs.BoolVar(&showVersion, "version", false, "")
	fs.StringVar(&qualityText, "q", "", "")
	fs.StringVar(&qualityText, "quality", "", "")
	fs.StringVar(&opts.Reason, "r", "", "")
	fs.StringVar(&opts.Reason, "reason", "", "")
	fs.BoolVar(&showHistory, "history", false, "")
	fs.BoolVar(&update, "update", false, "")
	fs.BoolVar(&opts.CheckOnly, "check", false, "")
	fs.Usage = func() {
		data, tools, _ := store.Dirs()
		fmt.Fprintf(os.Stderr, usage, data, deps.YtdlpPath(tools))
	}

	// Accept flags before or after the link: mindful-yt <link> -q 720.
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
	if showVersion { // before any folder is looked up, moved or written
		fmt.Println("mindful-yt", currentVersion())
		return 0
	}
	dataDir, toolsDir, err := store.Dirs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	st := store.Store{Dir: dataDir}
	ytdlpPath := deps.YtdlpPath(toolsDir)
	if len(positional) > 1 {
		fmt.Fprintln(os.Stderr, "One link at a time. (Put quotes around links.)")
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
		fmt.Fprintln(os.Stderr, "mindful-yt needs an interactive terminal.")
		return 2
	}

	p, err := proxy.Start(proxy.YouTubeLookup(proxy.NewResolver()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Couldn't start the local proxy:", err)
		return 1
	}
	defer p.Close()
	// A closed terminal (SIGHUP) cancels everything, like quitting does.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGHUP)
	defer cancel()

	code, err := tui.Run(&tui.App{
		Ctx: ctx, Store: st, Config: cfg, Entries: entries, Tools: toolsDir, ProxyURL: p.URL(),
	}, opts)
	cancel()                     // stops a yt-dlp that's still running...
	ytdlp.Wait(10 * time.Second) // ...and waits until it has, so none is left behind
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
