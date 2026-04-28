#!/usr/bin/env bash
# release-snapshot.sh — produce a local snapshot release using goreleaser.
#
# Wraps `goreleaser release --snapshot --clean` so the invocation stays
# identical between local development and CI verification. Outputs land
# in dist/ (gitignored): four afcli_*.tar.gz archives + checksums.txt.
#
# Snapshot mode never talks to GitHub — it short-circuits the publish
# step regardless of release.disable, but we keep release.disable=true
# in .goreleaser.yaml so a non-snapshot run also stays local until S04
# wires the publishers.

set -euo pipefail

if ! command -v goreleaser >/dev/null 2>&1; then
    echo "release-snapshot: goreleaser not installed — install via 'brew install goreleaser' or see https://goreleaser.com/install/" >&2
    exit 127
fi

cd "$(git rev-parse --show-toplevel)"

goreleaser check
goreleaser release --snapshot --clean
