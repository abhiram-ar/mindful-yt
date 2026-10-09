# A throwaway Arch Linux machine for checking mindful-yt end to end: the build,
# the tests, the live tests through the proxy, and the interface in a real
# terminal (tmux), with YouTube blocked in the container's own hosts file.
# Nothing on the host changes but the results folder. From the repo root:
#
#   sudo docker build -t mindful-yt-check test/container
#   sudo docker run --rm -v "$PWD":/src:ro -v "$PWD/test/container/out":/out mindful-yt-check
#
# Results are in test/container/out/results.txt, with logs and screens next to
# it. The scripts run from the mounted repo, so changing them needs no rebuild.
FROM archlinux:latest

# ffmpeg comes ready, so mindful-yt only installs yt-dlp and Node.js itself.
RUN pacman -Syu --noconfirm --needed curl tmux file go git sudo jq procps-ng ffmpeg ca-certificates

# A normal user with sudo, like an Arch desktop user. No password, since
# nobody is at the keyboard.
RUN useradd -m -s /bin/bash tester \
	&& echo 'tester ALL=(ALL) NOPASSWD: ALL' >/etc/sudoers.d/tester \
	&& chmod 440 /etc/sudoers.d/tester

ENTRYPOINT ["sh", "/src/test/container/run.sh"]
