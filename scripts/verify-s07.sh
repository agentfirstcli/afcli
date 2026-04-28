#!/usr/bin/env bash
# verify-s07.sh — slice S07 demo integration check.
#
# Single local entry point that regression-tests the entire M001 contract.
# Each case echoes a short label/value diagnostic on the way through so the
# script log reads as a one-line-per-budget timeline. Skip-with-pass on
# absent dependencies (e.g. /usr/bin/git) keeps the script useful in
# heterogeneous environments without false negatives.
#
# Cases:
#   1. fail-on-default-high-self-audit-passes — `audit ./afcli` exits 0 at
#      default --fail-on high (zero fail findings is the M001 contract).
#   2. fail-on-rejects-bogus-value — exit 2 + USAGE envelope on stderr.
#   3. fail-on-never-overrides-threshold — exit 0 even if engine ever
#      emits a fail (today no-op safety net; canary for future).
#   4. fail-on-accepts-all-five-values — every documented value yields
#      an exit in {0,1}, never 2/3.
#   5. golden-self-audit-byte-identical — diff against the pinned wire form.
#   6. markdown-parity-self-audit — JSON principle ids match markdown ids.
#   7. external-target-git-schema-shape — 16 findings + manifest_version
#      + every principle_id matches ^P\d+$ (skip-with-pass if no git).
#   8. perf-cold-start-under-100ms — `--help` cold start budget.
#   9. perf-static-audit-under-2s — `audit /bin/echo` budget.
#  10. perf-stripped-binary-under-20MB — production binary size budget.
#  11. s06-regression — chains s05→s04→s03→s02→s01 via verify-s06.sh.
#
# Per-check verdicts and unit-level invariants are tested in
# internal/audit/*_test.go and internal/cli/*_test.go; this script verifies
# the binary's end-to-end shape that tests cannot reach.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

TMPDIR_S07="$(mktemp -d)"
BUILT_PROD_BIN=0

cleanup() {
    rm -rf "$TMPDIR_S07"
    if (( BUILT_PROD_BIN )); then
        rm -f "$REPO_ROOT/afcli"
    fi
}
trap cleanup EXIT

# When BIN is unset we build a stripped production binary into ./afcli (in
# the repo root) so cases that audit ./afcli — and the golden case in
# particular — see the same wire form CI does. When BIN is provided we
# trust the caller; CI sets BIN=./afcli before invoking this script.
if [[ -z "${BIN:-}" ]]; then
    if [[ ! -x "./afcli" ]]; then
        CGO_ENABLED=0 go build -ldflags='-s -w' -o ./afcli ./cmd/afcli || {
            echo "verify-s07: failed to build ./afcli" >&2
            exit 1
        }
        BUILT_PROD_BIN=1
    fi
    BIN="./afcli"
fi
export BIN

# Deterministic mode keeps started_at empty, paths normalized, and details
# map keys sorted — matches verify-s0{1..6}.sh.
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
    local stdout_file="$TMPDIR_S07/$label.out"
    local stderr_file="$TMPDIR_S07/$label.err"
    "$BIN" "$@" >"$stdout_file" 2>"$stderr_file"
    echo $? >"$TMPDIR_S07/$label.ec"
}

ec_of()    { cat "$TMPDIR_S07/$1.ec"; }
stdout_of(){ cat "$TMPDIR_S07/$1.out"; }
stderr_of(){ cat "$TMPDIR_S07/$1.err"; }

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
        stdout) src="$TMPDIR_S07/$label.out" ;;
        stderr) src="$TMPDIR_S07/$label.err" ;;
        *)      fail "$label" "internal: bad stream $stream"; return 1 ;;
    esac
    actual="$(jq -r "$filter" <"$src" 2>/dev/null)"
    if [[ "$actual" != "$expected" ]]; then
        fail "$label" "jq($stream) '$filter' got '$actual', expected '$expected'"
        return 1
    fi
}

# ---- Case 1: fail-on-default-high-self-audit-passes ----
run_case fail-on-default-high-self-audit-passes audit ./afcli --output json
case1_ec="$(ec_of fail-on-default-high-self-audit-passes)"
echo "fail-on-default-high-self-audit-passes: exit=${case1_ec}"
if assert_exit_in fail-on-default-high-self-audit-passes 0; then
    pass fail-on-default-high-self-audit-passes
fi

# ---- Case 2: fail-on-rejects-bogus-value ----
run_case fail-on-rejects-bogus-value audit /bin/echo --fail-on nuclear --output json
if assert_exit_in fail-on-rejects-bogus-value 2; then
    if assert_jq fail-on-rejects-bogus-value stderr '.error.code' 'USAGE'; then
        actual_stdout="$(stdout_of fail-on-rejects-bogus-value)"
        if [[ -n "$actual_stdout" ]]; then
            fail fail-on-rejects-bogus-value "expected empty stdout, got: $actual_stdout"
        else
            pass fail-on-rejects-bogus-value
        fi
    fi
fi

# ---- Case 3: fail-on-never-overrides-threshold ----
run_case fail-on-never-overrides-threshold audit /bin/echo --fail-on never --output json
if assert_exit_in fail-on-never-overrides-threshold 0; then
    pass fail-on-never-overrides-threshold
fi

# ---- Case 4: fail-on-accepts-all-five-values ----
fail4=0
for sev in low medium high critical never; do
    label="fail-on-accepts-all-five-values-${sev}"
    run_case "$label" audit /bin/echo --fail-on "$sev" --output json
    actual="$(ec_of "$label")"
    case "$actual" in
        0|1) ;;
        *)
            fail fail-on-accepts-all-five-values "value=${sev} got exit ${actual}, expected 0 or 1"
            fail4=1
            ;;
    esac
