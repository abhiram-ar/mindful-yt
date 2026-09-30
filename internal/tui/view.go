package tui

import (
	"cmp"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/deps"
	"github.com/abhiram-ar/youtube-downloader-via-dns-over-http/internal/human"
)

var (
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	bold   = lipgloss.NewStyle().Bold(true)
	faint  = lipgloss.NewStyle().Faint(true)
	bad    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	good   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
)

func (m model) View() tea.View {
	if m.quitting && m.final == "" {
		return tea.NewView("")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n\n", accent.Render("ytget"),
		faint.Render(fmt.Sprintf("%d/%d downloads today", m.usedToday(), m.app.Config.DailyLimit)))
	if m.quitting {
		b.WriteString(m.final + "\n")
		return tea.NewView(b.String())
	}

	var taskbar *tea.ProgressBar // progress in the terminal's tab or taskbar, where supported
	switch m.stage {
	case stageLink:
		b.WriteString("Paste a YouTube link:\n" + m.link.View() + "\n")

	case stageSaved:
		b.WriteString(bold.Render(m.fit(m.saved[0].Title)) + "\n\nYou already have this video:\n")
		for i, e := range m.saved {
			label := "saved " + e.Date
			if q := cmp.Or(e.Height, e.Quality); q > 0 { // what arrived, else what was asked for
				label = fmt.Sprintf("%dp, %s", q, label)
			}
			b.WriteString(m.choice(i, "Play it ("+label+")") + "\n")
		}
		b.WriteString(m.choice(len(m.saved), "Download another resolution (counts toward today's limit)") + "\n")

	case stageBusy:
		b.WriteString(m.spin.View() + " " + m.busyLabel + "\n")

	case stageDeps:
		b.WriteString("ytget needs some tools that aren't installed:\n\n")
		for _, d := range m.missing {
			how := "install it yourself: " + d.Manual
			switch {
			case d.Download != nil:
				how = "ytget downloads the official build from GitHub and checks its checksum"
			case d.Command != nil:
				how = "runs: " + deps.CommandLine(d.Command)
			}
			fmt.Fprintf(&b, "  %s %s\n    %s\n", bold.Render(d.Name), faint.Render("("+d.Why+")"), faint.Render(how))
		}

	case stageInstalling:
		pct := fraction(float64(m.installed), float64(m.needed))
		fmt.Fprintf(&b, "Downloading %s from GitHub\n\n", m.installing)
		b.WriteString(barView(m.bar, pct, m.lineWidth()) + "\n")
		b.WriteString(faint.Render(human.Bytes(float64(m.installed))+" of "+human.Bytes(float64(m.needed))) + "\n")
		taskbar = tea.NewProgressBar(tea.ProgressBarDefault, int(pct*100))

	case stagePick:
		b.WriteString(m.videoHeader() + "Pick a resolution:\n")
		for i, q := range m.qualities {
			label := fmt.Sprintf("%-6s", fmt.Sprintf("%dp", q.Res))
			if q.Size > 0 {
				label += faint.Render("  about " + human.Bytes(q.Size))
			}
			b.WriteString(m.choice(i, label) + "\n")
		}

	case stageReason:
		b.WriteString(m.videoHeader())
		b.WriteString(faint.Render(fmt.Sprintf("Resolution: up to %dp", m.quality)) + "\n\n")
		b.WriteString("Why are you watching this?\n" + m.reason.View() + "\n")

	case stageDownloading:
		b.WriteString(m.videoHeader())
		rows, overall, finishing := m.partsView()
		b.WriteString(rows)
		taskbar = tea.NewProgressBar(tea.ProgressBarDefault, int(overall*100))
		if finishing {
			taskbar = tea.NewProgressBar(tea.ProgressBarIndeterminate, 0)
		}

	case stageDone:
		b.WriteString(m.savedLine() + "\n")
	}

	if m.problem != "" {
		b.WriteString("\n" + bad.Render(m.problem) + "\n")
	}
	if help := m.help(); help != "" {
		b.WriteString("\n" + faint.Render(help) + "\n")
	}
	v := tea.NewView(b.String())
	if m.taskbar {
		v.ProgressBar = taskbar
	}
	return v
}

