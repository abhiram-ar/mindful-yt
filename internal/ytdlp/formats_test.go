package ytdlp

import (
	"slices"
	"testing"
)

func TestQualitiesPreferH264AndAddAudio(t *testing.T) {
	info := VideoInfo{Formats: []Format{
		{ID: "sb0", Ext: "mhtml", VCodec: "none", ACodec: "none", Width: 160, Height: 90},
		{ID: "140", Ext: "m4a", VCodec: "none", ACodec: "mp4a.40.2", TBR: 129, Filesize: 3e6},
		{ID: "251", Ext: "webm", VCodec: "none", ACodec: "opus", TBR: 140, Filesize: 3.5e6},
		{ID: "137", Ext: "mp4", VCodec: "avc1.640028", ACodec: "none", Width: 1920, Height: 1080, FPS: 30, TBR: 4000, Filesize: 40e6},
		{ID: "248", Ext: "webm", VCodec: "vp9", ACodec: "none", Width: 1920, Height: 1080, FPS: 30, TBR: 3000, Filesize: 30e6},
		{ID: "136", Ext: "mp4", VCodec: "avc1.4d401f", ACodec: "none", Width: 1280, Height: 720, TBR: 2000, FilesizeApprox: 20e6},
		{ID: "18", Ext: "mp4", VCodec: "avc1.42001E", ACodec: "mp4a.40.2", Width: 640, Height: 360, TBR: 500, Filesize: 5e6},
	}}
	want := []Quality{{1080, 43e6}, {720, 23e6}, {360, 5e6}}
	if got := Qualities(info); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestVerticalVideoCountsItsShortSide(t *testing.T) {
	info := VideoInfo{Formats: []Format{{VCodec: "avc1", ACodec: "none", Width: 1080, Height: 1920}}}
	if got := Qualities(info); len(got) != 1 || got[0].Res != 1080 {
		t.Errorf("got %v", got)
	}
}

func TestDefaultQualityIndex(t *testing.T) {
	qs := func(res ...int) []Quality {
		var q []Quality
		for _, r := range res {
			q = append(q, Quality{Res: r})
		}
		return q
	}
	for _, c := range []struct {
		qualities []Quality
		limit     int
		want      int
	}{
		{qs(2160, 1440, 1080, 720), 1080, 2},
		{qs(480, 360), 1080, 0},
		{qs(1440), 1080, 0}, // nothing fits: take the smallest there is
	} {
		if got := DefaultQualityIndex(c.qualities, c.limit); got != c.want {
			t.Errorf("%v up to %d: got %d, want %d", c.qualities, c.limit, got, c.want)
		}
	}
}
