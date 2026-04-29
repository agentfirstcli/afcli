#!/usr/bin/env bash
# verify-s02-m003.sh — slice M003/S02 postship smoke runner.
#
# Proves the slice success criterion end-to-end: `afcli inspect <fixture>`
# emits a YAML descriptor that round-trips through
# `afcli audit <fixture> --descriptor <yaml> --output json` to a
# non-trivial 16-finding verdict, on three real fixture styles
# (cobra, urfave, hand-rolled flag.Parse). Mirrors the regime/banner/
# WARN-skip conventions established by scripts/verify-s01-m003.sh
# (Regime A — pre-execution structural). No Regimes B/C/D yet: this
# slice's evidence does not depend on docker/go-install/brew variations.
#
# Regimes:
#
#   Regime A — pre-execution structural (always runs, no env vars):
#     1. inspect-cmd-registered
#     2. inspect-emits-yaml-cobra-cli
#     3. inspect-yaml-roundtrips-cobra-cli
#     4. inspect-emits-yaml-urfave-cli
#     5. inspect-yaml-roundtrips-urfave-cli
#     6. inspect-emits-yaml-flag-parse
#     7. inspect-yaml-roundtrips-flag-parse
#
# Pass: every executed check prints PASS; exit 0.
# Fail: at least one check prints FAIL on stderr; exit 1.
# Warn: a soft-degraded check prints WARN on stderr; counts as pass.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TMPDIR_S02M3="$(mktemp -d)"
AFCLI_BIN="$TMPDIR_S02M3/afcli"

cleanup() {
    if [[ -d "$TMPDIR_S02M3" ]]; then
        chmod -R u+w "$TMPDIR_S02M3" 2>/dev/null || true
        rm -rf "$TMPDIR_S02M3"
    fi
}
trap cleanup EXIT

fails=0
passes=0
warns=0

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
    warns=$((warns + 1))
    passes=$((passes + 1))
}

# Mirror MEM071's WARN-skip convention: missing-tool checks degrade to
# pass-with-warn so a host without `go` or `jq` still produces a
# grep-matchable tally line for postship CI logs.
warn_skip() {
    echo "SKIP [$1]: $2" >&2
    warns=$((warns + 1))
    passes=$((passes + 1))
}

# ----------------------------------------------------------------------
# Regime banner
# ----------------------------------------------------------------------
echo "verify-s02-m003: Regime A: pre-execution structural (always)"
echo

# ======================================================================
# Build afcli once (shared by every check below)
# ======================================================================
if ! command -v go >/dev/null 2>&1; then
    warn_skip build-afcli "go not on PATH — every Regime A check WARN-skipped"
    warn_skip inspect-cmd-registered "go not on PATH"
    for fx in cobra-cli urfave-cli flag-parse; do
        warn_skip "inspect-emits-yaml-$fx" "go not on PATH"
        warn_skip "inspect-yaml-roundtrips-$fx" "go not on PATH"
    done
    echo
    echo "verify-s02-m003: $passes checks passed, $warns warned"
    exit 0
fi

if ! go build -o "$AFCLI_BIN" ./cmd/afcli \
        >"$TMPDIR_S02M3/afcli-build.out" 2>"$TMPDIR_S02M3/afcli-build.err"; then
    fail build-afcli "go build ./cmd/afcli failed:
$(cat "$TMPDIR_S02M3/afcli-build.err")"
    echo
    echo "verify-s02-m003: $fails failed, $passes passed, $warns warned"
    exit 1
fi

# ---- Case 1: inspect-cmd-registered ----
# `--help-schema --output json` reflects the public CLI surface; the
# inspect subcommand must show up there so any future tooling that
# walks the schema picks it up automatically.
if "$AFCLI_BIN" --help-schema --output json \
        >"$TMPDIR_S02M3/help-schema.json" 2>"$TMPDIR_S02M3/help-schema.err"; then
    if grep -q '"name": *"inspect"' "$TMPDIR_S02M3/help-schema.json"; then
        pass inspect-cmd-registered
    else
        fail inspect-cmd-registered "schema JSON has no inspect subcommand entry; first 200 chars:
$(head -c 200 "$TMPDIR_S02M3/help-schema.json")"
    fi
else
    fail inspect-cmd-registered "afcli --help-schema --output json failed: $(cat "$TMPDIR_S02M3/help-schema.err")"
fi

# ----------------------------------------------------------------------
# Per-fixture: build → inspect → roundtrip-audit
# ----------------------------------------------------------------------
have_jq=1
if ! command -v jq >/dev/null 2>&1; then
    have_jq=0
fi

