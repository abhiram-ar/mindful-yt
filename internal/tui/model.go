// Package tui is ytget's terminal interface, built on Bubble Tea: paste a
// link, pick a resolution, say why you're watching, and watch it download.
package tui

import (
	"context"
	"path/filepath"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

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
	Ytdlp    string // ytget's own copy of yt-dlp.exe
	ProxyURL string // the private proxy yt-dlp goes through
}

// Run shows the interface until the user is done and returns the exit code.
func Run(app *App, opts Options) (int, error) {
	final, err := tea.NewProgram(newModel(app, opts)).Run()
	if err != nil {
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
	stageInstalling               // downloading yt-dlp
	stagePick                     // choose a resolution
	stageReason                   // say why you're watching
	stageDownloading              // progress bar
	stageDone                     // saved: play, open folder or quit
)

type (
	depsCheckedMsg struct {
		missing []deps.Dependency
		summary string // for --check
	}
	installProgressMsg struct{ done, total int64 }
	installDoneMsg     struct{ err error }
	wingetDoneMsg      struct{ err error }
	probeDoneMsg       struct {
		info ytdlp.VideoInfo
		raw  []byte
		err  error
	}
)

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

	link   textinput.Model
	reason textinput.Model
	spin   spinner.Model
	bar    progress.Model
	cursor int

	videoID, watchURL string
	saved             []store.Entry

	missing           []deps.Dependency
	queue             []deps.Dependency
	triedInstall      bool
	installed, needed int64

	info       ytdlp.VideoInfo
	infoJSON   []byte
	qualities  []ytdlp.Quality
	quality    int
	reasonText string

	events <-chan any
	cancel context.CancelFunc
	prog   ytdlp.Progress
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
		bar:  progress.New(progress.WithDefaultBlend(), progress.WithWidth(50)),
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.bar.SetWidth(min(60, max(10, msg.Width-4)))
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
			return m.fail("Couldn't download yt-dlp: " + msg.err.Error())
		}
		return m.installNext()
	case wingetDoneMsg:
		return m.installNext() // whether it worked is judged by checking again
	case probeDoneMsg:
		return m.probed(msg)
	case ytdlp.Progress:
		m.prog = msg
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
