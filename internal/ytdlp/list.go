package ytdlp

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/abhiram-ar/mindful-yt/internal/link"
)

// ListEntry is one video in a list yt-dlp made without looking each video up
// (--flat-playlist): search results, or a channel's videos.
type ListEntry struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"` // a watch or /shorts/ link
	Title        string  `json:"title"`
	Channel      string  `json:"channel"`
	Uploader     string  `json:"uploader"`
	UploaderID   string  `json:"uploader_id"` // the channel's handle, "@name", when YouTube gave it
	Duration     float64 `json:"duration"`    // seconds; 0 if YouTube didn't say
	Views        float64 `json:"view_count"`  // 0 if YouTube didn't say
	Timestamp    float64 `json:"timestamp"`   // approximate, from YouTube's "3 weeks ago"; 0 if unknown
	LiveStatus   string  `json:"live_status"`
	Availability string  `json:"availability"`
}

func (e ListEntry) ChannelName() string { return cmp.Or(e.Channel, e.Uploader) }

// Handle is the channel's "@name", or "" when the search result didn't carry
// it, which is often: about half the results in some searches.
func (e ListEntry) Handle() string {
	if strings.HasPrefix(e.UploaderID, "@") {
		return e.UploaderID
	}
	return ""
}

// spare is how many more than n a list asks for, since live, upcoming and
// members-only videos are left out: measured, that's 0 to 2 in 20 search
// results, almost always live streams. YouTube sends 20 search results a
// request, so 15 and 5 spare is one request.
const spare = 5

// Search is the List target for a YouTube search. The "ytsearch" prefix
// means a query can never be read as an option.
func Search(query string, n int) string { return fmt.Sprintf("ytsearch%d:%s", n+spare, query) }

// Uploads is the List target for a channel's videos, newest first, from its
// handle ("@name"). Shorts and live streams have tabs of their own.
func Uploads(handle string) string {
	return "https://www.youtube.com/" + url.PathEscape(handle) + "/videos"
}

// List asks yt-dlp for the videos at target, in the order YouTube gives them,
// and keeps the first n that can be saved. at is yt-dlp's clock when it made
// the list, which the approximate timestamps count back from.
func List(ctx context.Context, path, proxyURL, target string, n int) (videos []ListEntry, at time.Time, err error) {
	out, err := output(ctx, path, proxyURL, listArgs(target, n+spare)...)
	if err != nil {
		return nil, time.Time{}, err
	}
	return parseList(out, n)
}

// listArgs lists the first most videos at target in one go, without looking
// each video up. A search stops by itself, but a channel's videos come a page
// at a time until the channel runs out: -I stops yt-dlp asking for more pages
// once it has enough. YouTube gives each video's age only as "3 weeks ago",
// which approximate_date turns into a timestamp; human.Ago turns it back into
// words.
func listArgs(target string, most int) []string {
	return []string{"-J", "--flat-playlist", "--extractor-args", "youtubetab:approximate_date",
		"-I", fmt.Sprintf("1:%d", most), target}
}

func parseList(out []byte, n int) ([]ListEntry, time.Time, error) {
	var list struct {
		Entries []*ListEntry `json:"entries"`
		Epoch   int64        `json:"epoch"` // when yt-dlp wrote the list out
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, time.Time{}, fmt.Errorf("couldn't read yt-dlp's answer: %w", err)
	}
	at := time.Now()
	if list.Epoch > 0 {
		at = time.Unix(list.Epoch, 0)
	}
	var videos []ListEntry
	seen := map[string]bool{}
	for _, e := range list.Entries {
		if len(videos) == n {
			break
		}
		if e == nil {
			continue
		}
		id, ok := saveable(*e)
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		videos = append(videos, *e)
	}
	return videos, at, nil
}

// saveable reports whether e is a single video mindful-yt can save, and its
// ID. Live and upcoming streams aren't finished videos, and members-only or
// private ones fail when looked up.
func saveable(e ListEntry) (id string, ok bool) {
	id, _, err := link.Canonical(e.URL)
	if err != nil { // a playlist, a channel, or no link at all
		return "", false
	}
	switch e.LiveStatus {
	case "is_live", "is_upcoming":
		return "", false
	}
	switch e.Availability {
	case "private", "premium_only", "subscriber_only", "needs_auth":
		return "", false
	}
	return id, true
}
