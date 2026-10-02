package tui

// Searching YouTube, or listing a channel's newest videos, and picking one. A
// list stays apart from a download until a video is picked: then it goes on
// as if its link had been pasted.

import (
	"cmp"
	"context"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abhiram-ar/mindful-yt/internal/deps"
	"github.com/abhiram-ar/mindful-yt/internal/human"
	"github.com/abhiram-ar/mindful-yt/internal/ytdlp"
)

// videoList is what's being listed, and the words that go with it.
type videoList struct {
	target   string // what yt-dlp lists, e.g. "ytsearch20:cats"
	heading  string // above the videos
	busy     string // while yt-dlp works
	empty    string // when there's nothing to pick
	failed   string // before yt-dlp's error
	missing  string // when YouTube says there's no such page; "": use failed
	channels bool   // the videos come from many channels, so each row says whose
}

func searchList(query string, n int) videoList {
	q := `"` + query + `"`
	return videoList{
		target:   ytdlp.Search(query, n),
		heading:  "Results for " + q + ":",
		busy:     "Searching YouTube for " + q + "...",
		empty:    "No videos found for " + q + ".",
		failed:   "Couldn't search YouTube: ",
		channels: true,
	}
}

func channelList(handle string) videoList {
	return videoList{
		target:  ytdlp.Uploads(handle),
		heading: "Newest videos from " + handle + ":",
		busy:    "Listing " + handle + "'s videos...",
		empty:   handle + " has no videos to list.",
		failed:  "Couldn't list " + handle + "'s videos: ",
		missing: "There's no channel " + handle + " on YouTube.",
	}
}

// listVideos is ytdlp.List; tests replace it.
var listVideos = ytdlp.List

type listedMsg struct {
	seq    int // which fetch this answers; see model.listings
	videos []ytdlp.ListEntry
	at     time.Time // when YouTube's "3 weeks ago" was read
	err    error
}

// startSearch searches YouTube for text.
func (m model) startSearch(text string) (model, tea.Cmd) {
	m.link.SetValue(strings.TrimSpace(oneLine(text)))
	m.link.CursorEnd()
	query := m.link.Value() // cut to the box's length, so the search is what the box shows
	if query == "" {
		m.stage, m.problem = stageLink, "Paste a YouTube link, or type what to search for."
		return m, m.link.Focus()
	}
	return m.startList(searchList(query, m.app.Config.SearchResults))
}

// startChannel lists the newest videos of the channel with handle "@name".
func (m model) startChannel(handle string) (model, tea.Cmd) {
	m.link.SetValue(handle)
	m.link.CursorEnd()
	return m.startList(channelList(handle))
}

// startList lists l's videos. A list is how a download starts, so the daily
// limit and the tools come first, as for a pasted link.
func (m model) startList(l videoList) (model, tea.Cmd) {
	m.list, m.problem = l, ""
	m.videoID, m.watchURL, m.saved = "", "", nil
	m.link.Blur()
	return m.startDownloadFlow()
}

// fetchList has yt-dlp list the videos, once the tools are there.
func (m model) fetchList() (model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.app.Ctx)
	m.cancel = cancel
	m.listings++
	m.stage, m.busyLabel = stageListing, m.list.busy
	seq, exe, proxyURL, target, n := m.listings, deps.YtdlpPath(m.app.Tools), m.app.ProxyURL, m.list.target,
		m.app.Config.SearchResults
	return m, func() tea.Msg {
		videos, at, err := listVideos(ctx, exe, proxyURL, target, n)
		return listedMsg{seq: seq, videos: videos, at: at, err: err}
	}
}

func (m model) listed(msg listedMsg) (model, tea.Cmd) {
	if msg.seq != m.listings || m.stage != stageListing {
		return m, nil // the answer to a search the user has left
	}
	switch {
	case msg.err != nil && m.list.missing != "" && strings.Contains(msg.err.Error(), "HTTP Error 404"):
		return m.backToSearch(m.list.missing)
	case msg.err != nil:
		return m.backToSearch(m.list.failed + msg.err.Error())
	case len(msg.videos) == 0:
		return m.backToSearch(m.list.empty)
	}
	m = m.stopListing()
	m.stage, m.results = stageResults, newResults(msg.videos, msg.at, m.list.channels)
	return m.sizeResults(), nil
}

