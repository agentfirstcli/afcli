#!/usr/bin/env bash
# verify-s04-m003.sh — slice M003/S04 dogfood/badge contract check.
#
# Closes the dogfood loop end-to-end. Each case echoes a one-line verdict
# so the script log reads as a top-down timeline of the slice contract.
#
# Cases:
#   1. preflight — afcli.yaml descriptor exists at the repo root and the
#      Go toolchain is available to build the binary.
#   2. build-binary — `go build -o /tmp/afcli ./cmd/afcli` succeeds.
#   3. badge-render-run-1 — `audit /tmp/afcli --probe --descriptor afcli.yaml
#      --badge --badge-out <tmp>` exits 0 and writes badge.svg + badge.json.
#   4. badge-render-run-2 — same command, second run.
#   5. badge-svg-non-empty — badge.svg from run 1 has non-zero size.
#   6. badge-json-non-empty — badge.json from run 1 has non-zero size.
#   7. badge-svg-byte-stable — `cmp -s` between the two runs.
#   8. badge-json-byte-stable — `cmp -s` between the two runs.
#   9. readme-references-badge — README.md contains `docs/badge.svg`.
#  10. release-workflow-has-regen — `.github/workflows/release.yml` contains
#      the literal string `regenerate-badge`.
#
# Pass: every check prints PASS; exit 0.
# Fail: at least one check prints FAIL on stderr; exit 1.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TMPDIR_S04M3="$(mktemp -d)"
BIN="$TMPDIR_S04M3/afcli"
RUN1_DIR="$TMPDIR_S04M3/run1"
RUN2_DIR="$TMPDIR_S04M3/run2"

cleanup() {
    rm -rf "$TMPDIR_S04M3"
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

# ---- Case 1: preflight ----
if [[ ! -f "afcli.yaml" ]]; then
    fail preflight "afcli.yaml missing at repo root"
elif ! command -v go >/dev/null 2>&1; then
    fail preflight "go toolchain not on PATH"
else
    pass preflight
fi

# ---- Case 2: build-binary ----
if go build -o "$BIN" ./cmd/afcli; then
    pass build-binary
else
    fail build-binary "go build failed"
    echo
    echo "verify-s04-m003: aborting — no binary to exercise" >&2
    echo "verify-s04-m003: $fails failed, $passes passed"
    exit 1
fi

# ---- Case 3: badge-render-run-1 ----
mkdir -p "$RUN1_DIR"
if AFCLI_DETERMINISTIC=1 "$BIN" audit "$BIN" \
        --probe --descriptor afcli.yaml \
        --badge --badge-out "$RUN1_DIR" \
        >"$TMPDIR_S04M3/run1.out" 2>"$TMPDIR_S04M3/run1.err"; then
    if [[ -f "$RUN1_DIR/badge.svg" && -f "$RUN1_DIR/badge.json" ]]; then
        pass badge-render-run-1
    else
        fail badge-render-run-1 "audit exited 0 but badge files missing in $RUN1_DIR"
    fi
else
    fail badge-render-run-1 "audit failed (exit $?); stderr tail:
$(tail -n 20 "$TMPDIR_S04M3/run1.err")"
fi

# ---- Case 4: badge-render-run-2 ----
mkdir -p "$RUN2_DIR"
if AFCLI_DETERMINISTIC=1 "$BIN" audit "$BIN" \
        --probe --descriptor afcli.yaml \
        --badge --badge-out "$RUN2_DIR" \
        >"$TMPDIR_S04M3/run2.out" 2>"$TMPDIR_S04M3/run2.err"; then
    if [[ -f "$RUN2_DIR/badge.svg" && -f "$RUN2_DIR/badge.json" ]]; then
        pass badge-render-run-2
    else
        fail badge-render-run-2 "audit exited 0 but badge files missing in $RUN2_DIR"
    fi
else
    fail badge-render-run-2 "audit failed (exit $?); stderr tail:
$(tail -n 20 "$TMPDIR_S04M3/run2.err")"
fi

# ---- Case 5: badge-svg-non-empty ----
if [[ -s "$RUN1_DIR/badge.svg" ]]; then
    pass badge-svg-non-empty
else
    fail badge-svg-non-empty "badge.svg is empty or missing"
fi

# ---- Case 6: badge-json-non-empty ----
if [[ -s "$RUN1_DIR/badge.json" ]]; then
    pass badge-json-non-empty
else
    fail badge-json-non-empty "badge.json is empty or missing"
fi

# ---- Case 7: badge-svg-byte-stable ----
if cmp -s "$RUN1_DIR/badge.svg" "$RUN2_DIR/badge.svg"; then
    pass badge-svg-byte-stable
else
    fail badge-svg-byte-stable "two runs produced differing badge.svg under AFCLI_DETERMINISTIC=1"
fi

# ---- Case 8: badge-json-byte-stable ----
if cmp -s "$RUN1_DIR/badge.json" "$RUN2_DIR/badge.json"; then
    pass badge-json-byte-stable
else
    fail badge-json-byte-stable "two runs produced differing badge.json under AFCLI_DETERMINISTIC=1"
fi

# ---- Case 9: readme-references-badge ----
if grep -q 'docs/badge.svg' README.md; then
    pass readme-references-badge
else
    fail readme-references-badge "README.md does not reference docs/badge.svg"
fi

# ---- Case 10: release-workflow-has-regen ----
if grep -q 'regenerate-badge' .github/workflows/release.yml; then
    pass release-workflow-has-regen
else
    fail release-workflow-has-regen ".github/workflows/release.yml missing regenerate-badge step"
fi

echo
if (( fails > 0 )); then
    echo "verify-s04-m003: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s04-m003: $passes checks passed"
