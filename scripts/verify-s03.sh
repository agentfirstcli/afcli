#!/usr/bin/env bash
# verify-s03.sh — slice S03 demo integration check.
#
# Exercises the static check engine end-to-end on the built binary:
#   1. afcli audit /usr/bin/git --output {json,text,markdown} produces
#      16 findings — 5 real (P6/P7/P14/P15/P16) and 11 stubs.
#   2. afcli audit /bin/echo --output json never leaks a "panicked" string
#      from safeRun (production checks must not panic).
#   3. afcli audit /usr/bin/git --output json --deterministic is byte-
#      identical across two invocations.
# Plus a regression chain: this script inline-calls verify-s02.sh which
# itself inline-calls verify-s01.sh, so a single entry point covers
# every prior slice contract.
#
# Per-principle verdicts (panic-isolation, severity table, kind=automated
# vs requires-review counts) are unit-tested in internal/audit/*_test.go;
# this script verifies the binary's end-to-end shape that tests cannot
# reach. Cases that depend on /usr/bin/git skip cleanly when it is
# absent (CI macOS friendliness — research §"Verification plan" item 2).

set -u

BIN="${BIN:-/tmp/afcli}"
TMPDIR_S03="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_S03"' EXIT

# Deterministic mode keeps started_at empty and target path normalized —
# stabilizes byte-identity case 5 across machines (matches verify-s0{1,2}.sh).
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

run_case() {
    local label="$1"; shift
    local stdout_file="$TMPDIR_S03/$label.out"
    local stderr_file="$TMPDIR_S03/$label.err"
    "$BIN" "$@" >"$stdout_file" 2>"$stderr_file"
    echo $? >"$TMPDIR_S03/$label.ec"
}

ec_of()    { cat "$TMPDIR_S03/$1.ec"; }
stdout_of(){ cat "$TMPDIR_S03/$1.out"; }
stderr_of(){ cat "$TMPDIR_S03/$1.err"; }

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

assert_contains() {
    local label="$1" stream="$2" needle="$3" hay
    case "$stream" in
        stdout) hay="$(stdout_of "$label")" ;;
        stderr) hay="$(stderr_of "$label")" ;;
        *)      fail "$label" "internal: bad stream $stream"; return 1 ;;
    esac
    if [[ "$hay" != *"$needle"* ]]; then
        fail "$label" "$stream missing '$needle'"
        return 1
    fi
}

assert_not_contains() {
    local label="$1" stream="$2" needle="$3" hay
    case "$stream" in
        stdout) hay="$(stdout_of "$label")" ;;
        stderr) hay="$(stderr_of "$label")" ;;
        *)      fail "$label" "internal: bad stream $stream"; return 1 ;;
    esac
    if [[ "$hay" == *"$needle"* ]]; then
        fail "$label" "$stream unexpectedly contains '$needle'"
        return 1
    fi
}

assert_min_lines() {
    local label="$1" stream="$2" min="$3" actual
    case "$stream" in
        stdout) actual="$(stdout_of "$label" | wc -l)" ;;
        stderr) actual="$(stderr_of "$label" | wc -l)" ;;
    esac
    if (( actual < min )); then
        fail "$label" "$stream had $actual lines, expected >= $min"
        return 1
    fi
}

assert_valid_json_stdout() {
    local label="$1"
    if ! jq -e . <"$TMPDIR_S03/$label.out" >/dev/null 2>&1; then
        fail "$label" "stdout is not valid JSON"
        return 1
    fi
}

assert_jq() {
    local label="$1" filter="$2" expected="$3" actual
    actual="$(jq -r "$filter" <"$TMPDIR_S03/$label.out" 2>/dev/null)"
    if [[ "$actual" != "$expected" ]]; then
        fail "$label" "jq '$filter' got '$actual', expected '$expected'"
        return 1
    fi
}

GIT_BIN="/usr/bin/git"
have_git=1
if [[ ! -x "$GIT_BIN" ]]; then
    have_git=0
    echo "INFO: $GIT_BIN not present — git-dependent cases will be skipped." >&2
fi

# ---- Case 1: audit /usr/bin/git --output json — full shape of a real run ----
if (( have_git )); then
    run_case audit-git-json audit "$GIT_BIN" --output json
    # Exit may be 0 (no fail-grade findings) or 1 (some non-pass at threshold).
    # Both are valid evidence that the engine ran — we assert the body, not the rc.
    if assert_exit_in audit-git-json 0 1; then
        assert_valid_json_stdout audit-git-json && \
        assert_jq audit-git-json '.findings | length' '16' && \
        assert_jq audit-git-json '.manifest_version' '0.0.1' && \
        assert_jq audit-git-json '.findings[] | select(.principle_id == "P7") | .status' 'pass' && \
        assert_jq audit-git-json '[.findings[] | select(.kind == "automated")] | length' '5' && \
        assert_jq audit-git-json '[.findings[] | select(.kind == "requires-review")] | length' '11' && \
        pass audit-git-json
    fi
