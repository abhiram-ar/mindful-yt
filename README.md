# mindful-yt

Keep YouTube blocked on your machine, and still watch the videos that matter.

mindful-yt downloads a single YouTube video even when YouTube's DNS is
blocked, on Windows, macOS or Linux. It runs `yt-dlp` through a private
local proxy that looks YouTube up over DNS-over-HTTPS, so your block
stays in place.

Single videos only, a daily limit, and a reason for every download.


## Disclaimer - AI-led project

This is personal software, built to solve a problem of my own. I kept losing
hours to YouTube, so I blocked it in my OS's DNS resolver (pointing
youtube.com to 0.0.0.0). But friends recommend videos worth watching, and
sometimes I need a video to learn about a specific topic. `mindful-yt` solves
this: it downloads only the one video I ask for, with no recommendations and
leaves the block in place, so we can watch what matters without getting pulled
back into the feed. And with a little friction before every download, we only
end up with videos worth watching.

If you're in the same spot, or you just want a YouTube downloader that works
even when YouTube is blocked, you're welcome to use it. It's provided as is,
without warranty.


## Installation

**macOS and Linux**:

```sh
repo=abhiram-ar/mindful-yt
curl -fsSL "https://raw.githubusercontent.com/$repo/main/install.sh" | sh
```
<br/>

**Windows**, in PowerShell:

```powershell
$repo = 'abhiram-ar/mindful-yt'
irm "https://raw.githubusercontent.com/$repo/main/install.ps1" | iex
```

> Both download the latest release for your system, check it against the
> published checksums, and install mindful-yt. Run them again to update.

<br/>

If you have Golang installed, this works too:

```sh
go install github.com/abhiram-ar/mindful-yt/cmd/mindful-yt@latest
```


## Usage

```sh
mindful-yt "https://youtu.be/dQw4w9WgXcQ"   # a video link
mindful-yt "lofi hip hop"                   # search YouTube, then pick a video
mindful-yt @jawed                           # a channel's newest videos, then pick one
```

Or run `mindful-yt` on its own and type any of these.

> mindful-yt offers to install yt-dlp, Deno and ffmpeg if they're missing.

<br/>

YouTube not blocked yet? This blocks it in your OS's hosts file but mindful-yt keeps working:

```sh
mindful-yt lock-me-in
```


## Settings

mindful-yt creates `config.json` on first run, next to your download history, in:

- **Windows:** `%APPDATA%\mindful-yt`
- **macOS:** `~/Library/Application Support/mindful-yt`
- **Linux:** `~/.config/mindful-yt`

`mindful-yt --help` prints the exact path.

| Setting | Default | What it does |
| --- | --- | --- |
| `output_dir` | `Videos/YT-Saved` in your home folder (`Movies/YT-Saved` on macOS) | Where videos are saved. `~` and `%VAR%` are expanded. |
| `daily_limit` | `3` | How many downloads you get per day. |
| `max_height` | `1080` | The resolution the picker highlights. |
| `min_reason_length` | `10` | The shortest reason accepted. |
| `search_results` | `15` | How many videos a search or a channel lists (at most 50). |
