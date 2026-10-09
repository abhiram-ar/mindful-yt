#!/bin/sh
# Installs mindful-yt from its GitHub releases on macOS or Linux:
#
#   curl -fsSL https://raw.githubusercontent.com/abhiram-ar/mindful-yt/main/install.sh | sh
#
# It installs only the mindful-yt binary. mindful-yt itself offers to install yt-dlp,
# Node.js and ffmpeg the first time it runs. Run it again to update.
#
# Environment:
#   MINDFUL_YT_VERSION      release to install, e.g. v0.1.0 (default: the latest)
#   MINDFUL_YT_INSTALL_DIR  where to put mindful-yt (default: ~/.local/bin)
#   MINDFUL_YT_BASE_URL     where to download from instead of GitHub (for testing)

set -eu

repo=abhiram-ar/mindful-yt

fail() {
	echo "mindful-yt install: $*" >&2
	exit 1
}

download() { # url file
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget -q -O "$2" "$1"
	else
		fail "needs curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "needs sha256sum or shasum to check the download"
	fi
}

# Everything runs from here, on the script's last line, so a download that
# was cut off can't run half a script.
main() {
	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "unsupported OS $(uname -s). On Windows, use install.ps1." ;;
	esac
	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "unsupported CPU $(uname -m)" ;;
	esac

	version=${MINDFUL_YT_VERSION:-latest}
	if [ "$version" = latest ]; then
		base=https://github.com/$repo/releases/latest/download
	else
		base=https://github.com/$repo/releases/download/$version
	fi
	base=${MINDFUL_YT_BASE_URL:-$base}
	dir=${MINDFUL_YT_INSTALL_DIR:-$HOME/.local/bin}
	asset=mindful-yt_${os}_${arch}.tar.gz

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	echo "Downloading $asset ($version)..."
	download "$base/$asset" "$tmp/$asset" || fail "couldn't download $base/$asset"
	download "$base/checksums.txt" "$tmp/checksums.txt" || fail "couldn't download $base/checksums.txt"
	want=$(awk -v name="$asset" '$2 == name { print $1 }' "$tmp/checksums.txt")
	[ -n "$want" ] || fail "$asset isn't listed in checksums.txt"
	[ "$(sha256 "$tmp/$asset")" = "$want" ] || fail "$asset failed its checksum, so it wasn't installed"

	tar -xzf "$tmp/$asset" -C "$tmp" mindful-yt
	mkdir -p "$dir"
	# Copy next to the target, then rename: a rename within one folder is
	# atomic, and replaces even a copy of mindful-yt that's running.
	cp "$tmp/mindful-yt" "$dir/.mindful-yt.new"
	chmod 755 "$dir/.mindful-yt.new"
	mv -f "$dir/.mindful-yt.new" "$dir/mindful-yt"

	installed=$("$dir/mindful-yt" --version) || fail "installed $dir/mindful-yt, but it doesn't run"
	echo "Installed $installed to $dir/mindful-yt"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*)
		printf '\n%s is not on your PATH. Add this line to your shell profile\n' "$dir"
		printf '(~/.zshrc, ~/.bashrc or ~/.profile), then open a new terminal:\n\n'
		printf '  export PATH="%s:$PATH"\n\n' "$dir"
		;;
	esac
	echo "Run mindful-yt. It offers to install yt-dlp, Node.js and ffmpeg if they're missing."
}

main "$@"
