#!/usr/bin/env bash
# verify-s05.sh — slice S05 demo integration check.
#
# Exercises the probe pipeline end-to-end on the built binary:
#   1. --probe + descriptor with commands.safe[--version] / commands.destructive
#      [--burn-the-disk] invokes only safe argv against an argv-recording
#      fixture; the recorded log carries --version, --help, --afcli-bogus-flag
#      but never --burn-the-disk (R008 destructive-overlap tripwire).
#   2. Without --probe, the same descriptor is loaded but the behavioral
#      pass is suppressed — recorder sees only --help and --afcli-bogus-flag
#      (R004 default-off byte-identity).
#   3. --probe-timeout=200ms against a hanging probe finishes in <2s with
#      exit 0/1 (NEVER 3 or 4) and the P3 finding is decorated with
#      status:review and "timeout" in evidence (R008 isolation).
#   4. `audit --help-schema --output json` lists --probe and --probe-timeout
#      so an agent inspecting the surface can discover them without running
#      the binary in audit mode.
# Plus a regression chain: this script inline-calls verify-s04.sh which
# itself inline-calls the prior verify-s0N.sh, so a single entry point
# covers every prior slice contract.

set -u

BIN="${BIN:-/tmp/afcli}"
TMPDIR_S05="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_S05"' EXIT

# Deterministic mode keeps started_at empty and target path normalized —
# matches verify-s0{1,2,3,4}.sh.
export AFCLI_DETERMINISTIC=1

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