else
    pass audit-git-json-skipped
fi

# ---- Case 2: audit /bin/echo --output json — no panic-evidence leakage ----
run_case audit-echo-json audit /bin/echo --output json
if assert_exit_in audit-echo-json 0 1; then
    assert_valid_json_stdout audit-echo-json && \
    assert_jq audit-echo-json '.findings | length' '16' && \
    assert_not_contains audit-echo-json stdout 'panicked' && \
    pass audit-echo-json
fi

# ---- Case 3: audit /usr/bin/git --output text — text renderer through real findings ----
if (( have_git )); then
    run_case audit-text-renders-findings audit "$GIT_BIN" --output text
    if assert_exit_in audit-text-renders-findings 0 1; then
        assert_contains  audit-text-renders-findings stdout 'P6'  && \
        assert_contains  audit-text-renders-findings stdout 'P7'  && \
        assert_contains  audit-text-renders-findings stdout 'P14' && \
        assert_contains  audit-text-renders-findings stdout 'P15' && \
        assert_contains  audit-text-renders-findings stdout 'P16' && \
        assert_min_lines audit-text-renders-findings stdout 16 && \
        pass audit-text-renders-findings
    fi
else
    pass audit-text-renders-findings-skipped
fi

# ---- Case 4: audit /usr/bin/git --output markdown — category H2 grouping survives real findings ----
if (( have_git )); then
    run_case audit-md-renders-findings audit "$GIT_BIN" --output markdown
    if assert_exit_in audit-md-renders-findings 0 1; then
        assert_contains audit-md-renders-findings stdout '## ' && \
        pass audit-md-renders-findings
    fi
else
    pass audit-md-renders-findings-skipped
fi

# ---- Case 5: --deterministic is byte-identical across invocations ----
if (( have_git )); then
    "$BIN" audit "$GIT_BIN" --output json --deterministic \
        >"$TMPDIR_S03/audit-det-a.out" 2>"$TMPDIR_S03/audit-det-a.err"
    ec_a=$?
    "$BIN" audit "$GIT_BIN" --output json --deterministic \
        >"$TMPDIR_S03/audit-det-b.out" 2>"$TMPDIR_S03/audit-det-b.err"
    ec_b=$?
    if [[ "$ec_a" != "0" && "$ec_a" != "1" ]] || [[ "$ec_b" != "0" && "$ec_b" != "1" ]]; then
        fail audit-deterministic-byte-identical "unexpected exit (a=$ec_a, b=$ec_b)"
    elif [[ "$ec_a" != "$ec_b" ]]; then
        fail audit-deterministic-byte-identical "exit codes differ across runs (a=$ec_a, b=$ec_b)"
    elif ! diff -q "$TMPDIR_S03/audit-det-a.out" "$TMPDIR_S03/audit-det-b.out" >/dev/null; then
        fail audit-deterministic-byte-identical "stdout differs across invocations"
    else
        pass audit-deterministic-byte-identical
    fi
else
    pass audit-deterministic-byte-identical-skipped
fi

# ---- Case 6: belt-and-suspenders — no production check leaks "panicked" through evidence ----
# T01's safeRun should never trigger on the real registry, but this asserts the
# wire-level invariant: a finding's .evidence string never contains "panicked".
if jq -e '[.findings[] | .evidence | tostring | contains("panicked")] | any | not' \
       <"$TMPDIR_S03/audit-echo-json.out" >/dev/null 2>&1; then
    pass panic-evidence-not-leaking
else
    fail panic-evidence-not-leaking "at least one finding's evidence contains 'panicked'"
fi

# ---- Regression: chain through verify-s02.sh (which inline-calls verify-s01.sh) ----
if BIN="$BIN" bash "$(dirname "$0")/verify-s02.sh" \
       >"$TMPDIR_S03/verify-s02.out" 2>"$TMPDIR_S03/verify-s02.err"; then
    pass s02-regression
else
    fail s02-regression "verify-s02.sh failed; output: $(cat "$TMPDIR_S03/verify-s02.out") $(cat "$TMPDIR_S03/verify-s02.err")"
fi

echo
if (( fails > 0 )); then
    echo "verify-s03: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s03: $passes checks passed"
