# AGENTS.md

mindful-yt downloads one YouTube video at a time while YouTube is blocked in the
hosts file. It runs yt-dlp with `--proxy` pointed at a private proxy on
`127.0.0.1` (`internal/proxy`) that resolves YouTube over DNS-over-HTTPS. Go
and Bubble Tea v2, on Windows, macOS and Linux.

## Before pushing

Users install with `install.sh` / `install.ps1`, which are served straight
from `main`, so changes to them ship on push. Binaries ship when a `v*` tag
is pushed: `.github/workflows/release.yml` runs GoReleaser
(`.goreleaser.yaml`). The scripts rely on the release file names having no
version in them (`mindful-yt_<os>_<arch>.tar.gz`, `.zip` on Windows, plus
`checksums.txt`). `go install ...@latest` builds the newest `v*` tag, not
`main`, so it too updates only when a tag is pushed.

Test `install.ps1` the way users run it: pasted at a prompt
(`$lines | powershell -Command -`), where PSReadLine is loaded. A `-File` run
doesn't load PSReadLine, so it misses the bugs PSReadLine causes.

```sh
gofmt -l .      # must print nothing
go vet ./...    # also with GOOS=linux and GOOS=darwin
go test ./...
go test -tags live -run Live -v ./internal/ytdlp   # after changing ytdlp, proxy or deps
```

The end-to-end check runs in a throwaway Arch Linux container
(`test/container/`). It runs the build, the tests and the live tests, then
drives the interface in tmux with YouTube blocked in the container's own
hosts file. Run it after changing the interface, ytdlp or deps. Docker needs
`sudo` on the dev machine, so give the user these to run from the repo root:

```sh
sudo docker build -t mindful-yt-check test/container   # once, and after changing the Dockerfile
sudo docker run --rm -v "$PWD":/src:ro -v "$PWD/test/container/out":/out mindful-yt-check
```

Results are in `test/container/out/results.txt` (gitignored), with logs and
screens next to it.

## Bubble Tea

The TUI (`internal/tui`) is built on Charm's v2 modules:
`charm.land/bubbletea/v2`, `charm.land/bubbles/v2` and `charm.land/lipgloss/v2`.
v2's API differs from v1 and most examples online are v1. For instance,
`View()` returns a `tea.View`, and key presses arrive as `tea.KeyPressMsg`.

Work from the docs of the versions pinned in `go.mod`, not the latest:

```sh
go doc charm.land/bubbletea/v2 View                      # docs for the pinned version
go list -m -f '{{.Version}} {{.Dir}}' charm.land/bubbletea/v2   # version and source
```

The docs are also on pkg.go.dev, at `https://pkg.go.dev/charm.land/bubbletea/v2@<version>`.

`github.com/charmbracelet/ultraviolet` (Bubble Tea's renderer) is pinned to
`v0.0.0-20261001125412-878653296cfd`, newer than Bubble Tea v2.0.10 asks for. The older one left the top of
a taller frame on screen whenever a screen got shorter, e.g. going back from
the search results. Don't let a `go get` or `go mod tidy` move it back; once a
Bubble Tea release requires that build or newer, the pin can go.

## Rules

- **Follow the standard Go project layout.**
  - The command's entry point is `cmd/mindful-yt/`. All other code goes in
    `internal/<package>/`: extend the package that owns the concern, or add a
    new one. No Go code at the repo root.
  - Tests go next to the code, in `_test.go` files.
  - OS-specific code uses `_windows.go` files or `//go:build unix`.
- **YouTube is blocked on the dev machine.** Reach it only through the proxy,
  as `liveSetup` in `internal/ytdlp/live_test.go` does. Never edit the hosts
  file, and never run `lock-me-in` there: it reads or writes the real one
  (`--write-hosts` writes without asking). `internal/hosts` tests use temp
  files, and the lock-me-in TUI tests replace `apply` and `verify`.
- **Don't touch the user's real data.** Their settings and history
  (`store.Dirs`) drive the daily limit. Set `MINDFUL_YT_HOME` to a temp folder
  before running the binary; only `--version` is safe without it. Every other
  command, even `--help`, may move an old `ytget` folder to `mindful-yt`.
  - **Your commands may not see the user's real AppData.** An agent's sandbox
    can keep its own copy of `%APPDATA%` and `%LOCALAPPDATA%`, so changes
    made there never reach the user's programs. To check or fix their real
    data, give them commands to run in their own terminal.
- **The guardrails are the product.** Single videos only, the daily limit and a
  reason for every download stay.
- **Keep the proxy locked down:** `127.0.0.1` only, the per-run password, and
  CONNECT only.
- **Change yt-dlp's output and mindful-yt's parsing together.**
  `ytdlp.DownloadArgs` makes yt-dlp print `mindful-yt-progress`, `mindful-yt-formats` and
  `mindful-yt-done` lines, which `Download`, `parseProgress` and `parseDone` parse.
  `ytdlp.listArgs` asks for one flat `-J` list (a search, or a channel's
  Videos tab) with `approximate_date` timestamps and `-I 1:N`, which
  `parseList` reads and `human.Ago` turns back into YouTube's "3 weeks ago".
  Keep the `-I`: without it, a channel is paged through to its oldest video.
- **The TUI needs a real terminal.** Test it through `Update` and `View()`, as
  `internal/tui/tui_test.go` does.
- **Commit and push only when the user asks.** Stage files by name and
  `git fetch` first, because the user edits and commits too.
- **The README is the user's.** Keep it minimal and user-facing. The
  disclaimer is in their own words: fix typos, but show any rewording before
  pushing.
- **Write down what matters before your context runs out.** Before a session
  is compacted or ends, save critical decisions and open items: rules
  everyone needs go here, background and status go in a worklog.

Background and past decisions are in `.agents/worklogs/`. Put less important
context there too, one file per feature, named
`[<last edited IST date-time>][<work first started IST date-time>][<feature>].md`,
e.g. `[2026-09-30T23-20-48_IST][2026-09-30T02-10-20_IST][proxy-design].md`.
Rename the file when you edit it. No colons: Windows forbids them.
