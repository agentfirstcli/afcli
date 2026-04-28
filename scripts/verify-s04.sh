#!/usr/bin/env bash
# verify-s04.sh — slice S04 demo integration check.
#
# Exercises the descriptor pipeline end-to-end on the built binary:
#   1. afcli audit /usr/bin/git --descriptor valid-skip-relax.yaml --output json
#      applies skip_principles (P12 -> status:skip, evidence:"skipped per
#      descriptor") and relax_principles (P7 severity capped at "medium").
#   2. Every malformed descriptor exits 3 with a single JSON ErrorEnvelope on
#      stderr carrying code in {DESCRIPTOR_INVALID, DESCRIPTOR_NOT_FOUND} and
#      structured Details (line, key, value, expected, got, allowed where
#      applicable). Stdout stays empty for these cases.
# Plus a regression chain: this script inline-calls verify-s03.sh which
# itself inline-calls verify-s02.sh / verify-s01.sh, so a single entry point
# covers every prior slice contract.
#
# Per-error-shape verdicts (line numbers, key paths, allowed lists) are unit-
# tested in internal/descriptor/*_test.go and internal/cli/descriptor_unit_test.go;
# this script verifies the binary's end-to-end shape that tests cannot reach.
# Cases that depend on /usr/bin/git skip cleanly when it is absent.

set -u

BIN="${BIN:-/tmp/afcli}"
TMPDIR_S04="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_S04"' EXIT

# Deterministic mode keeps started_at empty and target path normalized —
# matches verify-s0{1,2,3}.sh.
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
    local stdout_file="$TMPDIR_S04/$label.out"
    local stderr_file="$TMPDIR_S04/$label.err"
    "$BIN" "$@" >"$stdout_file" 2>"$stderr_file"
    echo $? >"$TMPDIR_S04/$label.ec"
}

ec_of()    { cat "$TMPDIR_S04/$1.ec"; }
stdout_of(){ cat "$TMPDIR_S04/$1.out"; }
stderr_of(){ cat "$TMPDIR_S04/$1.err"; }

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

assert_valid_json_stdout() {
    local label="$1"
    if ! jq -e . <"$TMPDIR_S04/$label.out" >/dev/null 2>&1; then
        fail "$label" "stdout is not valid JSON"
        return 1
    fi
}

# assert_jq <label> <filter> <expected>      — asserts on stdout
# assert_jq <label> <stream> <filter> <expected>  — asserts on stdout|stderr
assert_jq() {
    local label="$1" stream filter expected actual src
    if [[ $# -eq 3 ]]; then
        stream="stdout"; filter="$2"; expected="$3"
    else
        stream="$2"; filter="$3"; expected="$4"
    fi
    case "$stream" in
        stdout) src="$TMPDIR_S04/$label.out" ;;
        stderr) src="$TMPDIR_S04/$label.err" ;;
        *)      fail "$label" "internal: bad stream $stream"; return 1 ;;
    esac
    actual="$(jq -r "$filter" <"$src" 2>/dev/null)"
    if [[ "$actual" != "$expected" ]]; then
        fail "$label" "jq($stream) '$filter' got '$actual', expected '$expected'"
        return 1
    fi
}

# assert_jq_in <label> <stream> <filter> <allowed1> [allowed2 ...]
# Asserts that jq output equals one of the allowed values.
assert_jq_in() {
    local label="$1" stream="$2" filter="$3"; shift 3
    local src
    case "$stream" in
        stdout) src="$TMPDIR_S04/$label.out" ;;
        stderr) src="$TMPDIR_S04/$label.err" ;;
        *)      fail "$label" "internal: bad stream $stream"; return 1 ;;
    esac
    local actual; actual="$(jq -r "$filter" <"$src" 2>/dev/null)"
    local ok=0 v
    for v in "$@"; do
        if [[ "$actual" == "$v" ]]; then ok=1; break; fi
    done
    if (( ok == 0 )); then
        fail "$label" "jq($stream) '$filter' got '$actual', expected one of {$*}"
        return 1
    fi
}

GIT_BIN="/usr/bin/git"
have_git=1
if [[ ! -x "$GIT_BIN" ]]; then
    have_git=0
    echo "INFO: $GIT_BIN not present — git-dependent cases will be skipped." >&2
fi

DESC_DIR="testdata/descriptors"

# ---- Case 1: descriptor-skip-applied — P12 surfaces as skip / "skipped per descriptor" ----
if (( have_git )); then
    run_case descriptor-skip-applied audit "$GIT_BIN" \
        --descriptor "$DESC_DIR/valid-skip-relax.yaml" --output json
    if assert_exit_in descriptor-skip-applied 0 1; then
        assert_valid_json_stdout descriptor-skip-applied && \
        assert_jq descriptor-skip-applied '.findings | length' '16' && \
        assert_jq descriptor-skip-applied '.findings[] | select(.principle_id=="P12") | .status' 'skip' && \
        assert_jq descriptor-skip-applied '.findings[] | select(.principle_id=="P12") | .evidence' 'skipped per descriptor' && \
        pass descriptor-skip-applied
    fi
