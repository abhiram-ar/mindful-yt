#!/bin/sh
# The checks, run as "tester" in the container by run.sh. Each one adds a
# PASS or FAIL line to /out/results.txt; logs and tmux screen captures go next
# to it. A failing check doesn't stop the rest.
set -u
out=/out
results=$out/results.txt
: >"$results"
export MINDFUL_YT_HOME=/tmp/myt-home
export GOTOOLCHAIN=auto GOFLAGS=-buildvcs=false
bin=$HOME/bin/mindful-yt

pass() { echo "PASS  $1${2:+  ($2)}" | tee -a "$results"; }
fail() { echo "FAIL  $1${2:+  ($2)}" | tee -a "$results"; }

printf 'set -g status off\n' >/tmp/tmux.conf
T() { tmux -L myt -f /tmp/tmux.conf "$@"; }
screen() { T capture-pane -p -t app 2>/dev/null; }
snap() { screen >"$out/screen-$1.txt"; }
wait_for() { # TEXT SECONDS
	i=0
	while [ "$i" -lt "$2" ]; do
		screen | grep -qF -- "$1" && return 0
		sleep 1; i=$((i + 1))
	done
	return 1
}
start_app() { # COLS ROWS COMMAND
	T kill-server 2>/dev/null
	T new-session -d -s app -x "$1" -y "$2" "$3; echo APP-EXIT=\$?; sleep 3600"
}
type_line() { T send-keys -t app -l -- "$1"; sleep 0.3; T send-keys -t app Enter; }
key() { T send-keys -t app "$@"; }
no_ytdlp_left() { ! pgrep -u tester -f 'yt-dlp' >/dev/null; }
# clean NAME: the screen starts with the header and holds one frame only.
clean() {
	if head -1 "$out/screen-$1.txt" | grep -q '^mindful-yt  .*downloads today' &&
		[ "$(grep -c '^mindful-yt  .*downloads today' "$out/screen-$1.txt")" -eq 1 ]; then
		pass "$1-no-leftover-lines"
	else fail "$1-no-leftover-lines" "see screen-$1.txt"; fi
}

echo "== 1. build and offline tests"
cd /work
go version >"$out/1-go.log" 2>&1
go build -o "$bin" ./cmd/mindful-yt >"$out/1-build.log" 2>&1 && pass 1-build || fail 1-build "see 1-build.log"
go test -count=1 ./... >"$out/1-test.log" 2>&1 && pass 1-test || fail 1-test "see 1-test.log"

echo "== 2. block YouTube in this container's hosts file"
sudo "$bin" lock-me-in --write-hosts >"$out/2-lock.log" 2>&1
getent hosts youtube.com >"$out/2-dns.log" 2>&1
if grep -qE '^(0\.0\.0\.0|::)\s' "$out/2-dns.log" && ! curl -sS -m 10 -o /dev/null https://www.youtube.com/ 2>/dev/null; then
	pass 2-youtube-blocked
else fail 2-youtube-blocked "see 2-dns.log"; fi

echo "== 3. live tests"
MINDFUL_YT_HOME=/tmp/myt-live go test -count=1 -tags live -run Live -v ./internal/ytdlp >"$out/3-live.log" 2>&1
if [ $? -eq 0 ]; then
	pass 3-live-tests "$(grep -c -- '--- PASS' "$out/3-live.log") passed, $(grep -c -- '--- SKIP' "$out/3-live.log") skipped"
else fail 3-live-tests "see 3-live.log"; fi

echo "== 4. the search in a real terminal (80x24)"
start_app 80 24 "$bin -q 144 -r 'checking search in docker'"
if wait_for 'Paste a YouTube link, type a search, or @handle:' 20; then
	snap 4-prompt; pass 4-prompt
	type_line 'me at the zoo'
	if wait_for 'needs some tools' 30; then snap 4-deps; key Enter; fi
	if wait_for 'Results for "me at the zoo":' 900; then
		sleep 0.5; snap 4-results; pass 4-results; clean 4-results
		grep -q '•' "$out/screen-4-results.txt" && grep -q '←/→ page' "$out/screen-4-results.txt" &&
			pass 4-paged-at-24-rows || fail 4-paged-at-24-rows "see screen-4-results.txt"
		grep -qE 'views · .* ago' "$out/screen-4-results.txt" && pass 4-details-line || fail 4-details-line
		key Down; key Down; key Down; sleep 0.5; snap 4-scrolled; clean 4-scrolled
		key Right; sleep 0.5; snap 4-page-2; clean 4-page-2
		key Left; sleep 0.3; key Up; key Up; key Up; sleep 0.3
		key Escape
		if wait_for 'Paste a YouTube link, type a search, or @handle:' 10 && sleep 0.5 && screen | grep -qF 'me at the zoo'; then
			snap 4-back; pass 4-esc-back-keeps-query; clean 4-back
			screen | grep -qF 'Results for' && fail 4-back-results-gone "old results still on screen" || pass 4-back-results-gone
		else snap 4-back; fail 4-esc-back-keeps-query "see screen-4-back.txt"; fi
		key Enter
		if wait_for 'Searching YouTube for' 30 && wait_for 'Results for "me at the zoo":' 120; then
			sleep 0.5; snap 4-results-again; clean 4-results-again
			key Enter # the first result
			if wait_for 'Saved ' 600; then sleep 0.5; snap 4-saved; pass 4-pick-and-download; clean 4-saved
			else snap 4-download-stuck; fail 4-pick-and-download "see screen-4-download-stuck.txt"; fi
			key q; wait_for 'APP-EXIT=0' 15 && pass 4-quit-clean || { snap 4-quit; fail 4-quit-clean; }
		else snap 4-again-stuck; fail 4-search-again; fi
	else snap 4-results-stuck; fail 4-results "see screen-4-results-stuck.txt"; fi