run_fixture_checks() {
    local fx="$1"
    local fx_bin="$TMPDIR_S02M3/${fx}.bin"

    # Build the fixture.
    if ! go build -o "$fx_bin" "./testdata/fixtures/${fx}" \
            >"$TMPDIR_S02M3/${fx}-build.out" 2>"$TMPDIR_S02M3/${fx}-build.err"; then
        fail "inspect-emits-yaml-${fx}" "go build testdata/fixtures/${fx} failed:
$(cat "$TMPDIR_S02M3/${fx}-build.err")"
        fail "inspect-yaml-roundtrips-${fx}" "fixture build failed (see above)"
        return
    fi

    # ---- inspect-emits-yaml-<fx> ----
    local insp_out="$TMPDIR_S02M3/${fx}.yaml"
    local insp_err="$TMPDIR_S02M3/${fx}-inspect.err"
    if ! "$AFCLI_BIN" inspect "$fx_bin" >"$insp_out" 2>"$insp_err"; then
        fail "inspect-emits-yaml-${fx}" "afcli inspect exited non-zero; stderr:
$(cat "$insp_err")"
        fail "inspect-yaml-roundtrips-${fx}" "inspect failed (see above)"
        return
    fi
    local missing=()
    grep -qF 'format_version:' "$insp_out" || missing+=("format_version:")
    grep -qF '# REVIEW:'       "$insp_out" || missing+=("# REVIEW:")
    grep -qF 'commands:'       "$insp_out" || missing+=("commands:")
    grep -qF 'safe:'           "$insp_out" || missing+=("safe:")
    grep -qF 'destructive:'    "$insp_out" || missing+=("destructive:")
    if (( ${#missing[@]} == 0 )); then
        pass "inspect-emits-yaml-${fx}"
    else
        fail "inspect-emits-yaml-${fx}" "emitted YAML missing literal substrings: ${missing[*]}"
        fail "inspect-yaml-roundtrips-${fx}" "emitted YAML shape regressed (see above)"
        return
    fi

    # ---- inspect-yaml-roundtrips-<fx> ----
    local audit_out="$TMPDIR_S02M3/${fx}-audit.json"
    local audit_err="$TMPDIR_S02M3/${fx}-audit.err"
    set +e
    AFCLI_DETERMINISTIC=1 "$AFCLI_BIN" audit "$fx_bin" \
        --descriptor "$insp_out" \
        --output json \
        --deterministic \
        >"$audit_out" 2>"$audit_err"
    local rc=$?
    set -e 2>/dev/null || true

    if [[ $rc -ne 0 && $rc -ne 1 ]]; then
        fail "inspect-yaml-roundtrips-${fx}" "afcli audit exit code $rc (want 0 or 1); stderr:
$(cat "$audit_err")"
        return
    fi

    # No envelope-level error.
    if grep -qE '^[[:space:]]*"error"[[:space:]]*:' "$audit_out"; then
        # Could be a nested 'error' field, but at top level the renderer
        # places it on its own line in pretty JSON. Use jq when present
        # to be precise.
        if (( have_jq )); then
            if [[ "$(jq -r 'has("error")' "$audit_out" 2>/dev/null)" == "true" ]]; then
                fail "inspect-yaml-roundtrips-${fx}" "audit report carries envelope error key:
$(head -c 300 "$audit_out")"
                return
            fi
        else
            fail "inspect-yaml-roundtrips-${fx}" "audit report appears to carry an \"error\": envelope key (no jq to confirm):
$(head -c 300 "$audit_out")"
            return
        fi
    fi

    # Findings count must be exactly 16.
    local findings_count
    if (( have_jq )); then
        findings_count="$(jq -r '.findings | length' "$audit_out" 2>/dev/null || echo "")"
    else
        # Fallback: count distinct principle_id occurrences. The
        # renderer emits one principle_id field per finding.
        findings_count="$(grep -cE '"principle_id"[[:space:]]*:' "$audit_out" || true)"
    fi
    if [[ -z "$findings_count" ]]; then
        fail "inspect-yaml-roundtrips-${fx}" "could not extract findings count from audit JSON"
        return
    fi
    if [[ "$findings_count" -ne 16 ]]; then
        fail "inspect-yaml-roundtrips-${fx}" "findings count $findings_count (want 16); first 300 chars of audit JSON:
$(head -c 300 "$audit_out")"
        return
    fi
    pass "inspect-yaml-roundtrips-${fx}"
}

if (( have_jq == 0 )); then
    echo "verify-s02-m003: jq not on PATH — falling back to grep-based findings count" >&2
fi

for fx in cobra-cli urfave-cli flag-parse; do
    run_fixture_checks "$fx"
done

echo
if (( fails > 0 )); then
    echo "verify-s02-m003: $fails failed, $passes passed, $warns warned"
    exit 1
fi
echo "verify-s02-m003: $passes checks passed, $warns warned"
