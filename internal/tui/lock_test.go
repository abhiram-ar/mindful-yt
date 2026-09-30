package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/abhiram-ar/mindful-yt/internal/hosts"
	"github.com/abhiram-ar/mindful-yt/internal/platform"
)

// testLock is a lock-me-in screen whose countdown takes milliseconds and
// whose apply only counts, so no test touches the real hosts file.
func testLock(t *testing.T) (lockModel, *int) {
	t.Helper()
	m := newLockModel(Lock{
		Ctx: context.Background(), Path: filepath.Join(t.TempDir(), "hosts"), Lines: hosts.Lines(),
	})
	applied := new(int)
	m.apply = func() tea.Cmd {
		*applied++
		return func() tea.Msg { return lockAppliedMsg{} }
	}
	m.verify = func() ([]string, error) { return nil, nil }
	m.length, m.tick = 5*time.Millisecond, time.Millisecond
	m.width = 200 // the temp path is long
	return m, applied
}

func lockKey(m lockModel, k tea.KeyPressMsg) (lockModel, tea.Cmd) {
	next, cmd := m.Update(k)
	return next.(lockModel), cmd
}

var (
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey   = tea.KeyPressMsg{Code: tea.KeyEscape}
	upKey    = tea.KeyPressMsg{Code: tea.KeyUp}
	yKey     = tea.KeyPressMsg{Code: 'y', Text: "y"}
	nKey     = tea.KeyPressMsg{Code: 'n', Text: "n"}
)

// drive runs cmd the way Bubble Tea would, feeding every message it produces
// back into the model, until nothing is left.
func drive(t *testing.T, m lockModel, cmd tea.Cmd) lockModel {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 1000 {
			t.Fatal("the commands never settle")
		}
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil, tea.QuitMsg:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		default:
			next, more := m.Update(msg)
			m = next.(lockModel)
			queue = append(queue, more)
		}
	}
	return m
}

func screen(m lockModel) string { return ansi.Strip(m.View().Content) }

func TestLockStartsOnNoAndNoChangesNothing(t *testing.T) {
	m, applied := testLock(t)
	if view := screen(m); !strings.Contains(view, "You won't be able to use YouTube on this machine.") ||
		!strings.Contains(view, "› No") {
		t.Fatalf("the first question should start on No:\n%s", view)
	}
	m, _ = lockKey(m, enterKey)
	if !m.quitting || m.exitCode != 0 || m.final != "Nothing changed." || *applied != 0 {
		t.Fatalf("enter on No: quitting %v, code %d, final %q, applied %d", m.quitting, m.exitCode, m.final, *applied)
	}

	m, applied = testLock(t)
	m, _ = lockKey(m, yKey)
	if m.stage != lockConfirm || !strings.Contains(screen(m), "› No") {
		t.Fatalf("y on the first question should ask the second, starting on No:\n%s", screen(m))
	}
	m, _ = lockKey(m, nKey)
	if !m.quitting || m.final != "Nothing changed." || *applied != 0 {
		t.Fatalf("n on the second question: final %q, applied %d", m.final, *applied)
	}

	m, _ = testLock(t)
	if m, _ = lockKey(m, escKey); !m.quitting || m.exitCode != 0 {
		t.Fatalf("esc: quitting %v, code %d", m.quitting, m.exitCode)
	}
}

func TestLockConfirmShowsWhatChanges(t *testing.T) {
	m, _ := testLock(t)
	m, _ = lockKey(m, upKey)
	m, _ = lockKey(m, enterKey)
	view := screen(m)
	for _, want := range []string{"You should know what you're doing.", m.lock.Path, "+ 0.0.0.0 youtube.com", "+ :: www.youtu.be"} {
		if !strings.Contains(view, want) {
			t.Errorf("the second question doesn't show %q:\n%s", want, view)
		}
	}
}

