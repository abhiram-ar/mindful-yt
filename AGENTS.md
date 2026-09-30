# AGENTS.md

Notes for coding agents working on ytget. The README is for users; keep it
short and put development detail here.

## What this is

ytget is a Windows terminal app (Go, Bubble Tea v2) that downloads one YouTube
video at a time while YouTube stays blocked in the hosts file. It runs the
official `yt-dlp.exe` as a subprocess with `--proxy` pointing at a private
proxy on `127.0.0.1`.

The proxy resolves `youtube.com`, `youtu.be` and their subdomains over
DNS-over-HTTPS (Cloudflare `1.1.1.1`, then Google `8.8.8.8`; IPv4 only, cached
per run). The hostnames are `overrideDomains` in `internal/proxy/resolver.go`.
Every other host is tunneled with the normal system lookup, including
`*.googlevideo.com`, where the video data comes from. If the hosts file starts
blocking that too, downloads fail after the lookup succeeds; add it to
`overrideDomains`.

The release `yt-dlp.exe` bundles the JS challenge solver (`yt_dlp_ejs`) and
`curl_cffi`, and all of it honours `--proxy`. `BaseArgs` adds
`--js-runtimes node` and yt-dlp enables Deno by default, so either works
(`deps` accepts Node 22+ or Deno 2.3+).

## Layout

```
cmd/ytget/          entry point: flags; --history and --update run here; everything else starts the proxy and the TUI (which also handles --check)
internal/
  deps/             checking for ytget's own yt-dlp.exe, and Node.js/Deno and ffmpeg on PATH; downloading yt-dlp.exe (SHA-256 checked); winget ids
  human/            formatting sizes and durations
  link/             validating links and reducing them to one watch URL
  platform/         Windows only: ShellExecute, re-reading PATH from the registry, taskkill /T
  proxy/            CONNECT proxy (proxy.go) and DNS-over-HTTPS resolver (resolver.go)
  store/            config.json, history.jsonl, ytget's folders (Dirs)
  tui/              Bubble Tea model (model.go), flow (steps.go), rendering (view.go)
  ytdlp/            yt-dlp arguments, probe (-J), download with progress parsing, formats
```

## Flow

`internal/tui/steps.go`, in order:

1. `acceptLink`: `link.Canonical`, then offers any copies already saved.
2. `startDownloadFlow`: the daily limit.
3. `checkDeps` / `installNext`: downloads yt-dlp.exe, or runs winget through
   `tea.ExecProcess`, then `platform.RefreshPath`.
4. `startProbe`: `yt-dlp -J`.
5. The resolution picker.
6. `acceptReason`.
7. `startDownload`: passes the probe's JSON back with `--load-info-json`, so
   YouTube is only asked once.
8. `downloaded`: appends to the history.

Each command-line option that's set fills in its step and skips that screen.

## Commands

```sh
gofmt -l .                                          # must print nothing
go vet ./...
go vet -tags live ./...
go test ./...
go test -tags live -run Live -v ./internal/ytdlp    # network: GitHub, and YouTube through the proxy
go build -trimpath -ldflags "-s -w" -o bin/ytget.exe ./cmd/ytget
```

**Live tests.** One downloads a 144p copy of a short video into a temp folder;
the others only simulate. They need Node.js (or Deno) and ffmpeg on PATH. On
first run they install `yt-dlp.exe` into ytget's tools folder
(`%LOCALAPPDATA%\ytget`, or `YTGET_HOME` if set).

Unit tests never run yt-dlp. So run the live tests after changing
`internal/ytdlp`, `internal/proxy` or `deps.InstallYtdlp`. Put any new network
test in `live_test.go`, behind `//go:build live` and named `TestLive...`, so
`go test ./...` stays offline.

**Releases.** There are no tags, CI or release binaries. Users run
`go install ...@latest`, which builds the head of `main`, so anything pushed to
`main` ships. Run the checks above before pushing. `bin/` is gitignored and
only for local builds.

## Rules

- **Windows only.** `internal/platform` has only a `_windows.go` file, so the
  module doesn't build elsewhere.
- **YouTube is blocked on the dev machine.** Don't run yt-dlp, curl or a
  browser against YouTube directly, and never edit the hosts file. To reach
  YouTube, go through the proxy, as `liveSetup` in
  `internal/ytdlp/live_test.go` does.
