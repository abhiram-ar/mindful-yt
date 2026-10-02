package link

import (
	"errors"
	"strings"
	"testing"
)

const testID = "jNQXAC9IVRw"

func TestSingleVideoLinksAreRewritten(t *testing.T) {
	want := "https://www.youtube.com/watch?v=" + testID
	for _, text := range []string{
		"https://youtu.be/" + testID,
		"https://youtu.be/" + testID + "?si=abc123&t=5",
		"youtu.be/" + testID,
		want,
		"https://www.youtube.com/watch?v=" + testID + "&list=PLabc&index=3",
		"https://www.youtube.com/watch?feature=share&v=" + testID,
		"https://m.youtube.com/watch?v=" + testID,
		"https://music.youtube.com/watch?v=" + testID,
		"https://youtube.com/shorts/" + testID,
		"https://www.youtube.com/live/" + testID + "?si=x",
		"https://www.youtube.com/embed/" + testID,
		"  " + want + "  ",
	} {
		id, url, err := Canonical(text)
		if err != nil || id != testID || url != want {
			t.Errorf("%q: got %q, %q, %v", text, id, url, err)
		}
	}
}

func TestEverythingElseIsRefused(t *testing.T) {
	for _, text := range []string{
		"https://www.youtube.com/",
		"https://www.youtube.com/playlist?list=PLabc",
		"https://www.youtube.com/@somechannel",
		"https://www.youtube.com/@somechannel/shorts",
		"https://www.youtube.com/channel/UCabcdefghijklmnopqrstuv",
		"https://www.youtube.com/c/Something",
		"https://www.youtube.com/user/someone",
		"https://www.youtube.com/results?search_query=cats",
		"https://www.youtube.com/watch?list=PLabc",
		"https://www.youtube.com/watch?v=tooShort",
		"https://youtu.be/",
		"https://vimeo.com/123456",
		"https://notyoutube.com/watch?v=" + testID,
	} {
		if _, _, err := Canonical(text); err == nil {
			t.Errorf("%q was accepted", text)
		}
	}
}

// The interface searches YouTube for anything that isn't a YouTube link, and
// refuses YouTube links that aren't to a single video, so the two errors
// must split the way it expects.
func TestWordsAndOtherSitesAreNotYouTube(t *testing.T) {
	for _, text := range []string{
		"cute cats", "@mkbhd", "", "c++ tutorial", "how to tie a tie?", "강남스타일",
		"100% orange juice", "https://vimeo.com/123456",
	} {
		if _, _, err := Canonical(text); !errors.Is(err, ErrNotYouTube) {
			t.Errorf("%q: got %v, want ErrNotYouTube", text, err)
		}
	}
	for _, text := range []string{"youtube.com", "https://www.youtube.com/@someone", "https://www.youtube.com/playlist?list=PLabc"} {
		if _, _, err := Canonical(text); !errors.Is(err, ErrNotSingleVideo) {
			t.Errorf("%q: got %v, want ErrNotSingleVideo", text, err)
		}
	}
}

func TestHandles(t *testing.T) {
	for text, want := range map[string]string{
		"@jawed": "@jawed", "  @jawed ": "@jawed", "@MBCkpop": "@MBCkpop", "@a.b-c_d": "@a.b-c_d",
		"@강남스타일": "@강남스타일", "@abc": "@abc", "@" + strings.Repeat("a", 30): "@" + strings.Repeat("a", 30),
	} {
		if got, ok := Handle(text); !ok || got != want {
			t.Errorf("%q: got %q, %v; want %q", text, got, ok, want)
		}
	}
	for _, text := range []string{
		"@", "@ab", "@" + strings.Repeat("a", 31), "@jawed zoo", "@jawed/videos", "@@jawed",
		"jawed", "", "https://www.youtube.com/@jawed", "@jawed?x=1",
	} {
		if got, ok := Handle(text); ok {
			t.Errorf("%q was taken as the handle %q", text, got)
		}
	}
}