// partsView draws a row for each stream that has started. A finished stream
// stays on screen with a green bar, and the next one appears below it. With
// more than one stream there's a total underneath. It also returns the overall
// fraction done, and whether every stream is in and yt-dlp is finishing up.
func (m model) partsView() (rows string, overall float64, finishing bool) {
	if len(m.parts) == 0 || !m.parts[0].started {
		return m.spin.View() + " Starting the download...\n", 0, false
	}
	labelWidth := lipgloss.Width("Total")
	for _, p := range m.parts {
		labelWidth = max(labelWidth, lipgloss.Width(p.label))
	}
	labelWidth += 2
	barWidth := m.lineWidth() - labelWidth

	var b strings.Builder
	var done, total, finished float64
	totalKnown, finishing := true, true
	for _, p := range m.parts {
		size := cmp.Or(p.prog.Total, p.size)
		total += size
		totalKnown = totalKnown && size > 0
		if !p.started {
			finishing = false
			continue
		}
		bar, pct := m.bar, fraction(p.prog.Done, size)
		if p.prog.Finished {
			bar, pct = m.doneBar, 1
			finished++
			done += size
		} else {
			finishing = false
			done += p.prog.Done
		}
		b.WriteString(m.fit(pad(p.label, labelWidth)+barView(bar, pct, barWidth)) + "\n")
		b.WriteString(m.fit(strings.Repeat(" ", labelWidth)+faint.Render(p.stats())) + "\n")
	}

	if totalKnown {
		overall = fraction(done, total)
	} else if current := m.parts[min(int(finished), len(m.parts)-1)]; current.started && !current.prog.Finished {
		overall = (finished + fraction(current.prog.Done, cmp.Or(current.prog.Total, current.size))) / float64(len(m.parts))
	} else {
		overall = finished / float64(len(m.parts))
	}
	if len(m.parts) > 1 && totalKnown {
		bar := m.bar
		if finishing {
			bar = m.doneBar
		}
		b.WriteString(m.fit(pad("Total", labelWidth)+barView(bar, overall, barWidth)) + "\n")
	}
	if finishing {
		label := "Finishing up..."
		if len(m.parts) > 1 {
			label = "Joining video and audio..."
		}
		b.WriteString("\n" + m.spin.View() + " " + label + "\n")
	}
	return b.String(), overall, finishing
}

func (p part) stats() string {
	size := cmp.Or(p.prog.Total, p.size)
	if p.prog.Finished {
		return human.Bytes(size) + " · done"
	}
	stats := []string{human.Bytes(p.prog.Done)}
	if size > 0 {
		stats[0] += " of " + human.Bytes(size)
	}
	if p.prog.Speed > 0 {
		stats = append(stats, human.Bytes(p.prog.Speed)+"/s")
	}
	if p.prog.ETA > 0 {
		stats = append(stats, human.Duration(p.prog.ETA)+" left")
	}
	return strings.Join(stats, " · ")
}

func barView(bar progress.Model, pct float64, width int) string {
	bar.SetWidth(max(10, width))
	return bar.ViewAs(pct)
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

// lineWidth is how wide a line may be. One column stays spare, so a terminal
// that shrinks by a column doesn't wrap the frame.
func (m model) lineWidth() int {
	if m.width <= 0 {
		return 79 // before the first WindowSizeMsg
	}
	return max(20, m.width-1)
}

// fit shortens s to one line.
func (m model) fit(s string) string { return ansi.Truncate(s, m.lineWidth(), "…") }

func (m model) videoHeader() string {
	return bold.Render(m.fit(m.info.Title)) + "\n" +
		faint.Render(m.fit(m.info.ChannelName()+" · "+human.Duration(m.info.Duration))) + "\n\n"
}

func (m model) choice(i int, label string) string {
	if i == m.cursor {
		return accent.Render("› ") + label
	}
	return "  " + label
}

func (m model) savedLine() string {
	e := m.logged
	return good.Render("Saved ") + e.File + "\n" +
		faint.Render(fmt.Sprintf("%dp · %d/%d downloads today", e.Height, m.usedToday(), m.app.Config.DailyLimit))
}

func (m model) help() string {
	switch m.stage {
	case stageLink:
		return "enter continue · esc quit"
	case stageSaved, stagePick:
		return "↑/↓ choose · enter select · esc quit"
	case stageDeps:
		return "enter install · esc quit"
	case stageReason:
		return "enter start download · esc quit"
	case stageBusy, stageInstalling, stageDownloading:
		return "esc cancel"
	case stageDone:
		return "enter play · o open folder · q quit"
	}
	return ""
}

func fraction(done, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return min(1, done/total)
}
