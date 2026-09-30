package link

import "testing"

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