func TestLockCountsDownThenBlocks(t *testing.T) {
	m, applied := testLock(t)
	m.length, m.tick = 10*time.Second, time.Second
	m, _ = lockKey(m, yKey)
	m, _ = lockKey(m, yKey)
	view := screen(m)
	if !strings.Contains(view, "Locking you in, bud... 10s") || !strings.Contains(view, "esc cancel") {
		t.Fatalf("countdown:\n%s", view)
	}
	if strings.ContainsAny(view, "▌█░") {
		t.Errorf("the countdown is just the line, with no bar:\n%s", view)
	}

	m, applied = testLock(t) // 5 steps of a millisecond
	m, _ = lockKey(m, yKey)
	m, start := lockKey(m, yKey)
	next, rest := m.Update(start()) // the first tick
	m = next.(lockModel)
	if view := screen(m); !strings.Contains(view, "Locking you in, bud... 4s") || *applied != 0 {
		t.Fatalf("after one step, applied %d:\n%s", *applied, view)
	}
	m = drive(t, m, rest)
	if *applied != 1 || !m.quitting || m.exitCode != 0 {
		t.Fatalf("after the countdown: applied %d, quitting %v, code %d", *applied, m.quitting, m.exitCode)
	}
	if view := screen(m); !strings.Contains(view, "YouTube is blocked on this machine. Added to "+m.lock.Path) ||
		!strings.Contains(view, ":: youtube.com") || !strings.Contains(view, "restart your browser") {
		t.Errorf("done:\n%s", view)
	}
}

func TestLockEscDuringCountdownCancels(t *testing.T) {
	m, applied := testLock(t)
	m, _ = lockKey(m, yKey)
	m, _ = lockKey(m, yKey)
	m, _ = lockKey(m, escKey)
	if !m.quitting || m.exitCode != 130 || m.final != "Cancelled. Nothing changed." || *applied != 0 {
		t.Fatalf("quitting %v, code %d, final %q, applied %d", m.quitting, m.exitCode, m.final, *applied)
	}
}

func TestLockResultIsWhatTheFileSays(t *testing.T) {
	stillMissing := func() ([]string, error) { return []string{":: youtube.com"}, nil }
	for _, c := range []struct {
		name   string
		err    error
		verify func() ([]string, error)
		code   int
		want   string
	}{
		{"written", nil, nil, 0, "YouTube is blocked"},
		{"written, though the helper reported a problem", errors.New("exit status 1"), nil, 0, "YouTube is blocked"},
		{"UAC declined", platform.ErrDeclined, stillMissing, 1, "You said no to the permission prompt. Nothing changed."},
		{"failed", errors.New("exit status 1"), stillMissing, 1, "Couldn't update"},
		{"a real address appeared", nil, func() ([]string, error) {
			return nil, &hosts.ConflictError{Entries: []string{"1.2.3.4 youtube.com"}}
		}, 1, "Couldn't block YouTube: the hosts file points YouTube at a real address (1.2.3.4 youtube.com)"},
	} {
		m, _ := testLock(t)
		if c.verify != nil {
			m.verify = c.verify
		}
		m.stage = lockApplying
		next, _ := m.Update(lockAppliedMsg{c.err})
		m = next.(lockModel)
		if !m.quitting || m.exitCode != c.code || !strings.Contains(strings.ReplaceAll(screen(m), "\n", " "), c.want) {
			t.Errorf("%s: code %d, want %d:\n%s", c.name, m.exitCode, c.code, screen(m))
		}
	}
}

func TestLockScreensFitANarrowTerminal(t *testing.T) {
	m, _ := testLock(t)
	m.lock.Path = `C:\Windows\System32\drivers\etc\hosts`
	next, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 20})
	m = next.(lockModel)
	check := func(label string) {
		for _, line := range strings.Split(m.View().Content, "\n") {
			if w := ansi.StringWidth(line); w > 29 {
				t.Errorf("%s: line is %d columns in a 30-column terminal: %q", label, w, ansi.Strip(line))
			}
		}
	}
	check("first question")
	m, _ = lockKey(m, yKey)
	check("second question")
	m, _ = lockKey(m, yKey)
	check("countdown")
	m.stage = lockApplying
	check("applying")
	next, _ = m.Update(lockAppliedMsg{})
	m = next.(lockModel)
	check("done")
}
