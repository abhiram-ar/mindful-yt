package tui

// The steps from a pasted link to a saved video. Each returns the updated
// model and whatever command runs next.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/abhiram-ar/mindful-yt/internal/deps"
	"github.com/abhiram-ar/mindful-yt/internal/link"
	"github.com/abhiram-ar/mindful-yt/internal/platform"
	"github.com/abhiram-ar/mindful-yt/internal/store"
	"github.com/abhiram-ar/mindful-yt/internal/ytdlp"
)

func (m model) acceptLink(text string) (model, tea.Cmd) {
	if handle, ok := link.Handle(text); ok {
		return m.startChannel(handle)
	}
	id, watch, err := link.Canonical(text)
	if errors.Is(err, link.ErrNotYouTube) {
		return m.startSearch(text)
	}
	if err != nil {
		m.stage, m.problem = stageLink, err.Error()
		m.link.SetValue(strings.TrimSpace(text))
		return m, m.link.Focus()
	}
	m.videoID, m.watchURL, m.problem = id, watch, ""
	m.link.Blur()
	m.saved = store.SavedCopies(m.app.Entries, id)
	if m.opts.Quality > 0 {
		if e, ok := savedAt(m.saved, m.opts.Quality); ok {
			return m.play(e.File, "Already saved, opened "+e.File)
		}
		return m.startDownloadFlow()
	}
	if len(m.saved) > 0 {
		m.stage, m.cursor = stageSaved, 0
		return m, nil
	}
	return m.startDownloadFlow()
}

func (m model) startDownloadFlow() (model, tea.Cmd) {
	if used, limit := m.usedToday(), m.app.Config.DailyLimit; used >= limit {
		return m.fail(fmt.Sprintf("Daily limit reached (%d/%d). Resets tomorrow.", used, limit))
	}
	return m.checkDeps()
}

func (m model) checkDeps() (model, tea.Cmd) {
	m.stage, m.busyLabel = stageBusy, "Checking tools..."
	tools, summary := m.app.Tools, m.opts.CheckOnly
	return m, func() tea.Msg {
		msg := depsCheckedMsg{missing: deps.Missing(tools)}
		if summary && len(msg.missing) == 0 {
			msg.summary = deps.Summary(tools)
		}
		return msg
	}
}

func (m model) depsChecked(msg depsCheckedMsg) (model, tea.Cmd) {
	m.missing = msg.missing
	switch {
	case len(m.missing) == 0 && m.opts.CheckOnly:
		return m.quit(msg.summary, 0)
	case len(m.missing) == 0 && m.watchURL == "":
		return m.fetchList() // no video picked yet: the tools were checked for a search
	case len(m.missing) == 0:
		return m.startProbe()
	case m.triedInstall:
		lines := []string{"Still missing:"}
		for _, d := range m.missing {
			lines = append(lines, fmt.Sprintf("  %s: %s", d.Name, d.Manual))
		}
		lines = append(lines, "If an install just finished, open a new terminal and run mindful-yt again.")
		return m.fail(strings.Join(lines, "\n"))
	}
	m.stage = stageDeps
	return m, nil
}

func (m model) installNext() (model, tea.Cmd) {
	for len(m.queue) > 0 {
		dep := m.queue[0]
		m.queue = m.queue[1:]
		switch {
		case dep.Download != nil:
			return m.download(dep)
		case dep.Command != nil:
			// The installer takes over the terminal while it runs (sudo may ask
			// for a password), then the screen comes back.
			cmd := exec.Command(dep.Command[0], dep.Command[1:]...)
			return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return commandDoneMsg{err} })
		}
		// Neither: the check afterwards reports it, with how to get it.
	}
	platform.RefreshPath()
	return m.checkDeps()
}

// download has mindful-yt fetch a tool itself, showing a progress bar.
func (m model) download(dep deps.Dependency) (model, tea.Cmd) {
	m.stage, m.installing = stageInstalling, dep.Name
	m.installed, m.needed = 0, 0
	events := make(chan any, 16)
	m.events = events
	ctx := m.app.Ctx
	go func() {
		defer close(events)
		var last time.Time
		err := dep.Download(ctx, func(done, total int64) {
			// A few updates a second is enough; the bytes arrive much faster.
			if now := time.Now(); now.Sub(last) >= 100*time.Millisecond || done == total {
				last = now
				select {
				case events <- installProgressMsg{done, total}:
				default:
				}
			}
		})
		events <- installDoneMsg{dep.Name, err}
	}()
	return m, waitFor(events)
}

func (m model) startProbe() (model, tea.Cmd) {
	m.stage, m.busyLabel = stageBusy, "Looking up the video..."
	ctx, exe, proxyURL, watch := m.app.Ctx, deps.YtdlpPath(m.app.Tools), m.app.ProxyURL, m.watchURL
	return m, func() tea.Msg {
		info, raw, err := ytdlp.Probe(ctx, exe, proxyURL, watch)
		return probeDoneMsg{info, raw, err}
	}
}

