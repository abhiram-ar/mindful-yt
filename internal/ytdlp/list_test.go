package ytdlp

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestListArgs(t *testing.T) {
	want := []string{"-J", "--flat-playlist", "--extractor-args", "youtubetab:approximate_date",
		"-I", "1:15", "ytsearch15:cats"}
	if got := listArgs(Search("cats", 10), 15); !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestUploads(t *testing.T) {
	for handle, want := range map[string]string{
		"@jawed":  "https://www.youtube.com/@jawed/videos",
		"@a.b-c_": "https://www.youtube.com/@a.b-c_/videos",
		"@강남":     "https://www.youtube.com/@%EA%B0%95%EB%82%A8/videos",
	} {
		if got := Uploads(handle); got != want {
			t.Errorf("%q: got %q, want %q", handle, got, want)
		}
	}
}

func TestSearchQueryCantBecomeAnOption(t *testing.T) {
	cmd := Command(context.Background(), "yt-dlp", "http://mindful-yt:x@127.0.0.1:1",
		listArgs(Search("--exec rm -rf ~", 10), 15)...)
	if last := cmd.Args[len(cmd.Args)-1]; last != "ytsearch15:--exec rm -rf ~" {
		t.Errorf("last argument %q", last)
	}
	if slices.Contains(cmd.Args, "--exec") {
		t.Errorf("the query became an option: %q", cmd.Args)
	}
}

// flatSearch is shaped like yt-dlp's -J --flat-playlist output for a search,
// cut down to the fields that matter.
const flatSearch = `{"id": "cats", "title": "cats", "_type": "playlist", "extractor_key": "YoutubeSearch",
 "epoch": 1791267342, "entries": [
 {"_type": "url", "ie_key": "Youtube", "id": "aaaaaaaaaa1", "url": "https://www.youtube.com/watch?v=aaaaaaaaaa1",
  "title": "Cats being cats", "description": null, "duration": 634.0, "channel_id": "UCx", "channel": "Cat TV",
  "uploader": "Cat TV", "uploader_id": "@cattv", "thumbnails": [{"url": "https://i.ytimg.com/x.jpg"}],
  "timestamp": 1696291200, "release_timestamp": null, "availability": null, "view_count": 412000,
  "live_status": null, "channel_is_verified": true},
 {"_type": "url", "ie_key": "Youtube", "id": "aaaaaaaaaa2", "url": "https://www.youtube.com/shorts/aaaaaaaaaa2",
  "title": "Cat short", "duration": null, "channel": null, "uploader": "shortcat", "timestamp": null, "view_count": null},
 {"_type": "url", "ie_key": "Youtube", "id": "aaaaaaaaaa3", "url": "https://www.youtube.com/watch?v=aaaaaaaaaa3",
  "title": "Live cat cam", "channel": "Cam", "live_status": "is_live", "concurrent_view_count": 900},
 {"_type": "url", "ie_key": "Youtube", "id": "aaaaaaaaaa4", "url": "https://www.youtube.com/watch?v=aaaaaaaaaa4",
  "title": "Cat premiere", "channel": "Cam", "live_status": "is_upcoming", "release_timestamp": 1900000000},
 {"_type": "url", "ie_key": "Youtube", "id": "aaaaaaaaaa5", "url": "https://www.youtube.com/watch?v=aaaaaaaaaa5",
  "title": "Yesterday's cat stream", "channel": "Cam", "live_status": "was_live", "view_count": 50, "duration": 7200},
 {"_type": "url", "ie_key": "Youtube", "id": "aaaaaaaaaa6", "url": "https://www.youtube.com/watch?v=aaaaaaaaaa6",
  "title": "Members only cats", "channel": "Cat TV", "availability": "subscriber_only"},
 {"_type": "url", "ie_key": "YoutubeTab", "id": "PLcats", "url": "https://www.youtube.com/playlist?list=PLcats",
  "title": "All the cats"},
 {"_type": "url", "ie_key": "YoutubeTab", "id": "UCcats", "url": "https://www.youtube.com/channel/UCcatscatscatscatscats",
  "title": "Cat Channel"},
 {"_type": "url", "title": "A shelf with no link"},
 null,
 {"_type": "url", "ie_key": "Youtube", "id": "aaaaaaaaaa1", "url": "https://www.youtube.com/watch?v=aaaaaaaaaa1",
  "title": "Cats being cats, again"},
 {"_type": "url", "ie_key": "Youtube", "id": "aaaaaaaaaa7", "url": "https://www.youtube.com/watch?v=aaaaaaaaaa7",
  "title": "One view", "channel": "Tiny", "view_count": 1}
]}`

func TestParseListKeepsYouTubesOrderAndOnlySaveableVideos(t *testing.T) {
	videos, at, err := parseList([]byte(flatSearch), 10)
	if err != nil {
		t.Fatal(err)
	}
	if !at.Equal(time.Unix(1791267342, 0)) {
		t.Errorf("the list was made at %v, want yt-dlp's epoch", at)
	}
	var ids []string
	for _, v := range videos {
		ids = append(ids, v.ID)
	}
	if want := []string{"aaaaaaaaaa1", "aaaaaaaaaa2", "aaaaaaaaaa5", "aaaaaaaaaa7"}; !slices.Equal(ids, want) {
		t.Fatalf("got %q, want %q", ids, want)
	}
	first := ListEntry{
		ID: "aaaaaaaaaa1", URL: "https://www.youtube.com/watch?v=aaaaaaaaaa1", Title: "Cats being cats",
		Channel: "Cat TV", Uploader: "Cat TV", UploaderID: "@cattv", Duration: 634, Views: 412000, Timestamp: 1696291200,
	}
	if videos[0] != first {
		t.Errorf("got %+v, want %+v", videos[0], first)
	}
	short := videos[1]
	if short.ChannelName() != "shortcat" || short.Duration != 0 || short.Views != 0 || short.Timestamp != 0 {
		t.Errorf("short: %+v, channel %q", short, short.ChannelName())
	}
}

func TestParseListStopsAtN(t *testing.T) {
	var entries []string
	for i := range 15 {
		id := fmt.Sprintf("video%06d", i)
		entries = append(entries, fmt.Sprintf(`{"id": %q, "url": "https://www.youtube.com/watch?v=%s"}`, id, id))
	}
	videos, _, err := parseList([]byte(`{"entries": [`+strings.Join(entries, ",")+`]}`), 10)
	if err != nil || len(videos) != 10 || videos[9].ID != "video000009" {
		t.Fatalf("got %d videos, %v", len(videos), err)
	}
}

func TestParseListEmptyIsNotAnError(t *testing.T) {
	videos, at, err := parseList([]byte(`{"_type": "playlist", "entries": []}`), 10)
	if err != nil || len(videos) != 0 {
		t.Errorf("got %v, %v", videos, err)
	}
	if time.Since(at) > time.Minute { // no epoch: our own clock
		t.Errorf("made at %v", at)
	}
}

func TestParseListGarbage(t *testing.T) {
	if _, _, err := parseList([]byte("WARNING: not json"), 10); err == nil || !strings.Contains(err.Error(), "couldn't read") {
		t.Errorf("got %v", err)
	}
}

func TestSaveableLeavesOutWhatCantBeSaved(t *testing.T) {
	watch := "https://www.youtube.com/watch?v=aaaaaaaaaa1"
	for _, c := range []struct {
		e    ListEntry
		want bool
	}{
		{ListEntry{URL: watch}, true},
		{ListEntry{URL: "https://www.youtube.com/shorts/aaaaaaaaaa1"}, true},
		{ListEntry{URL: watch, LiveStatus: "was_live"}, true},
		{ListEntry{URL: watch, Availability: "public"}, true},
		{ListEntry{URL: watch, Availability: "unlisted"}, true},
		{ListEntry{URL: watch, LiveStatus: "is_live"}, false},
		{ListEntry{URL: watch, LiveStatus: "is_upcoming"}, false},
		{ListEntry{URL: watch, Availability: "private"}, false},
		{ListEntry{URL: watch, Availability: "premium_only"}, false},
		{ListEntry{URL: watch, Availability: "subscriber_only"}, false},
		{ListEntry{URL: watch, Availability: "needs_auth"}, false},
		{ListEntry{URL: "https://www.youtube.com/playlist?list=PLabc"}, false},
		{ListEntry{URL: ""}, false},
	} {
		if id, ok := saveable(c.e); ok != c.want || ok && id != "aaaaaaaaaa1" {
			t.Errorf("%+v: got %q, %v", c.e, id, ok)
		}
	}
}

func TestHandleIsOnlyAnAtName(t *testing.T) {
	for id, want := range map[string]string{"@cattv": "@cattv", "cattv": "", "UCcatscatscatscatscats": "", "": ""} {
		if got := (ListEntry{UploaderID: id}).Handle(); got != want {
			t.Errorf("%q: got %q, want %q", id, got, want)
		}
	}
}

func TestChannelNameFallsBackToTheUploader(t *testing.T) {
	if got := (ListEntry{Channel: "Cat TV", Uploader: "cattv"}).ChannelName(); got != "Cat TV" {
		t.Errorf("got %q, want the channel", got)
	}
	if got := (ListEntry{Uploader: "cattv"}).ChannelName(); got != "cattv" {
		t.Errorf("got %q, want the uploader", got)
	}
}
