#!/usr/bin/env bash
# verify-s02-m002.sh — slice M002/S02 mechanical contract check.
#
# Asserts the Homebrew tap publication contract for the local-only surface
# this slice owns. End-to-end `brew install` acceptance is S04-postship.
#
#   1. goreleaser-check-clean — `goreleaser check` exits 0. Required
#      prerequisite; aborts the whole script on failure.
#   2. snapshot-no-token — `goreleaser release --snapshot --clean` exits 0
#      with HOMEBREW_TAP_TOKEN explicitly unset. Proves snapshot mode skips
#      the publisher and never requires tap credentials locally.
#   3. formula-file-exists — dist/homebrew/Formula/afcli.rb exists and is
#      non-empty (test -s).
#   4. formula-stanzas-present — formula contains the stable stanzas
#      (class declaration, homepage, license, bin.install, --version test).
#      Each substring is a separate sub-assertion so a partial failure names
#      the missing piece.
#   5. formula-url-template-sane — formula's url line(s) point at the
#      source repo (agentfirstcli/afcli), use releases/download, and end
#      in .tar.gz. Confirms goreleaser interpolated S01-T03's archive
#      name_template into the brews block correctly.
#   6. brew-audit-strict — if `brew` is on PATH,
#      `brew audit --strict --formula dist/homebrew/Formula/afcli.rb`
#      exits 0; otherwise WARN (skipped, counts as neither pass nor fail).
#
# Pass: every check prints PASS; exit 0.
# Fail: at least one FAIL on stderr; exit 1.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v goreleaser >/dev/null 2>&1; then
    echo "verify-s02-m002: goreleaser not installed — required for slice contract" >&2
    exit 1
fi

TMPDIR_S02M2="$(mktemp -d)"

cleanup() {
    rm -rf "$TMPDIR_S02M2"
}
trap cleanup EXIT

fails=0
passes=0

fail() {
    echo "FAIL [$1]: $2" >&2
    fails=$((fails + 1))
}

pass() {
    echo "PASS [$1]"
    passes=$((passes + 1))
}

warn() {
    echo "WARN [$1]: $2" >&2
}

# ---- Case 1: goreleaser-check-clean ----
if goreleaser check >"$TMPDIR_S02M2/check.out" 2>&1; then
    pass goreleaser-check-clean
else
    fail goreleaser-check-clean "goreleaser check failed; tail:
$(tail -n 40 "$TMPDIR_S02M2/check.out")"
    echo
    echo "verify-s02-m002: aborting — goreleaser config invalid" >&2
    exit 1
fi

# ---- Case 2: snapshot-no-token ----
# Run in a subshell so the unset doesn't leak to the rest of the script.
if (
    unset HOMEBREW_TAP_TOKEN
    goreleaser release --snapshot --clean
) >"$TMPDIR_S02M2/snapshot.out" 2>&1; then
    pass snapshot-no-token
else
    fail snapshot-no-token "goreleaser snapshot failed with HOMEBREW_TAP_TOKEN unset; tail:
$(tail -n 40 "$TMPDIR_S02M2/snapshot.out")"
    echo
    echo "verify-s02-m002: aborting — no formula to inspect" >&2
    exit 1
fi

FORMULA="$REPO_ROOT/dist/homebrew/Formula/afcli.rb"

# ---- Case 3: formula-file-exists ----
if [[ -s "$FORMULA" ]]; then
    pass formula-file-exists
else
    fail formula-file-exists "expected non-empty $FORMULA"
    echo
    echo "verify-s02-m002: aborting — formula missing or empty" >&2
    exit 1
fi

# ---- Case 4: formula-stanzas-present ----
# Each substring is a separate assertion; a missing stanza names itself.
stanza_checks=(
    'class Afcli < Formula'
    'homepage "https://github.com/agentfirstcli/afcli"'
    'license "MIT"'
    'bin.install "afcli"'
    '--version'
)
stanza_missing=0
for needle in "${stanza_checks[@]}"; do
    if ! grep -qF -e "$needle" "$FORMULA"; then
        fail formula-stanzas-present "missing stanza: $needle"
        stanza_missing=$((stanza_missing + 1))
    fi
done
if (( stanza_missing == 0 )); then
    pass formula-stanzas-present
fi

# ---- Case 5: formula-url-template-sane ----
url_lines="$(grep -E '^\s*url ' "$FORMULA" || true)"
if [[ -z "$url_lines" ]]; then
    fail formula-url-template-sane "no url lines found in $FORMULA"
else
    url_bad=0
    while IFS= read -r line; do
        [[ -z "$line" ]] && continue
        if [[ "$line" != *"agentfirstcli/afcli"* ]]; then
            fail formula-url-template-sane "url line missing source repo 'agentfirstcli/afcli': $line"
            url_bad=$((url_bad + 1))
            continue
        fi
        if [[ "$line" == *"homebrew-afcli"* ]]; then
            fail formula-url-template-sane "url line points at tap repo, not source: $line"
            url_bad=$((url_bad + 1))
            continue
        fi
        if [[ "$line" != *"releases/download"* ]]; then
            fail formula-url-template-sane "url line missing 'releases/download': $line"
            url_bad=$((url_bad + 1))
            continue
        fi
        if [[ "$line" != *.tar.gz* ]]; then
            fail formula-url-template-sane "url line does not end in .tar.gz: $line"
            url_bad=$((url_bad + 1))
            continue
        fi
    done <<< "$url_lines"
    if (( url_bad == 0 )); then
        pass formula-url-template-sane
    fi
fi

# ---- Case 6: brew-audit-strict ----
# Newer Homebrew disables `brew audit [path ...]` and requires installed
# formula names instead. A snapshot formula isn't tapped, so on newer brew
# we degrade to WARN — same shape as a missing brew. End-to-end install
# acceptance is owned by S04 postship (`brew tap && brew install` on the
# real tap), where the formula is name-resolvable.
if command -v brew >/dev/null 2>&1; then
    if brew audit --strict --formula "$FORMULA" >"$TMPDIR_S02M2/audit.out" 2>&1; then
        pass brew-audit-strict
    elif grep -qF "brew audit [path ...]" "$TMPDIR_S02M2/audit.out"; then
        warn brew-audit-strict "brew audit by path is disabled on this brew; skipping (S04 postship covers tap-resolved audit)"
    else
        fail brew-audit-strict "brew audit --strict failed; tail:
$(tail -n 40 "$TMPDIR_S02M2/audit.out")"
    fi
else
    warn brew-audit-strict "brew not on PATH; skipping"
fi

echo
if (( fails > 0 )); then
    echo "verify-s02-m002: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s02-m002: $passes checks passed"