// backToSearch returns to the link box, keeping what was typed, so a search
// can be changed and tried again.
func (m model) backToSearch(problem string) (model, tea.Cmd) {
	m = m.stopListing()
	m.stage, m.problem = stageLink, problem
	return m, m.link.Focus()
}

// stopListing stops yt-dlp if it's still listing.
func (m model) stopListing() model {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	return m
}

// videoItem is a listed video as the list shows it. Its words are worked
// out once, so "3 weeks ago" stays put while the screen is up.
type videoItem struct {
	ytdlp.ListEntry
	title, desc string
}

func (v videoItem) Title() string       { return v.title }
func (v videoItem) Description() string { return v.desc }
func (v videoItem) FilterValue() string { return v.title }

// newResults is bubbles' list with only its rows and page dots: the header,
// heading and help line are mindful-yt's own, as on the other screens, and so
// are the keys that quit or go back. at is when YouTube said "3 weeks ago";
// channels says whether each row names its channel.
func newResults(videos []ytdlp.ListEntry, at time.Time, channels bool) list.Model {
	items := make([]list.Item, len(videos))
	for i, v := range videos {
		items[i] = videoItem{v, oneLine(cmp.Or(v.Title, "Untitled video")), details(v, at, channels)}
	}
	// The list's own colours are fixed for dark terminals; these follow the
	// terminal's, like the rest of mindful-yt.
	d := list.NewDefaultDelegate()
	pink := lipgloss.Color("212")
	d.Styles.NormalTitle = lipgloss.NewStyle().PaddingLeft(2)
	d.Styles.NormalDesc = d.Styles.NormalTitle.Faint(true)
	d.Styles.SelectedTitle = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(pink).Foreground(pink).PaddingLeft(1)
	d.Styles.SelectedDesc = d.Styles.SelectedTitle.Faint(true)

	l := list.New(items, d, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings() // its quit skips mindful-yt's: the exit code, the last screen
	l.KeyMap.ShowFullHelp.SetEnabled(false)
	l.KeyMap.CloseFullHelp.SetEnabled(false)
	l.Paginator.ActiveDot = accent.Render("•")
	l.Paginator.InactiveDot = faint.Render("•")
	return l
}

// sizeResults fits the list to the terminal. Everything else on the screen
// takes six lines: the header, a blank, the heading, a blank, the help line
// and the empty line after it (a frame taller than the terminal loses its top
// lines). Each video takes three, its two and a gap. When they don't all
// fit, the list pages, with a line of dots.
func (m model) sizeResults() model {
	const perVideo = 3
	room := cmp.Or(m.height, 24) - 6 // 24: before the first WindowSizeMsg
	needed := len(m.results.Items()) * perVideo
	paged := needed > room
	m.results.SetShowPagination(paged)
	if paged { // whole videos, then the dots
		room = max(1, (room-1)/perVideo)*perVideo + 1
	} else {
		room = needed
	}
	m.results.SetSize(m.lineWidth(), room)
	return m
}

func (m model) listView() string {
	// The list pads its last page to full height; past the dots, or the last
	// video, that's only blank lines.
	return m.fit(m.list.heading) + "\n" + strings.TrimRight(m.results.View(), " \n") + "\n"
}

// details is a video's second line: the channel's handle (its name when
// YouTube didn't give the handle) if withChannel, views, age and length,
// leaving out whatever YouTube didn't say.
func details(v ytdlp.ListEntry, now time.Time, withChannel bool) string {
	var parts []string
	if channel := oneLine(cmp.Or(v.Handle(), v.ChannelName())); withChannel && channel != "" {
		parts = append(parts, channel)
	}
	switch {
	case v.Views == 1:
		parts = append(parts, "1 view")
	case v.Views > 1:
		parts = append(parts, human.Count(v.Views)+" views")
	}
	if v.Timestamp > 0 {
		parts = append(parts, human.Ago(time.Unix(int64(v.Timestamp), 0), now))
	}
	if v.Duration > 0 {
		parts = append(parts, human.Duration(v.Duration))
	}
	return strings.Join(parts, " · ")
}

// oneLine turns control characters into spaces. A newline in a title would
// take an extra line on screen, and an escape would reach the terminal.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