- **Don't touch the user's real data.** `%APPDATA%\ytget\history.jsonl` drives
  the daily limit, `config.json` holds their settings, and
  `%LOCALAPPDATA%\ytget\yt-dlp.exe` is their copy of yt-dlp. Tests use
  `t.TempDir()`, or `YTGET_HOME` to put all of ytget's files in one folder.
  - **This applies to the built binary too.** Only `--help` and `--history`
    are safe against the real folders. `--update` replaces the user's
    yt-dlp.exe, the first run writes config.json, and any download counts
    toward today's limit. Set `YTGET_HOME` to a temp folder before running
    anything else.
- **Keep the guardrails.** Single videos only, the daily limit and the required
  reason are the point of the app. Don't weaken them to make testing easier.
- **Keep the proxy locked down.** Bind to `127.0.0.1` only, require the per-run
  password, allow CONNECT only, and use DNS-over-HTTPS only for YouTube hosts.
- **Change yt-dlp's output and ytget's parsing together.** `ytdlp.DownloadArgs`
  makes yt-dlp print lines marked `ytget-progress` (`--progress-template`),
  `ytget-formats` and `ytget-done` (`--print`). If you change those arguments,
  also change:
  - `parseProgress`, which expects exactly 7 fields,
  - `parseDone`,
  - the `ytget-formats` case in `Download`, which splits on `+`,
  - their tests.

  `Qualities` in `formats.go` mirrors the `-S` sort (short side, H.264, m4a),
  so change them together. `live_test.go` repeats the `-o` template.
- **Adding a flag.** Register both the short and long name in
  `cmd/ytget/main.go`, and add it to the hand-written `usage` text; the help
  strings are empty. Anything the TUI needs goes in `tui.Options`.
- **Adding a setting.** It goes in `store.Config` and `DefaultConfig`. An old
  config.json without the key keeps the default.
- **Old history lines must still load.** Lines written by the earlier Python
  version have no `quality` or `height` (they read as 0) and may have
  `"file": null`. `TestHistoryReadsPythonEraLinesAndCountsToday` covers this.
- **You can't drive the TUI.** It needs an interactive terminal. Test flows by
  calling `Update` with messages (see `internal/tui/tui_test.go`), and check
  layout by printing `View().Content`.
- **A running ytget doesn't block a rebuild.** `go build` moves a running
  `bin/ytget.exe` aside to `bin/ytget.exe~`, and the next build cleans that up.
  The user's running copy is unaffected. Never kill it; check with
  `Get-Process ytget`.
- **Line endings are LF,** enforced by `.gitattributes`.

## Windows PowerShell 5.1 gotchas

- Double quotes inside native-command arguments get stripped. Put test scripts
  in files instead of passing code or quoted strings inline, such as
  `python -c "..."` or `-ldflags` values with embedded quotes.
- Don't pipe a commit message to `git commit -F -`: PS 5.1 pipes to native
  commands as ASCII. Write the message to a file, without `Set-Content`, and
  use `-F <file>`.
- `Set-Content -Encoding utf8` writes a BOM. `store.LoadConfig` and
  `ReadHistory` tolerate one, but `gofmt -l .` flags a Go file that has one.
  Write files with your editor tool or `[IO.File]::WriteAllText` instead.

## API notes

- **Bubble Tea v2** (`charm.land/bubbletea/v2`): `View()` returns `tea.View`
  (use `tea.NewView`), and keys arrive as `tea.KeyPressMsg` (match on
  `msg.String()`, e.g. `"enter"`, `"esc"`, `"ctrl+c"`).
- **bubbles v2 `progress`:** the width includes the percentage label.
  `ViewAs(p)` renders without animation.
- **lipgloss v2:** `lipgloss.Color("212")` returns a `color.Color`.

## Tried and rejected

Checked with yt-dlp 2026.08.19; worth rechecking after big yt-dlp changes.

- `-N` (concurrent fragments) does nothing for YouTube's adaptive formats,
  which are plain `https` with a 10 MB chunk size. `formats=dashy` with `-N 4`
  failed with HTTP 403.
- Downloading video and audio in two parallel yt-dlp processes saved about 3%
  (6.7 s including the merge, against 6.9 s at 720p). Most of the time goes to
  start-up and extraction.
- Running the merge ourselves (`ffmpeg -i <video> -i <audio> -c copy
  -map 0:v:0 -map 1:a:0 -movflags +faststart <out>`, yt-dlp's own command)
  gave an identical file.
