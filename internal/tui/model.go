// Package tui is ytget's terminal interface, built on Bubble Tea: paste a
// link, pick a resolution, say why you're watching, and watch it download.
package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/deps"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/store"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/ytdlp"
)

// Options come from the command line; each one that's set skips a screen.
type Options struct {
	URL       string
	Quality   int    // 0: pick on screen
	Reason    string // "": ask on screen
	CheckOnly bool   // only check for (and offer to install) the tools ytget needs
}

// App is what the interface works with.
type App struct {
	Ctx      context.Context
	Store    store.Store
	Config   store.Config
	Entries  []store.Entry
	Tools    string // ytget's tools folder: its own yt-dlp, and Deno if it installed one
	ProxyURL string // the private proxy yt-dlp goes through
}

// Run shows the interface until the user is done, or until app.Ctx is
// cancelled (the terminal went away), and returns the exit code.
func Run(app *App, opts Options) (int, error) {
	final, err := tea.NewProgram(newModel(app, opts), tea.WithContext(app.Ctx)).Run()
	switch {
	case errors.Is(err, tea.ErrProgramKilled) && app.Ctx.Err() != nil:
		return 129, nil // hung up
	case err != nil:
		return 1, err
	}
	return final.(model).exitCode, nil
}

type stage int

const (
	stageLink        stage = iota // paste a link
	stageSaved                    // already saved: play a copy or get another resolution
	stageBusy                     // spinner while checking tools or looking the video up
	stageDeps                     // tools are missing: offer to install them
	stageInstalling               // ytget downloading yt-dlp or Deno
	stagePick                     // choose a resolution
	stageReason                   // say why you're watching
	stageDownloading              // progress bars
	stageDone                     // saved: play, open folder or quit
)

type (
	depsCheckedMsg struct {
		missing []deps.Dependency
		summary string // for --check
	}
	installProgressMsg struct{ done, total int64 }
	installDoneMsg     struct {
		name string
		err  error
	}
	commandDoneMsg struct{ err error }
	probeDoneMsg   struct {
		info ytdlp.VideoInfo
		raw  []byte
		err  error
	}
)

// part is one stream of the download (the video, then the audio).
type part struct {
	label   string  // "Video 1080p", "Audio"
	size    float64 // expected bytes from the lookup; 0 if YouTube didn't say
	started bool
	prog    ytdlp.Progress
}

type model struct {
	app  *App
	opts Options

	stage     stage
	busyLabel string
	problem   string // shown under the current screen, e.g. a rejected link
	final     string // what stays on screen after quitting
	exitCode  int
	quitting  bool
	initCmd   tea.Cmd
	width     int  // terminal columns; 0 until the first WindowSizeMsg
	taskbar   bool // the terminal shows progress in its tab or taskbar

	link    textinput.Model
	reason  textinput.Model
	spin    spinner.Model
	bar     progress.Model // in progress
	doneBar progress.Model // finished
	cursor  int

	videoID, watchURL string
	saved             []store.Entry

	missing           []deps.Dependency
	queue             []deps.Dependency
	triedInstall      bool
	installing        string // what ytget is downloading
	installed, needed int64

	info       ytdlp.VideoInfo
	infoJSON   []byte
	qualities  []ytdlp.Quality
	quality    int
	reasonText string

	events <-chan any
	cancel context.CancelFunc
	parts  []part
	logged store.Entry
}

func newModel(app *App, opts Options) model {
	linkInput := textinput.New()
	linkInput.Prompt = "› "
	linkInput.Placeholder = "https://youtu.be/..."
	linkInput.CharLimit = 500
	linkInput.SetWidth(60)

	reasonInput := textinput.New()
	reasonInput.Prompt = "› "
	reasonInput.Placeholder = "what you want to get out of it"
	reasonInput.CharLimit = 300
	reasonInput.SetWidth(60)

	m := model{
		app: app, opts: opts, link: linkInput, reason: reasonInput,
		spin: spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(accent)),
		bar:  progress.New(progress.WithDefaultBlend()),
		// One colour fills solidly only with full blocks; the default half block
		// looks solid only when blended.
		doneBar: progress.New(progress.WithColors(lipgloss.Color("42")), progress.WithFillCharacters('█', '░')),
		taskbar: termProgress(os.Getenv),
	}
	var cmd tea.Cmd
	switch {
	case opts.CheckOnly:
		m, cmd = m.checkDeps()
	case opts.URL != "":
		m, cmd = m.acceptLink(opts.URL)
	default:
		cmd = m.link.Focus()
	}
	m.initCmd = cmd
	return m
}