else snap 4-start; fail 4-prompt "see screen-4-start.txt"; fi
T kill-server 2>/dev/null
cp "$MINDFUL_YT_HOME/history.jsonl" "$out/4-history.jsonl" 2>/dev/null
jq -e 'select(.reason=="checking search in docker")' "$out/4-history.jsonl" >/dev/null 2>&1 &&
	pass 4-history-has-reason "$(jq -r '.title' "$out/4-history.jsonl" | tail -1)" || fail 4-history-has-reason

echo "== 5. resizing and narrow terminals"
start_app 40 12 "$bin 'me at the zoo'"
if wait_for 'Results for' 120; then
	sleep 0.5; snap 5-narrow; clean 5-narrow
	awk 'length > 39 { bad=1 } END { exit bad }' "$out/screen-5-narrow.txt" && pass 5-narrow-fits || fail 5-narrow-fits
	T resize-window -t app -x 100 -y 52; sleep 1; snap 5-tall; clean 5-tall # 15 videos fit from 51 rows
	grep -q '•' "$out/screen-5-tall.txt" && fail 5-tall-shows-all "still paged" || pass 5-tall-shows-all
	T resize-window -t app -x 60 -y 16; sleep 1; snap 5-shrunk; clean 5-shrunk
else snap 5-stuck; fail 5-narrow "see screen-5-stuck.txt"; fi
T kill-server 2>/dev/null

echo "== 6. cancelling a search"
start_app 80 24 "$bin 'lofi hip hop'"
if wait_for 'Searching YouTube for' 30; then
	key Escape
	wait_for 'Paste a YouTube link, type a search, or @handle:' 10 && pass 6-esc-back || { snap 6-esc; fail 6-esc-back; }
	sleep 2; no_ytdlp_left && pass 6-esc-stops-yt-dlp || { pgrep -af yt-dlp >"$out/6-orphans.log"; fail 6-esc-stops-yt-dlp; }
	key Enter
	if wait_for 'Searching YouTube for' 30; then
		key C-c
		wait_for 'APP-EXIT=130' 15 && pass 6-ctrl-c-quits || { snap 6-ctrl-c; fail 6-ctrl-c-quits; }
		sleep 2; no_ytdlp_left && pass 6-ctrl-c-stops-yt-dlp || fail 6-ctrl-c-stops-yt-dlp
	else snap 6-again; fail 6-ctrl-c-quits "search didn't restart"; fi
else snap 6-start; fail 6-esc-back "never saw the search spinner"; fi
T kill-server 2>/dev/null

echo "== 7. guardrails"
start_app 80 24 "$bin"
wait_for 'Paste a YouTube link, type a search, or @handle:' 20
type_line 'https://www.youtube.com/playlist?list=PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI'
wait_for 'Only links to a single video' 10 && pass 7-playlist-refused || fail 7-playlist-refused
snap 7-playlist; T kill-server 2>/dev/null

start_app 80 24 "$bin 'qwzxqwzxqwzx zzkkqvvbnm plmokn'"
if wait_for 'No videos found' 120 || wait_for 'Results for' 5; then
	snap 7-nonsense; pass 7-nonsense-query-handled "$(grep -E 'No videos|Results' "$out/screen-7-nonsense.txt")"
else snap 7-nonsense; fail 7-nonsense-query-handled "see screen-7-nonsense.txt"; fi
T kill-server 2>/dev/null

cp "$MINDFUL_YT_HOME/config.json" /tmp/config.json
jq '.daily_limit=1' /tmp/config.json >"$MINDFUL_YT_HOME/config.json"
start_app 80 24 "$bin cats"
wait_for 'Daily limit reached (1/1)' 20 && pass 7-limit-refuses-search || fail 7-limit-refuses-search
snap 7-limit; T kill-server 2>/dev/null
no_ytdlp_left && pass 7-limit-ran-no-yt-dlp || fail 7-limit-ran-no-yt-dlp
cp /tmp/config.json "$MINDFUL_YT_HOME/config.json"

echo "== 8. a channel's newest videos"
start_app 80 24 "$bin '@jawed'"
if wait_for 'Newest videos from @jawed:' 120; then
	sleep 0.5; snap 8-channel; clean 8-channel; pass 8-channel-listed
	grep -q 'Me at the zoo' "$out/screen-8-channel.txt" && pass 8-channel-has-zoo || fail 8-channel-has-zoo "see screen-8-channel.txt"
	key Escape
	wait_for 'Paste a YouTube link, type a search, or @handle:' 10 && sleep 0.3 && screen | grep -qF '@jawed' &&
		pass 8-esc-keeps-handle || { snap 8-esc; fail 8-esc-keeps-handle; }
else snap 8-channel-stuck; fail 8-channel-listed "see screen-8-channel-stuck.txt"; fi
T kill-server 2>/dev/null

start_app 80 24 "$bin '@LofiGirl'"
if wait_for 'Newest videos from @LofiGirl:' 120; then
	sleep 0.5; snap 8-big-channel; clean 8-big-channel; pass 8-big-channel-listed
else snap 8-big-channel-stuck; fail 8-big-channel-listed "see screen-8-big-channel-stuck.txt"; fi
T kill-server 2>/dev/null

start_app 80 24 "$bin '@zz9nochannelhere7qx'"
if wait_for "There's no channel @zz9nochannelhere7qx on YouTube." 120; then
	snap 8-unknown; pass 8-unknown-handle
else snap 8-unknown; fail 8-unknown-handle "see screen-8-unknown.txt"; fi
T kill-server 2>/dev/null