func (m model) probed(msg probeDoneMsg) (model, tea.Cmd) {
	if msg.err != nil {
		return m.fail(msg.err.Error())
	}
	m.info, m.infoJSON = msg.info, msg.raw
	m.qualities = ytdlp.Qualities(msg.info)
	if len(m.qualities) == 0 {
		return m.fail("YouTube didn't offer any video for this link.")
	}
	if m.opts.Quality > 0 {
		return m.chooseQuality(m.opts.Quality)
	}
	m.stage = stagePick
	m.cursor = ytdlp.DefaultQualityIndex(m.qualities, m.app.Config.MaxHeight)
	return m, nil
}

func (m model) chooseQuality(res int) (model, tea.Cmd) {
	m.quality = res
	if e, ok := savedAt(m.saved, res); ok {
		return m.play(e.File, "Already saved, opened "+e.File)
	}
	if m.opts.Reason != "" {
		return m.acceptReason(m.opts.Reason)
	}
	m.stage = stageReason
	return m, m.reason.Focus()
}

func (m model) acceptReason(text string) (model, tea.Cmd) {
	text = strings.TrimSpace(text)
	if minLen := m.app.Config.MinReasonLength; utf8.RuneCountInString(text) < minLen {
		m.stage = stageReason
		m.problem = fmt.Sprintf("Give a real reason (at least %d characters).", minLen)
		return m, m.reason.Focus()
	}
	m.reasonText = text
	m.reason.Blur()
	return m.startDownload()
}

func (m model) startDownload() (model, tea.Cmd) {
	infoFile, err := os.CreateTemp("", "mindful-yt-*.info.json")
	if err == nil {
		_, err = infoFile.Write(m.infoJSON)
		if closeErr := infoFile.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		return m.fail("Couldn't write a temporary file: " + err.Error())
	}

	ctx, cancel := context.WithCancel(m.app.Ctx)
	events := make(chan any, 64)
	m.events, m.cancel = events, cancel
	m.stage, m.parts = stageDownloading, nil
	exe, proxyURL := deps.YtdlpPath(m.app.Tools), m.app.ProxyURL
	args := ytdlp.DownloadArgs(infoFile.Name(), m.app.Config.OutputDir, m.quality)
	go func() {
		defer os.Remove(infoFile.Name())
		ytdlp.Download(ctx, exe, proxyURL, args, events)
	}()
	return m, waitFor(events)
}

// selected sets up one progress row per stream yt-dlp is about to download.
func (m model) selected(sel ytdlp.Selected) model {
	m.parts = make([]part, len(sel.Formats))
	for i, id := range sel.Formats {
		f, ok := m.info.Format(id)
		m.parts[i] = part{label: streamLabel(f, ok), size: f.Size()}
	}
	return m
}

func streamLabel(f ytdlp.Format, known bool) string {
	switch {
	case !known:
		return "Download"
	case f.HasVideo() && f.HasAudio():
		return fmt.Sprintf("Video %dp + audio", f.Res())
	case f.HasVideo():
		return fmt.Sprintf("Video %dp", f.Res())
	case f.HasAudio():
		return "Audio"
	}
	return "Download"
}

func (m model) progressed(p ytdlp.Progress) model {
	i := max(1, p.Part) - 1
	for len(m.parts) <= i {
		m.parts = append(m.parts, part{label: "Download"})
	}
	m.parts[i].started, m.parts[i].prog = true, p
	return m
}

func (m model) downloaded(result ytdlp.Result) (model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	if result.Err != nil {
		return m.fail(result.Err.Error())
	}
	e := store.Entry{
		TS:       time.Now().Format("2006-01-02T15:04:05"),
		Date:     store.Today(),
		VideoID:  m.videoID,
		Title:    m.info.Title,
		Channel:  m.info.ChannelName(),
		Duration: m.info.Duration,
		Quality:  m.quality,
		Height:   result.Height,
		Reason:   m.reasonText,
		File:     result.Path,
	}
	if err := m.app.Store.AppendHistory(e); err != nil {
		return m.fail("The video is saved, but the history couldn't be written: " + err.Error())
	}
	m.app.Entries = append(m.app.Entries, e)
	m.logged, m.stage = e, stageDone
	return m, nil
}

func (m model) play(path, final string) (model, tea.Cmd) {
	if err := platform.Open(path); err != nil {
		return m.fail(fmt.Sprintf("Couldn't open %s: %v", path, err))
	}
	return m.quit(final, 0)
}

func (m model) fail(problem string) (model, tea.Cmd) {
	return m.quit(bad.Render(problem), 1)
}

func (m model) quit(final string, code int) (model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	m.final, m.exitCode, m.quitting = final, code, true
	return m, tea.Quit
}

func waitFor(events <-chan any) tea.Cmd {
	return func() tea.Msg {
		if ev, ok := <-events; ok {
			return ev
		}
		return nil
	}
}

func savedAt(saved []store.Entry, quality int) (store.Entry, bool) {
	for _, e := range saved {
		if e.Quality == quality {
			return e, true
		}
	}
	return store.Entry{}, false
}

func (m model) usedToday() int { return store.DownloadsOn(m.app.Entries, store.Today()) }
