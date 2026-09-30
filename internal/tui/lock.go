package tui

// mindful-yt lock-me-in: two questions and a countdown, then YouTube's block
// lines go into the hosts file.

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/timer"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/abhiram-ar/mindful-yt/internal/hosts"
	"github.com/abhiram-ar/mindful-yt/internal/platform"
)

// Lock is what lock-me-in works with.
type Lock struct {
	Ctx    context.Context
	Path   string   // the hosts file
	Lines  []string // the block lines it's missing
	Helper []string // mindful-yt and the arguments that make it write the lines, run as admin
}

// RunLock asks, counts down and blocks YouTube, and returns the exit code.
func RunLock(l Lock) (int, error) {
	final, err := tea.NewProgram(newLockModel(l), tea.WithContext(l.Ctx)).Run()
	switch {
	case errors.Is(err, tea.ErrProgramKilled) && l.Ctx.Err() != nil:
		return 129, nil // hung up
	case err != nil:
		return 1, err
	}
	return final.(lockModel).exitCode, nil
}

type lockStage int

const (
	lockWarn      lockStage = iota // you won't be able to use YouTube
	lockConfirm                    // this changes the hosts file
	lockCountdown                  // last chance to back out
	lockApplying                   // writing, once the OS allows it
)

const (
	answerYes = iota
	answerNo
)

type lockAppliedMsg struct{ err error }

type lockModel struct {
	lock     Lock
	stage    lockStage
	cursor   int // answerYes or answerNo
	width    int
	final    string
	exitCode int
	quitting bool

	countdown timer.Model
	length    time.Duration // of the countdown
	tick      time.Duration // between its steps

	// apply writes the lines and verify reads back what's still missing.
	// Tests replace both, so they never touch the real hosts file.
	apply  func() tea.Cmd
	verify func() ([]string, error)
}

func newLockModel(l Lock) lockModel {
	return lockModel{
		lock: l, cursor: answerNo, length: 10 * time.Second, tick: time.Second,
		apply: func() tea.Cmd { return writeHosts(l) },
		verify: func() ([]string, error) {
			content, err := os.ReadFile(l.Path)
			if err != nil {
				return nil, err
			}
			return hosts.Check(content)
		},
	}
}

// writeHosts adds the lines: straight away when mindful-yt already runs as
// admin, otherwise through the helper, which the OS runs as admin once the
// user allows it. sudo asks for its password in the terminal, so the helper
// gets the terminal while it runs.
func writeHosts(l Lock) tea.Cmd {
	done := func(err error) tea.Msg { return lockAppliedMsg{err} }
	if platform.IsAdmin() {
		return func() tea.Msg {
			_, err := hosts.Append(l.Path)
			if err == nil {
				hosts.FlushDNS()
			}
			return done(err)
		}
	}
	if len(l.Helper) == 0 {
		return func() tea.Msg { return done(errors.New("there's nothing to run as admin")) }
	}
	cmd, err := platform.AdminCommand(l.Helper[0], l.Helper[1:]...)
	if err != nil {
		return func() tea.Msg { return done(err) }
	}
	return tea.Exec(cmd, done)
}

func (m lockModel) Init() tea.Cmd { return nil }

func (m lockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case timer.TickMsg, timer.StartStopMsg:
		var cmd tea.Cmd
		m.countdown, cmd = m.countdown.Update(msg)
		return m, cmd
	case timer.TimeoutMsg:
		if m.stage == lockCountdown && msg.ID == m.countdown.ID() {
			m.stage = lockApplying
			return m, m.apply()
		}
	case lockAppliedMsg:
		return m.applied(msg.err)
	}
	return m, nil
}

func (m lockModel) handleKey(msg tea.KeyPressMsg) (lockModel, tea.Cmd) {
	switch m.stage {
	case lockWarn, lockConfirm:
		switch {
		case key.Matches(msg, keyInterrupt):
			return m.quit("Cancelled. Nothing changed.", 130)
		case key.Matches(msg, keyQuit):
			return m.quit("Nothing changed.", 0)
		case key.Matches(msg, keyUp):
			m.cursor = answerYes
		case key.Matches(msg, keyDown):
			m.cursor = answerNo
		case key.Matches(msg, keySelect):
			return m.answer(m.cursor == answerYes)
		case key.Matches(msg, keyYes):
			return m.answer(true)
		case key.Matches(msg, keyNo):
			return m.answer(false)
		}
	case lockCountdown:
		if key.Matches(msg, keyInterrupt, keyCancel) {
			return m.quit("Cancelled. Nothing changed.", 130)
		}
	}
	// While applying, the OS is asking for permission or the lines are being
	// written, which takes a moment; stopping halfway would help no one.
	return m, nil
}

