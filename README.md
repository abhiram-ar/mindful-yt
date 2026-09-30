## ytget

Download a single YouTube video while YouTube stays blocked in your hosts file,
on Windows, macOS or Linux. yt-dlp runs through a private local proxy that
looks YouTube up over DNS-over-HTTPS, so nothing else on your machine gets past
the block.

Single videos only, a daily limit, and a reason for every download.

## Disclaimer - AI-led project

This is personal software built to solve a problem of my own. 
I kept losing hours to YouTube, so I blocked it in my OS DNS resolver (poiting youtube.com -> 0.0.0.0). 
But friends and articles I read, recommend videos that are worth watching. 
ytget solves this: it downloads only the one video I ask for and leaves the block in place, 
so I can watch what matters without getting pulled back into the feed.

If you're in the same spot, or you just want a YouTube downloader that works
even when YouTube is blocked, you're welcome to use it. It's provided as is,
without warranty.


## Installation

**macOS and Linux**, in a terminal:

```sh
repo=abhiram-ar/youtube-downloader-via-dns-over-http
curl -fsSL "https://raw.githubusercontent.com/$repo/main/install.sh" | sh
```
<br/>

**Windows**, in PowerShell:

```powershell
$repo = 'abhiram-ar/youtube-downloader-via-dns-over-http'
irm "https://raw.githubusercontent.com/$repo/main/install.ps1" | iex
```

> Both download the latest release for your system, check it against the
published checksums, and install ytget. Run them again to update.

<br/>

With Go installed, this works too:

```sh
go install github.com/abhiram-ar/youtube-downloader-via-dns-over-http/cmd/ytget@latest
```


## Usage

```sh
ytget "https://youtu.be/..."
```

> ytget offers to install yt-dlp, Deno and ffmpeg if they're missing.

<br/>

## Settings

ytget creates `config.json` on first run, next to your download history, in:

- **Windows:** `%APPDATA%\ytget`
- **macOS:** `~/Library/Application Support/ytget`
- **Linux:** `~/.config/ytget`

`ytget --help` prints the exact path.

| Setting | Default | What it does |
| --- | --- | --- |
| `output_dir` | `Videos/YT-Saved` in your home folder (`Movies/YT-Saved` on macOS) | Where videos are saved. `~` and `%VAR%` are expanded. |
| `daily_limit` | `3` | How many downloads you get per day. |
| `max_height` | `1080` | The resolution the picker highlights. |
| `min_reason_length` | `10` | The shortest reason accepted. |