done
if (( fail4 == 0 )); then
    pass fail-on-accepts-all-five-values
fi

# ---- Case 5: golden-self-audit-byte-identical ----
run_case golden-self-audit-byte-identical audit ./afcli --output json
if assert_exit_in golden-self-audit-byte-identical 0 1; then
    if diff -q "$TMPDIR_S07/golden-self-audit-byte-identical.out" \
              testdata/golden-self-audit.json >/dev/null 2>&1; then
        pass golden-self-audit-byte-identical
    else
        diff -u testdata/golden-self-audit.json \
                "$TMPDIR_S07/golden-self-audit-byte-identical.out" \
                > "$TMPDIR_S07/golden.diff" 2>&1 || true
        fail golden-self-audit-byte-identical \
             "byte diff against testdata/golden-self-audit.json — first 40 lines:
$(head -n 40 "$TMPDIR_S07/golden.diff")"
    fi
fi

# ---- Case 6: markdown-parity-self-audit ----
run_case markdown-parity-self-audit-json audit ./afcli --output json
run_case markdown-parity-self-audit-md   audit ./afcli --output markdown
if assert_exit_in markdown-parity-self-audit-json 0 1 \
   && assert_exit_in markdown-parity-self-audit-md 0 1; then
    jq -r '.findings[].principle_id' \
        <"$TMPDIR_S07/markdown-parity-self-audit-json.out" \
        | sort -u >"$TMPDIR_S07/json.ids"
    grep -oE 'P[0-9]+' \
        <"$TMPDIR_S07/markdown-parity-self-audit-md.out" \
        | sort -u >"$TMPDIR_S07/md.ids"
    if diff -q "$TMPDIR_S07/json.ids" "$TMPDIR_S07/md.ids" >/dev/null 2>&1; then
        pass markdown-parity-self-audit
    else
        diff -u "$TMPDIR_S07/json.ids" "$TMPDIR_S07/md.ids" \
            >"$TMPDIR_S07/md-parity.diff" 2>&1 || true
        fail markdown-parity-self-audit "principle id sets differ:
$(cat "$TMPDIR_S07/md-parity.diff")"
    fi
fi

# ---- Case 7: external-target-git-schema-shape ----
GIT_BIN="/usr/bin/git"
if [[ -x "$GIT_BIN" ]]; then
    run_case external-target-git-schema-shape audit "$GIT_BIN" --output json
    if assert_exit_in external-target-git-schema-shape 0 1; then
        if assert_jq external-target-git-schema-shape '.findings | length' '16'; then
            mv_present="$(jq -r 'has("manifest_version")' \
                <"$TMPDIR_S07/external-target-git-schema-shape.out" 2>/dev/null)"
            if [[ "$mv_present" != "true" ]]; then
                fail external-target-git-schema-shape \
                     "manifest_version key absent in JSON"
            else
                all_match="$(jq -r 'all(.findings[]; .principle_id | test("^P[0-9]+$"))' \
                    <"$TMPDIR_S07/external-target-git-schema-shape.out" 2>/dev/null)"
                if [[ "$all_match" != "true" ]]; then
                    fail external-target-git-schema-shape \
                         "at least one principle_id failed the ^P[0-9]+\$ check"
                else
                    pass external-target-git-schema-shape
                fi
            fi
        fi
    fi
else
    echo "INFO: $GIT_BIN absent — case skipped." >&2
    pass external-target-git-schema-shape-skipped
fi

# ---- Case 8: perf-cold-start-under-100ms ----
TIMEFORMAT='%R'
elapsed_help=$( { time "$BIN" --help >/dev/null; } 2>&1 )
echo "perf-cold-start: ${elapsed_help}s"
if awk -v t="$elapsed_help" 'BEGIN { exit !(t < 0.1) }'; then
    pass perf-cold-start-under-100ms
else
    fail perf-cold-start-under-100ms \
         "cold start ${elapsed_help}s exceeds 0.1s budget"
fi

# ---- Case 9: perf-static-audit-under-2s ----
elapsed_audit=$( { time "$BIN" audit /bin/echo --output json --deterministic >/dev/null; } 2>&1 )
echo "perf-static-audit: ${elapsed_audit}s"
if awk -v t="$elapsed_audit" 'BEGIN { exit !(t < 2.0) }'; then
    pass perf-static-audit-under-2s
else
    fail perf-static-audit-under-2s \
         "static audit ${elapsed_audit}s exceeds 2.0s budget"
fi

# ---- Case 10: perf-stripped-binary-under-20MB ----
STRIPPED="$TMPDIR_S07/afcli-stripped"
if CGO_ENABLED=0 go build -ldflags='-s -w' -o "$STRIPPED" ./cmd/afcli; then
    size=$(wc -c < "$STRIPPED" | tr -d ' ')
    echo "perf-stripped-binary: ${size} bytes"
    if (( size < 20971520 )); then
        pass perf-stripped-binary-under-20MB
    else
        fail perf-stripped-binary-under-20MB \
             "stripped binary ${size} bytes exceeds 20971520 (20 MiB) budget"
    fi
else
    fail perf-stripped-binary-under-20MB "stripped build failed"
fi

# ---- Case 11: s06-regression — chains s05→s04→s03→s02→s01 ----
if BIN="$BIN" bash "$(dirname "$0")/verify-s06.sh" \
       >"$TMPDIR_S07/verify-s06.out" 2>"$TMPDIR_S07/verify-s06.err"; then
    pass s06-regression
else
    fail s06-regression "verify-s06.sh failed; output:
$(cat "$TMPDIR_S07/verify-s06.out")
$(cat "$TMPDIR_S07/verify-s06.err")"
fi

echo
if (( fails > 0 )); then
    echo "verify-s07: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s07: $passes checks passed"
