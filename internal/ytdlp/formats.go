package ytdlp

import (
	"slices"
	"strings"
)

// VideoInfo is the part of yt-dlp's -J output that ytget uses.
type VideoInfo struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Channel  string   `json:"channel"`
	Uploader string   `json:"uploader"`
	Duration float64  `json:"duration"`
	Formats  []Format `json:"formats"`
}

func (v VideoInfo) ChannelName() string {
	if v.Channel != "" {
		return v.Channel
	}
	return v.Uploader
}

// Format is one of the streams YouTube offers for a video.
type Format struct {
	ID             string  `json:"format_id"`
	Ext            string  `json:"ext"`
	VCodec         string  `json:"vcodec"`
	ACodec         string  `json:"acodec"`
	Width          float64 `json:"width"`
	Height         float64 `json:"height"`
	FPS            float64 `json:"fps"`
	TBR            float64 `json:"tbr"`
	Filesize       float64 `json:"filesize"`
	FilesizeApprox float64 `json:"filesize_approx"`
}

func (f Format) size() float64 {
	if f.Filesize > 0 {
		return f.Filesize
	}
	return f.FilesizeApprox
}

func (f Format) hasVideo() bool { return f.VCodec != "none" && f.Height > 0 && f.Ext != "mhtml" }
func (f Format) hasAudio() bool { return f.ACodec != "none" && f.ACodec != "" }

// res is the frame's short side, which is what yt-dlp's -S res: compares, so
// a vertical 1080x1920 video counts as 1080p.
func (f Format) res() int {
	if f.Width > 0 && f.Width < f.Height {
		return int(f.Width)
	}
	return int(f.Height)
}

// Quality is a resolution on offer.
type Quality struct {
	Res  int
	Size float64 // rough bytes for video plus audio; 0 if YouTube didn't say
}

// Qualities lists the resolutions on offer, highest first. Sizes follow the
// download's preferences: H.264 video, m4a audio.
func Qualities(info VideoInfo) []Quality {
	best := map[int]Format{}
	var audio Format
	for _, f := range info.Formats {
		switch {
		case f.hasVideo():
			if cur, ok := best[f.res()]; !ok || betterVideo(f, cur) {
				best[f.res()] = f
			}
		case f.hasAudio() && f.Ext == "m4a" && f.TBR >= audio.TBR:
			audio = f
		}
	}
	var qualities []Quality
	for res, f := range best {
		size := f.size()
		if size > 0 && !f.hasAudio() {
			size += audio.size()
		}
		qualities = append(qualities, Quality{Res: res, Size: size})
	}
	slices.SortFunc(qualities, func(a, b Quality) int { return b.Res - a.Res })
	return qualities
}

func betterVideo(a, b Format) bool {
	aH264, bH264 := strings.HasPrefix(a.VCodec, "avc1"), strings.HasPrefix(b.VCodec, "avc1")
	if aH264 != bH264 {
		return aH264
	}
	if a.FPS != b.FPS {
		return a.FPS > b.FPS
	}
	return a.TBR > b.TBR
}

// DefaultQualityIndex picks the highest quality that fits limit, else the smallest.
func DefaultQualityIndex(qualities []Quality, limit int) int {
	for i, q := range qualities {
		if q.Res <= limit {
			return i
		}
	}
	return len(qualities) - 1
}
