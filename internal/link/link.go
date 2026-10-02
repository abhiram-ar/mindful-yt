// Package link checks YouTube links and reduces them to a single video, and
// recognises a channel's handle.
package link

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	videoIDRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	youtubeHosts = map[string]bool{
		"youtube.com": true, "www.youtube.com": true, "m.youtube.com": true, "music.youtube.com": true,
	}
	shortHosts = map[string]bool{"youtu.be": true, "www.youtu.be": true}
	idPrefixes = map[string]bool{"shorts": true, "live": true, "embed": true, "v": true}
	// A YouTube handle is 3 to 30 letters (in any script), digits, "_", "-"
	// and ".", after the "@".
	handleRe = regexp.MustCompile(`^@[\p{L}\p{M}\p{N}_.-]{3,30}$`)
)

// The messages are shown to the user as they are.
var (
	ErrNotYouTube     = errors.New("That's not a YouTube link.")
	ErrNotSingleVideo = errors.New("Only links to a single video are allowed " +
		"(no playlists, channels, search results or the home page).")
)

// Handle returns text as a channel's handle, "@name", if that's all it is.
// "@name and more words" isn't a handle; it's left to be searched for.
func Handle(text string) (string, bool) {
	text = strings.TrimSpace(text)
	return text, handleRe.MatchString(text)
}

// Canonical returns the video ID and a plain watch URL for a single-video
// link. The rewrite drops list=, si= and everything else, so a link that
// carries a playlist still gets only the one video.
func Canonical(text string) (id, watchURL string, err error) {
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	u, err := url.Parse(text)
	if err != nil {
		return "", "", ErrNotYouTube
	}
	host := strings.ToLower(u.Hostname())
	var segments []string
	for _, s := range strings.Split(u.Path, "/") {
		if s != "" {
			segments = append(segments, s)
		}
	}

	switch {
	case shortHosts[host]:
		if len(segments) == 1 {
			id = segments[0]
		}
	case youtubeHosts[host]:
		if len(segments) == 1 && segments[0] == "watch" {
			id = u.Query().Get("v")
		} else if len(segments) == 2 && idPrefixes[segments[0]] {
			id = segments[1]
		}
	default:
		return "", "", ErrNotYouTube
	}

	if !videoIDRe.MatchString(id) {
		return "", "", ErrNotSingleVideo
	}
	return id, "https://www.youtube.com/watch?v=" + id, nil
}
