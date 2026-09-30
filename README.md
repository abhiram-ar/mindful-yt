# ytget

Download a single YouTube video on Windows while YouTube stays blocked in your
hosts file. yt-dlp runs through a private local proxy that looks YouTube up
over DNS-over-HTTPS, so nothing else on your machine gets past the block.

Single videos only, a daily limit, and a reason for every download.

## Disclaimer

This is personal software, built for a problem I had. I kept losing hours to
YouTube, so I blocked it in my hosts file, but friends and articles still send
me videos worth watching. ytget is my fix: it downloads the one video I ask
for and leaves the block in place.

If you're in the same spot, or you just want a YouTube downloader that works
even when YouTube is blocked, you're welcome to use it. It's provided as is,
without warranty.

## Install

Needs [Go](https://go.dev/dl/).

```sh
go install github.com/abhiram-ar/youtube-downloader-via-dns-over-http/cmd/ytget@latest
```

## Use

```sh
ytget "https://youtu.be/..."
```

ytget offers to install yt-dlp, Node.js and ffmpeg if they're missing. Videos
go to `Videos\YT-Saved`; to change that or the daily limit of 3, edit
`config.json` in the settings folder that `ytget --help` shows.