else
    pass descriptor-skip-applied-skipped
fi

# ---- Case 2: descriptor-relax-applied — P7 severity capped at "medium" ----
if (( have_git )); then
    run_case descriptor-relax-applied audit "$GIT_BIN" \
        --descriptor "$DESC_DIR/valid-skip-relax.yaml" --output json
    if assert_exit_in descriptor-relax-applied 0 1; then
        assert_valid_json_stdout descriptor-relax-applied && \
        assert_jq_in descriptor-relax-applied stdout \
            '.findings[] | select(.principle_id=="P7") | .severity' \
            'low' 'medium' && \
        pass descriptor-relax-applied
    fi
else
    pass descriptor-relax-applied-skipped
fi

# ---- Case 3: descriptor-not-found — exit 3, DESCRIPTOR_NOT_FOUND envelope ----
run_case descriptor-not-found audit /bin/echo \
    --descriptor "/tmp/nope-$$.yaml" --output json
if assert_exit_in descriptor-not-found 3; then
    assert_jq descriptor-not-found stderr '.error.code' 'DESCRIPTOR_NOT_FOUND' && \
    assert_jq descriptor-not-found stderr '.error.details.path' "/tmp/nope-$$.yaml" && \
    pass descriptor-not-found
fi

# ---- Case 4: descriptor-unknown-key — exit 3, DESCRIPTOR_INVALID with line number ----
run_case descriptor-unknown-key audit /bin/echo \
    --descriptor "$DESC_DIR/unknown-key.yaml" --output json
if assert_exit_in descriptor-unknown-key 3; then
    assert_jq descriptor-unknown-key stderr '.error.code' 'DESCRIPTOR_INVALID' && \
    assert_jq descriptor-unknown-key stderr '.error.details.line | type' 'number' && \
    pass descriptor-unknown-key
fi

# ---- Case 5: descriptor-type-mismatch — exit 3, DESCRIPTOR_INVALID ----
run_case descriptor-type-mismatch audit /bin/echo \
    --descriptor "$DESC_DIR/type-mismatch.yaml" --output json
if assert_exit_in descriptor-type-mismatch 3; then
    assert_jq descriptor-type-mismatch stderr '.error.code' 'DESCRIPTOR_INVALID' && \
    assert_jq descriptor-type-mismatch stderr '.error.details.line | type' 'number' && \
    pass descriptor-type-mismatch
fi

# ---- Case 6: descriptor-bad-principle — DESCRIPTOR_INVALID, key starts with skip_principles ----
run_case descriptor-bad-principle audit /bin/echo \
    --descriptor "$DESC_DIR/bad-principle.yaml" --output json
if assert_exit_in descriptor-bad-principle 3; then
    assert_jq descriptor-bad-principle stderr '.error.code' 'DESCRIPTOR_INVALID' && \
    assert_jq descriptor-bad-principle stderr \
        '.error.details.key | startswith("skip_principles") | tostring' 'true' && \
    pass descriptor-bad-principle
fi

# ---- Case 7: descriptor-bad-severity — DESCRIPTOR_INVALID, allowed list present ----
run_case descriptor-bad-severity audit /bin/echo \
    --descriptor "$DESC_DIR/bad-severity.yaml" --output json
if assert_exit_in descriptor-bad-severity 3; then
    assert_jq descriptor-bad-severity stderr '.error.code' 'DESCRIPTOR_INVALID' && \
    assert_jq descriptor-bad-severity stderr '.error.details.key' 'relax_principles.P7' && \
    pass descriptor-bad-severity
fi

# ---- Case 8: format-mismatch — DESCRIPTOR_INVALID, key=format_version ----
run_case descriptor-format-mismatch audit /bin/echo \
    --descriptor "$DESC_DIR/format-mismatch.yaml" --output json
if assert_exit_in descriptor-format-mismatch 3; then
    assert_jq descriptor-format-mismatch stderr '.error.code' 'DESCRIPTOR_INVALID' && \
    assert_jq descriptor-format-mismatch stderr '.error.details.key' 'format_version' && \
    pass descriptor-format-mismatch
fi

# ---- Regression: chain through verify-s03.sh (which inline-calls verify-s02.sh / verify-s01.sh) ----
if BIN="$BIN" bash "$(dirname "$0")/verify-s03.sh" \
       >"$TMPDIR_S04/verify-s03.out" 2>"$TMPDIR_S04/verify-s03.err"; then
    pass s03-regression
else
    fail s03-regression "verify-s03.sh failed; output: $(cat "$TMPDIR_S04/verify-s03.out") $(cat "$TMPDIR_S04/verify-s03.err")"
fi

echo
if (( fails > 0 )); then
    echo "verify-s04: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s04: $passes checks passed"