func (m lockModel) answer(yes bool) (lockModel, tea.Cmd) {
	switch {
	case !yes:
		return m.quit("Nothing changed.", 0)
	case m.stage == lockWarn:
		m.stage, m.cursor = lockConfirm, answerNo
		return m, nil
	}
	m.stage = lockCountdown
	m.countdown = timer.New(m.length, timer.WithInterval(m.tick))
	return m, m.countdown.Init()
}

// applied reads the hosts file back: the lines being there is what counts.
func (m lockModel) applied(err error) (lockModel, tea.Cmd) {
	missing, verifyErr := m.verify()
	if verifyErr == nil && len(missing) == 0 {
		return m.quit(m.lockedIn(), 0)
	}
	if errors.Is(err, platform.ErrDeclined) {
		return m.quit(bad.Render("You said no to the permission prompt. Nothing changed."), 1)
	}
	var conflict *hosts.ConflictError
	if errors.As(verifyErr, &conflict) {
		return m.quit(bad.Render("Couldn't block YouTube: "+conflict.Error()+"."), 1)
	}
	problem := "Couldn't update " + m.lock.Path
	if why := errors.Join(err, verifyErr); why != nil {
		problem += ": " + strings.ReplaceAll(why.Error(), "\n", "; ")
	}
	return m.quit(bad.Render(problem)+"\n"+
		faint.Render("Antivirus software that protects the hosts file, or a read-only hosts file, is the usual cause."), 1)
}

func (m lockModel) lockedIn() string {
	var b strings.Builder
	b.WriteString(good.Render("YouTube is blocked on this machine.") + " Added to " + m.lock.Path + ":\n")
	for _, line := range m.lock.Lines {
		b.WriteString(faint.Render("  "+line) + "\n")
	}
	b.WriteString("\nClose your YouTube tabs and restart your browser; it may still have YouTube cached.")
	return b.String()
}

func (m lockModel) quit(final string, code int) (lockModel, tea.Cmd) {
	m.final, m.exitCode, m.quitting = final, code, true
	return m, tea.Quit
}

func (m lockModel) View() tea.View {
	width := lineWidthFor(m.width)
	wrap := func(s string) string { return ansi.Wrap(s, width, "") }
	var b strings.Builder
	b.WriteString(ansi.Truncate(accent.Render("mindful-yt")+"  "+faint.Render("lock-me-in"), width, "…") + "\n\n")
	if m.quitting {
		b.WriteString(wrap(m.final) + "\n")
		return tea.NewView(b.String())
	}

	var keys []key.Binding
	switch m.stage {
	case lockWarn:
		b.WriteString(wrap("You won't be able to use YouTube on this machine. Would you like to continue?") + "\n")
		b.WriteString(faint.Render(wrap("(mindful-yt will still download videos.)")) + "\n\n")
		b.WriteString(m.answers())
		keys = []key.Binding{keyChoose, keySelect, keyQuit}

	case lockConfirm:
		b.WriteString(wrap("You should know what you're doing. We are going to update the DNS lookup in your OS's hosts file:") + "\n")
		b.WriteString(ansi.Hardwrap("  "+m.lock.Path, width, true) + "\n")
		for _, line := range m.lock.Lines {
			b.WriteString(faint.Render("  + "+line) + "\n")
		}
		b.WriteString("\n" + m.answers())
		keys = []key.Binding{keyChoose, keySelect, keyQuit}

	case lockCountdown:
		steps := int((m.countdown.Timeout + m.tick - 1) / m.tick) // whole seconds left, rounded up
		b.WriteString(wrap(bold.Render("Locking you in, bud... "+strconv.Itoa(steps)+"s")) + "\n")
		keys = []key.Binding{keyCancel}

	case lockApplying:
		b.WriteString(wrap("Updating the hosts file. Your OS may ask for permission...") + "\n")
	}
	if len(keys) > 0 {
		b.WriteString("\n" + helpLine(width, keys...) + "\n")
	}
	return tea.NewView(b.String())
}

func (m lockModel) answers() string {
	return choiceRow(m.cursor, answerYes, "Yes") + "\n" + choiceRow(m.cursor, answerNo, "No") + "\n"
}
