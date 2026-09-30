# AGENTS.md

ytget downloads one YouTube video at a time while YouTube is blocked in the
hosts file. It runs yt-dlp with `--proxy` pointed at a private proxy on
`127.0.0.1` (`internal/proxy`) that resolves YouTube over DNS-over-HTTPS. Go
and Bubble Tea v2, on Windows, macOS and Linux.

## Before pushing

Anything on `main` ships: users install with `go install ...@latest`.

```sh
gofmt -l .      # must print nothing
go vet ./...    # also with GOOS=linux and GOOS=darwin
go test ./...
go test -tags live -run Live -v ./internal/ytdlp   # after changing ytdlp, proxy or deps
```

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

## Rules

- **YouTube is blocked on the dev machine.** Reach it only through the proxy,
  as `liveSetup` in `internal/ytdlp/live_test.go` does. Never edit the hosts
  file.
- **Don't touch the user's real data.** Their settings and history
  (`store.Dirs`) drive the daily limit. Set `YTGET_HOME` to a temp folder
  before running the binary; only `--help` and `--history` are safe without it.
- **The guardrails are the product.** Single videos only, the daily limit and a
  reason for every download stay.
- **Keep the proxy locked down:** `127.0.0.1` only, the per-run password, and
  CONNECT only.
- **Change yt-dlp's output and ytget's parsing together.**
  `ytdlp.DownloadArgs` makes yt-dlp print `ytget-progress`, `ytget-formats` and
  `ytget-done` lines, which `Download`, `parseProgress` and `parseDone` parse.
- **The TUI needs a real terminal.** Test it through `Update` and `View()`, as
  `internal/tui/tui_test.go` does.

Background and past decisions are in `.agents/worklogs/`. Put less important
context there too, one file per feature, named
`[<last edited IST date-time>][<work first started IST date-time>][<feature>].md`,
e.g. `[2026-09-30T23-20-48_IST][2026-09-30T02-10-20_IST][proxy-design].md`.
Rename the file when you edit it. No colons: Windows forbids them.