# run_case <label> [env KEY=VALUE]... -- <args...>
# When the first arg is `env`, the following KEY=VALUE pairs are exported
# to the subprocess (used by probe-only-safe-argv to set ARGV_RECORD_FILE).
run_case() {
    local label="$1"; shift
    local stdout_file="$TMPDIR_S05/$label.out"
    local stderr_file="$TMPDIR_S05/$label.err"
    if [[ "${1:-}" == "env" ]]; then
        shift
        local env_pairs=()
        while [[ "${1:-}" != "--" && $# -gt 0 ]]; do
            env_pairs+=("$1")
            shift
        done
        if [[ "${1:-}" == "--" ]]; then shift; fi
        env "${env_pairs[@]}" "$BIN" "$@" >"$stdout_file" 2>"$stderr_file"
    else
        "$BIN" "$@" >"$stdout_file" 2>"$stderr_file"
    fi
    echo $? >"$TMPDIR_S05/$label.ec"
}

ec_of()    { cat "$TMPDIR_S05/$1.ec"; }
stdout_of(){ cat "$TMPDIR_S05/$1.out"; }
stderr_of(){ cat "$TMPDIR_S05/$1.err"; }

assert_exit_in() {
    local label="$1"; shift
    local actual; actual="$(ec_of "$label")"
    local ok=0 ec
    for ec in "$@"; do
        if [[ "$actual" == "$ec" ]]; then ok=1; break; fi
    done
    if (( ok == 0 )); then
        fail "$label" "expected exit in {$*}, got $actual"
        return 1
    fi
}

assert_jq() {
    local label="$1" stream filter expected actual src
    if [[ $# -eq 3 ]]; then
        stream="stdout"; filter="$2"; expected="$3"
    else
        stream="$2"; filter="$3"; expected="$4"
    fi
    case "$stream" in
        stdout) src="$TMPDIR_S05/$label.out" ;;
        stderr) src="$TMPDIR_S05/$label.err" ;;
        *)      fail "$label" "internal: bad stream $stream"; return 1 ;;
    esac
    actual="$(jq -r "$filter" <"$src" 2>/dev/null)"
    if [[ "$actual" != "$expected" ]]; then
        fail "$label" "jq($stream) '$filter' got '$actual', expected '$expected'"
        return 1
    fi
}

DESC_DIR="testdata/descriptors"

# Build the argv-recorder fixture once. Lives under testdata/, so
# `go build ./...` does not pick it up; we build it explicitly here.
RECORDER="$TMPDIR_S05/recorder"
if ! go build -o "$RECORDER" ./testdata/fixtures/argv-recorder >"$TMPDIR_S05/recorder.build.err" 2>&1; then
    fail recorder-build "go build argv-recorder failed: $(cat "$TMPDIR_S05/recorder.build.err")"
    echo "verify-s05: $fails failed, $passes passed"
    exit 1
fi

# ---- Case 1: probe-only-safe-argv ----
ARGV_LOG="$TMPDIR_S05/argv.log"
run_case probe-only-safe-argv env "ARGV_RECORD_FILE=$ARGV_LOG" -- \
    audit "$RECORDER" \
    --probe \
    --descriptor "$DESC_DIR/probe-authorizing.yaml" \
    --output json
if assert_exit_in probe-only-safe-argv 0 1; then
    if assert_jq probe-only-safe-argv '.findings | length' '16'; then
        if grep -q -- '^--version$' "$ARGV_LOG" \
           && grep -q -- '^--help$' "$ARGV_LOG" \
           && grep -q -- '^--afcli-bogus-flag$' "$ARGV_LOG"; then
            if grep -q -- '--burn-the-disk' "$ARGV_LOG"; then
                fail probe-only-safe-argv "destructive argv leaked into recorder log"
            elif ! grep -q -- 'ENV:AFCLI_TEST_PROBE=1' "$ARGV_LOG"; then
                fail probe-only-safe-argv "descriptor.env did not reach behavioral probe"
            else
                pass probe-only-safe-argv
            fi
        else
            fail probe-only-safe-argv "expected --version, --help, --afcli-bogus-flag in $ARGV_LOG; got: $(cat "$ARGV_LOG")"
        fi
    fi
fi

# ---- Case 2: probe-off-baseline ----
ARGV_LOG2="$TMPDIR_S05/argv-off.log"
run_case probe-off-baseline env "ARGV_RECORD_FILE=$ARGV_LOG2" -- \
    audit "$RECORDER" \
    --descriptor "$DESC_DIR/probe-authorizing.yaml" \
    --output json
if assert_exit_in probe-off-baseline 0 1; then
    if assert_jq probe-off-baseline '.findings | length' '16'; then
        if grep -q -- '^--help$' "$ARGV_LOG2" \
           && ! grep -q -- '^--version$' "$ARGV_LOG2"; then
            pass probe-off-baseline
        else
            fail probe-off-baseline "expected --help present and --version absent in $ARGV_LOG2; got: $(cat "$ARGV_LOG2")"
        fi
    fi
fi

# ---- Case 3: probe-timeout-fires ----
start_ts=$(date +%s)
run_case probe-timeout-fires audit "$RECORDER" \
    --probe \
    --probe-timeout=200ms \
    --descriptor "$DESC_DIR/probe-hanging.yaml" \
    --output json
end_ts=$(date +%s)
elapsed=$(( end_ts - start_ts ))
if (( elapsed > 2 )); then
    fail probe-timeout-fires "wall time too long: ${elapsed}s (want <2s)"
elif assert_exit_in probe-timeout-fires 0 1; then
    if assert_jq probe-timeout-fires '.findings | length' '16'; then
        if assert_jq probe-timeout-fires '.findings[] | select(.principle_id=="P3") | .status' 'review'; then
            if assert_jq probe-timeout-fires '.findings[] | select(.principle_id=="P3") | .evidence | contains("timeout") | tostring' 'true'; then
                pass probe-timeout-fires
            fi
        fi
    fi
fi

# ---- Case 4: probe-helpschema-reflects ----
run_case probe-helpschema-reflects audit --help-schema --output json
if assert_exit_in probe-helpschema-reflects 0; then
    if assert_jq probe-helpschema-reflects '[.command.flags[].name] | index("probe") | type' 'number'; then
        if assert_jq probe-helpschema-reflects '[.command.flags[].name] | index("probe-timeout") | type' 'number'; then
            pass probe-helpschema-reflects
        fi
    fi
fi

# ---- Regression: chain through verify-s04.sh (which inline-calls the prior chain) ----
if BIN="$BIN" bash "$(dirname "$0")/verify-s04.sh" \
       >"$TMPDIR_S05/verify-s04.out" 2>"$TMPDIR_S05/verify-s04.err"; then
    pass s04-regression
else
    fail s04-regression "verify-s04.sh failed; output: $(cat "$TMPDIR_S05/verify-s04.out") $(cat "$TMPDIR_S05/verify-s04.err")"
fi

echo
if (( fails > 0 )); then
    echo "verify-s05: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s05: $passes checks passed"