func (m model) Init() tea.Cmd { return tea.Batch(m.initCmd, m.spin.Tick) }

// termProgress reports whether the terminal draws OSC 9;4 as progress in its
// tab or taskbar. Other terminals may print the sequence, or (iTerm2 before
// 3.6.6) pop up a notification for every update, so only known ones get it.
func termProgress(getenv func(string) string) bool {
	switch {
	case getenv("WT_SESSION") != "", getenv("ConEmuANSI") == "ON": // Windows Terminal, ConEmu
		return true
	}
	version := getenv("TERM_PROGRAM_VERSION")
	switch getenv("TERM_PROGRAM") {
	case "ghostty":
		return versionAtLeast(version, 1, 2, 0)
	case "iTerm.app":
		return versionAtLeast(version, 3, 6, 6)
	}
	return false
}

func versionAtLeast(version string, want ...int) bool {
	parts := strings.Split(version, ".")
	for i, w := range want {
		n := 0
		if i < len(parts) {
			digits := strings.TrimRightFunc(parts[i], func(r rune) bool { return r < '0' || r > '9' })
			n, _ = strconv.Atoi(digits)
		}
		if n != w {
			return n > w
		}
	}
	return true
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.quit("Cancelled.", 130)
		}
		return m.handleKey(msg)
	case depsCheckedMsg:
		return m.depsChecked(msg)
	case installProgressMsg:
		m.installed, m.needed = msg.done, msg.total
		return m, waitFor(m.events)
	case installDoneMsg:
		if msg.err != nil {
			return m.fail("Couldn't download " + msg.name + ": " + msg.err.Error())
		}
		return m.installNext()
	case commandDoneMsg:
		return m.installNext() // whether it worked is judged by checking again
	case probeDoneMsg:
		return m.probed(msg)
	case ytdlp.Selected:
		m = m.selected(msg)
		return m, waitFor(m.events)
	case ytdlp.Progress:
		m = m.progressed(msg)
		return m, waitFor(m.events)
	case ytdlp.Result:
		return m.downloaded(msg)
	}
	return m.updateInputs(msg) // paste, cursor blink
}

func (m model) updateInputs(msg tea.Msg) (model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.stage {
	case stageLink:
		m.link, cmd = m.link.Update(msg)
	case stageReason:
		m.reason, cmd = m.reason.Update(msg)
	}
	return m, cmd
}

func (m model) handleKey(msg tea.KeyPressMsg) (model, tea.Cmd) {
	key := msg.String()
	switch m.stage {
	case stageLink:
		switch key {
		case "enter":
			return m.acceptLink(m.link.Value())
		case "esc":
			return m.quit("", 0)
		}
		m.problem = ""
		return m.updateInputs(msg)

	case stageSaved:
		switch key {
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(len(m.saved), m.cursor+1)
		case "enter":
			if m.cursor < len(m.saved) {
				return m.play(m.saved[m.cursor].File, "Opened "+m.saved[m.cursor].File)
			}
			return m.startDownloadFlow()
		case "esc", "q":
			return m.quit("", 0)
		}

	case stageDeps:
		switch key {
		case "enter":
			m.queue, m.triedInstall = m.missing, true
			return m.installNext()
		case "esc", "q":
			return m.quit(bad.Render("ytget can't download without these."), 1)
		}

	case stagePick:
		switch key {
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(len(m.qualities)-1, m.cursor+1)
		case "enter":
			return m.chooseQuality(m.qualities[m.cursor].Res)
		case "esc", "q":
			return m.quit("", 0)
		}

	case stageReason:
		switch key {
		case "enter":
			return m.acceptReason(m.reason.Value())
		case "esc":
			return m.quit("", 0)
		}
		m.problem = ""
		return m.updateInputs(msg)

	case stageBusy, stageInstalling, stageDownloading:
		if key == "esc" {
			return m.quit("Cancelled.", 130)
		}

	case stageDone:
		switch key {
		case "enter", "p":
			return m.play(m.logged.File, m.savedLine())
		case "o":
			return m.play(filepath.Dir(m.logged.File), m.savedLine())
		case "esc", "q":
			return m.quit(m.savedLine(), 0)
		}
	}
	return m, nil
}
