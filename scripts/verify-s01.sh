#!/usr/bin/env bash
# verify-s01.sh — slice S01 demo integration check.
#
# Exercises the five contract cases defined in S01-PLAN.md:
#   1. audit /bin/echo --output json     → exit 0, JSON report on stdout
#   2. audit /bin/echo --output text     → exit 0, text report on stdout
#   3. audit /bin/echo --output markdown → exit 0, markdown report on stdout
#   4. audit /nonexistent                → exit 3, TARGET_NOT_FOUND envelope on stderr
#   5. afcli --bogus-flag                → exit 2, USAGE envelope on stderr
#
# Asserts the stdout/stderr split, exit code, and key envelope/header
# fields for each case. Schema validity is enforced by the Go test suite
# (internal/report/schema_test.go) — this script verifies the binary's
# end-to-end behavior the tests cannot reach.

set -u

BIN="${BIN:-/tmp/afcli}"
TMPDIR_S01="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_S01"' EXIT

# Deterministic mode keeps started_at empty and target relative — makes
# field assertions stable across runs and machines.
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
    local stdout_file="$TMPDIR_S01/$label.out"
    local stderr_file="$TMPDIR_S01/$label.err"
    "$BIN" "$@" >"$stdout_file" 2>"$stderr_file"
    echo $? >"$TMPDIR_S01/$label.ec"
}

ec_of()    { cat "$TMPDIR_S01/$1.ec"; }
stdout_of(){ cat "$TMPDIR_S01/$1.out"; }
stderr_of(){ cat "$TMPDIR_S01/$1.err"; }

assert_exit() {
    local label="$1" expected="$2" actual
    actual="$(ec_of "$label")"
    if [[ "$actual" != "$expected" ]]; then
        fail "$label" "expected exit=$expected, got $actual"
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

assert_empty() {
    local label="$1" stream="$2" hay
    case "$stream" in
        stdout) hay="$(stdout_of "$label")" ;;
        stderr) hay="$(stderr_of "$label")" ;;
    esac
    if [[ -n "$hay" ]]; then
        fail "$label" "$stream should be empty but was: $hay"
        return 1
    fi
}

# ---- Case 1: success, --output json ----
# S03 wired the static check engine into RunE, so /bin/echo now produces 16
# findings instead of the S01-era empty array — the S01 envelope contract
# being verified here is the report shape (manifest/afcli versions, target,
# findings key on stdout, empty stderr), not the absent-engine placeholder.
run_case audit-json audit /bin/echo --output json --fail-on never
if assert_exit audit-json 0; then
    assert_contains audit-json stdout '"manifest_version"' && \
    assert_contains audit-json stdout '"afcli_version"' && \
    assert_contains audit-json stdout '"findings":' && \
    assert_contains audit-json stdout '"target":' && \
    assert_empty    audit-json stderr && \
    pass audit-json
fi

# ---- Case 2: success, --output text ----
run_case audit-text audit /bin/echo --output text --fail-on never
if assert_exit audit-text 0; then
    assert_contains audit-text stdout "afcli " && \
    assert_contains audit-text stdout "manifest " && \
    assert_empty    audit-text stderr && \
    pass audit-text
fi

# ---- Case 3: success, --output markdown ----
run_case audit-md audit /bin/echo --output markdown --fail-on never
if assert_exit audit-md 0; then
    assert_contains audit-md stdout "# afcli audit report" && \
    assert_contains audit-md stdout "manifest_version" && \
    assert_empty    audit-md stderr && \
    pass audit-md
fi

# ---- Case 4: TARGET_NOT_FOUND, default format (json) ----
run_case audit-missing audit /nonexistent-binary-for-s01-verify
if assert_exit audit-missing 3; then
    assert_empty    audit-missing stdout && \
    assert_contains audit-missing stderr '"code": "TARGET_NOT_FOUND"' && \
    assert_contains audit-missing stderr '"target": "/nonexistent-binary-for-s01-verify"' && \
    pass audit-missing
fi

# ---- Case 5: USAGE on bogus flag, default format (json) ----
run_case usage-bogus --bogus-flag
if assert_exit usage-bogus 2; then
    assert_empty    usage-bogus stdout && \
    assert_contains usage-bogus stderr '"code": "USAGE"' && \
    pass usage-bogus
fi

# ---- Bonus: identical envelope code across all three formats ----
run_case envelope-text  audit /nonexistent --output text
run_case envelope-md    audit /nonexistent --output markdown
if assert_exit envelope-text 3 && assert_exit envelope-md 3; then
    assert_contains envelope-text stderr "TARGET_NOT_FOUND" && \
    assert_contains envelope-md   stderr "TARGET_NOT_FOUND" && \
    pass envelope-format-parity
fi

echo
if (( fails > 0 )); then
    echo "verify-s01: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s01: $passes checks passed"
