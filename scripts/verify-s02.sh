#!/usr/bin/env bash
# verify-s02.sh — slice S02 demo integration check.
#
# Exercises the slice's three deliverables end-to-end on the built binary:
#   1. afcli manifest list --output {json,text,markdown}
#   2. afcli --help-schema (root + per-subcommand)
#   3. afcli audit /bin/echo --output json carries the embedded
#      manifest_version (no v0-placeholder regression)
# Plus a determinism check (--help-schema invoked twice must be byte-
# identical) and a regression check that scripts/verify-s01.sh still
# passes after the placeholder swap.
#
# Schema validity for --help-schema is enforced by the Go test suite
# (internal/cli/help_schema_test.go); manifest principle count/density
# is enforced by internal/manifest/manifest_test.go. This script
# verifies the binary's end-to-end behavior the tests cannot reach.

set -u

BIN="${BIN:-/tmp/afcli}"
TMPDIR_S02="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_S02"' EXIT

# Deterministic mode keeps started_at empty and target relative — makes
# field assertions stable across runs and machines (matches verify-s01.sh).
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
    local stdout_file="$TMPDIR_S02/$label.out"
    local stderr_file="$TMPDIR_S02/$label.err"
    "$BIN" "$@" >"$stdout_file" 2>"$stderr_file"
    echo $? >"$TMPDIR_S02/$label.ec"
}

ec_of()    { cat "$TMPDIR_S02/$1.ec"; }
stdout_of(){ cat "$TMPDIR_S02/$1.out"; }
stderr_of(){ cat "$TMPDIR_S02/$1.err"; }

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
    if ! jq -e . <"$TMPDIR_S02/$label.out" >/dev/null 2>&1; then
        fail "$label" "stdout is not valid JSON"
        return 1
    fi
}

# ---- Case 1: manifest list --output json ----
run_case manifest-list-json manifest list --output json
if assert_exit manifest-list-json 0; then
    assert_valid_json_stdout manifest-list-json && \
    assert_contains manifest-list-json stdout '"manifest_version": "0.0.1"' && \
    assert_contains manifest-list-json stdout '"P1"' && \
    assert_contains manifest-list-json stdout '"P16"' && \
    assert_empty    manifest-list-json stderr && \
    pass manifest-list-json
fi

# ---- Case 2: manifest list --output text ----
run_case manifest-list-text manifest list --output text
if assert_exit manifest-list-text 0; then
    assert_contains  manifest-list-text stdout "P1" && \
    assert_contains  manifest-list-text stdout "P16" && \
    assert_min_lines manifest-list-text stdout 16 && \
    assert_empty     manifest-list-text stderr && \
    pass manifest-list-text
fi

# ---- Case 3: manifest list --output markdown ----
run_case manifest-list-markdown manifest list --output markdown
if assert_exit manifest-list-markdown 0; then
    assert_contains manifest-list-markdown stdout "# afcli manifest" && \
    assert_contains manifest-list-markdown stdout "## " && \
    assert_empty    manifest-list-markdown stderr && \
    pass manifest-list-markdown
fi

# ---- Case 4: --help-schema (root) ----
run_case help-schema-root --help-schema
if assert_exit help-schema-root 0; then
    assert_valid_json_stdout help-schema-root && \
    assert_contains help-schema-root stdout '"name": "audit"' && \
    assert_contains help-schema-root stdout '"name": "manifest"' && \
    assert_contains help-schema-root stdout '"manifest_version": "0.0.1"' && \
    assert_empty    help-schema-root stderr && \
    pass help-schema-root
fi

# ---- Case 5: audit --help-schema (per-subcommand scope) ----
run_case help-schema-audit audit --help-schema
if assert_exit help-schema-audit 0; then
    assert_valid_json_stdout help-schema-audit && \
    assert_contains     help-schema-audit stdout '"name": "audit"' && \
    assert_not_contains help-schema-audit stdout '"name": "manifest"' && \
    assert_empty        help-schema-audit stderr && \
    pass help-schema-audit
fi

# ---- Case 6: audit /bin/echo carries embedded manifest_version (no v0-placeholder regression) ----
run_case audit-manifest-version-live audit /bin/echo --output json
if assert_exit audit-manifest-version-live 0; then
    assert_valid_json_stdout audit-manifest-version-live && \
    assert_contains     audit-manifest-version-live stdout '"manifest_version": "0.0.1"' && \
    assert_not_contains audit-manifest-version-live stdout 'v0-placeholder' && \
    assert_empty        audit-manifest-version-live stderr && \
    pass audit-manifest-version-live
fi

# ---- Case 7: --help-schema is byte-identical across invocations ----
"$BIN" --help-schema >"$TMPDIR_S02/help-schema-a.out" 2>"$TMPDIR_S02/help-schema-a.err"
ec_a=$?
"$BIN" --help-schema >"$TMPDIR_S02/help-schema-b.out" 2>"$TMPDIR_S02/help-schema-b.err"
ec_b=$?
if [[ "$ec_a" != "0" || "$ec_b" != "0" ]]; then
    fail help-schema-deterministic "non-zero exit (a=$ec_a, b=$ec_b)"
elif ! diff -q "$TMPDIR_S02/help-schema-a.out" "$TMPDIR_S02/help-schema-b.out" >/dev/null; then
    fail help-schema-deterministic "stdout differs across invocations"
else
    pass help-schema-deterministic
fi

# ---- Regression: S01 still green after the placeholder swap ----
if BIN="$BIN" bash "$(dirname "$0")/verify-s01.sh" >"$TMPDIR_S02/verify-s01.out" 2>"$TMPDIR_S02/verify-s01.err"; then
    pass s01-regression
else
    fail s01-regression "verify-s01.sh failed; output: $(cat "$TMPDIR_S02/verify-s01.out") $(cat "$TMPDIR_S02/verify-s01.err")"
fi

echo
if (( fails > 0 )); then
    echo "verify-s02: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s02: $passes checks passed"
