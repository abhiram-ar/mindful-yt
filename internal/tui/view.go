package tui

import (
	"cmp"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

	switch m.stage {
	case stageLink:
		b.WriteString("Paste a YouTube link:\n" + m.link.View() + "\n")

	case stageSaved:
		b.WriteString(bold.Render(m.saved[0].Title) + "\n\nYou already have this video:\n")
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
		_, wingetErr := exec.LookPath("winget")
		for _, d := range m.missing {
			how := "downloads the official yt-dlp.exe from GitHub and checks its checksum"
			switch {
			case d.Winget != "" && wingetErr == nil:
				how = "runs: winget " + strings.Join(deps.WingetArgs(d.Winget), " ")
			case d.Winget != "":
				how = "winget isn't available; install it yourself: " + d.Manual
			}
			fmt.Fprintf(&b, "  %s %s\n    %s\n", bold.Render(d.Name), faint.Render("("+d.Why+")"), faint.Render(how))
		}

	case stageInstalling:
		b.WriteString("Downloading yt-dlp from GitHub\n\n")
		b.WriteString(m.bar.ViewAs(fraction(float64(m.installed), float64(m.needed))) + "\n")
		b.WriteString(faint.Render(human.Bytes(float64(m.installed))+" of "+human.Bytes(float64(m.needed))) + "\n")

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
		p := m.prog
		if p.Finished && p.Part == p.Parts {
			b.WriteString(m.spin.View() + " Finishing up...\n")
			break
		}
		b.WriteString(m.bar.ViewAs(fraction(p.Done, p.Total)) + "\n")
		stats := []string{fmt.Sprintf("part %d of %d", max(1, p.Part), max(1, p.Parts))}
		if p.Total > 0 {
			stats = append(stats, human.Bytes(p.Done)+" of "+human.Bytes(p.Total))
		}
		if p.Speed > 0 {
			stats = append(stats, human.Bytes(p.Speed)+"/s")
		}
		if p.ETA > 0 {
			stats = append(stats, human.Duration(p.ETA)+" left")
		}
		b.WriteString(faint.Render(strings.Join(stats, " · ")) + "\n")

	case stageDone:
		b.WriteString(m.savedLine() + "\n")
	}

	if m.problem != "" {
		b.WriteString("\n" + bad.Render(m.problem) + "\n")
	}
	if help := m.help(); help != "" {
		b.WriteString("\n" + faint.Render(help) + "\n")
	}
	return tea.NewView(b.String())
}

func (m model) videoHeader() string {
	return bold.Render(m.info.Title) + "\n" +
		faint.Render(m.info.ChannelName()+" · "+human.Duration(m.info.Duration)) + "\n\n"
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
