#!/bin/sh
# The container's entrypoint, as root. It copies the repo (mounted read-only
# at /src) to /work, runs check.sh as the tester user, and leaves the results
# in /out. See the Dockerfile for how to build and run it.
set -u
out=/out
mkdir -p "$out"
rm -rf "${out:?}"/* 2>/dev/null
owner=$(stat -c %u:%g /src) # the results go to whoever owns the repo
trap 'chown -R "$owner" "$out" 2>/dev/null' EXIT

# The working tree's tracked and new files, without anything gitignored
# (builds, earlier results).
git config --global --add safe.directory /src
mkdir -p /work
(cd /src && git ls-files -z --cached --others --exclude-standard | xargs -0 cp --parents -t /work 2>/dev/null)
chown -R tester:tester /work "$out"

su - tester -c "sh /src/test/container/check.sh"
code=$?
echo
echo "== results"
cat "$out/results.txt"
exit $code
