# ytget

Download one YouTube video at a time from a link, while YouTube stays blocked
in the Windows hosts file for everything else.

ytget is for the videos worth watching that friends share or articles link
to, without reopening the door to endless browsing. It takes links only (no
search, no recommendations), and it asks you to slow down:

- **Single videos only.** Playlist, channel, search and home-page links are
  refused, and a video link that carries a playlist gets just that video.
- **Daily limit.** 3 downloads a day by default. Reopening a video you
  already saved doesn't count.
- **A reason for every download.** It goes into your history, so you can look
  back at what you watched and why.

## How it works

The hosts file only affects the normal system lookup. While ytget is open, it
runs a small proxy on `127.0.0.1` and points [yt-dlp](https://github.com/yt-dlp/yt-dlp)
at it with `--proxy`:

- The proxy looks YouTube's hostnames up over DNS-over-HTTPS (Cloudflare
  `1.1.1.1`, with Google `8.8.8.8` as a fallback). Every other host uses the
  normal lookup.
- It only relays encrypted HTTPS connections, so it never sees what passes
  through.
- It requires a random password that changes every run, so other programs
  can't use it.
- It stops when ytget closes. Your browser and every other app still hit the
  block.

## Requirements

- Windows
- [Go](https://go.dev/dl/) 1.27 or newer, to build

ytget checks for these on first use and offers to install anything missing:

| Tool | Why | How ytget installs it |
| --- | --- | --- |
| yt-dlp | does the downloading | downloads the official `yt-dlp.exe` from GitHub and verifies its SHA-256 checksum |
| Node.js 22+ (or Deno 2.3+) | yt-dlp runs YouTube's player code with it | `winget install OpenJS.NodeJS.LTS` |
| ffmpeg | joins the separate video and audio streams | `winget install yt-dlp.FFmpeg` |

## Install

```sh
go install github.com/abhiram-ar/youtube-downloader-via-dns-over-http/cmd/ytget@latest
```

This puts `ytget.exe` in `%USERPROFILE%\go\bin`; make sure that folder is on
your PATH.

Or build from a clone:

```sh
git clone https://github.com/abhiram-ar/youtube-downloader-via-dns-over-http.git
cd youtube-downloader-via-dns-over-http
go build -trimpath -ldflags "-s -w" -o bin/ytget.exe ./cmd/ytget
```

Then add `bin` to your PATH to run `ytget` from any terminal.

## Usage

```sh
ytget                         # asks for a link
ytget "https://youtu.be/..."  # quote links in PowerShell
ytget "<link>" -q 720 -r "why I'm watching"
```

You paste a link, pick a resolution from what the video offers (with
estimated sizes), say why you're watching, and follow the progress bar. When
it's done, Enter plays the video and `o` opens its folder.

| Option | What it does |
| --- | --- |
| `-q`, `--quality` | resolution cap: 144, 240, 360, 480, 720, 1080, 1440 or 2160 (skips the picker) |
| `-r`, `--reason` | why you're watching (skips the question) |
| `--history` | today's count and your last 20 downloads |
| `--check` | check for the tools above and offer to install any that are missing |
| `--update` | update yt-dlp, which fixes most sudden breakages |

Above 1080p, YouTube only offers VP9 or AV1 video, which the Windows player
handles with its free VP9/AV1 codec extensions.

## Settings

| Path | Contents |
| --- | --- |
| `%APPDATA%\ytget\config.json` | settings, written with defaults on first run |
| `%APPDATA%\ytget\history.jsonl` | one line per download: date, title, resolution, reason, file |
| `%LOCALAPPDATA%\ytget\yt-dlp.exe` | ytget's own copy of yt-dlp |

```json
{
  "output_dir": "%USERPROFILE%\\Videos\\YT-Saved",
  "daily_limit": 3,
  "max_height": 1080,
  "min_reason_length": 10
}
```

`max_height` is the resolution the picker highlights. Set `YTGET_HOME` to keep
all three files in one folder instead.

## Development

```sh
go test ./...                                        # unit tests
go test -tags live -run Live -v ./internal/ytdlp     # real downloads through the proxy
```

```
cmd/ytget/          entry point: flags, --history, --update
internal/
  deps/             finding and installing yt-dlp, Node.js and ffmpeg
  human/            formatting sizes and durations
  link/             checking links and reducing them to one video
  platform/         Windows pieces: opening files, re-reading PATH, stopping processes
  proxy/            the private proxy and its DNS-over-HTTPS resolver
  store/            config.json, history.jsonl, and ytget's folders
  tui/              the Bubble Tea interface
  ytdlp/            running yt-dlp: lookups, resolutions, downloads with progress
```
