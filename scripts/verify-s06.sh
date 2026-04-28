#!/usr/bin/env bash
# verify-s06.sh — slice S06 demo integration check.
#
# Exercises the closed-stub-loop contract end-to-end on the built binary:
#   1. afcli audit /usr/bin/git --output json produces 16 findings, none
#      carrying the "no automated check yet" stub blurb (R008 closure).
#   2. Same shape against /bin/echo (works on hosts without git).
#   3. P2/P5/P9/P11/P12 are review-only checks: kind=requires-review and
#      status=review for every one.
#   4. afcli init <target> --out <path> writes a scaffold that round-trips
#      cleanly through descriptor.Load (audit exits in {0,1}, never 3).
#   5. afcli init refuses to overwrite an existing file: exit 3,
#      INIT_FILE_EXISTS envelope on stderr, stdout empty.
#   6. afcli init --force overwrites: target field is rewritten and the
#      replacement descriptor still round-trips.
#   7. afcli init escapes shell-quote characters in the positional target
#      so the produced YAML still round-trips through descriptor.Load.
#   8. afcli --help-schema --output json exposes the new init subcommand
#      and the INIT_FILE_EXISTS error code (R014 surface hygiene).
# Plus a regression chain: this script inline-calls verify-s05.sh which
# itself inline-calls the prior verify-s0N.sh, so a single entry point
# covers every prior slice contract.
#
# Per-check verdicts (severity table, manifest-derived rationale, descriptor
# decoration interaction with P3 probe-failure replacement) are unit-tested
# in internal/audit/*_test.go and internal/cli/init_test.go; this script
# verifies the binary's end-to-end shape that tests cannot reach. Cases
# that depend on /usr/bin/git skip cleanly when it is absent (CI macOS
# friendliness — copies the verify-s03.sh skip pattern).

set -u

BIN="${BIN:-/tmp/afcli}"
TMPDIR_S06="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_S06"' EXIT

# Deterministic mode keeps started_at empty and target path normalized —
# matches verify-s0{1,2,3,4,5}.sh.
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
    local stdout_file="$TMPDIR_S06/$label.out"
    local stderr_file="$TMPDIR_S06/$label.err"
    "$BIN" "$@" >"$stdout_file" 2>"$stderr_file"
    echo $? >"$TMPDIR_S06/$label.ec"
}

ec_of()    { cat "$TMPDIR_S06/$1.ec"; }
stdout_of(){ cat "$TMPDIR_S06/$1.out"; }
stderr_of(){ cat "$TMPDIR_S06/$1.err"; }

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

# assert_jq <label> <filter> <expected>             — asserts on stdout
# assert_jq <label> <stream> <filter> <expected>    — asserts on stdout|stderr
assert_jq() {
    local label="$1" stream filter expected actual src
    if [[ $# -eq 3 ]]; then
        stream="stdout"; filter="$2"; expected="$3"
    else
        stream="$2"; filter="$3"; expected="$4"
    fi
    case "$stream" in
        stdout) src="$TMPDIR_S06/$label.out" ;;
        stderr) src="$TMPDIR_S06/$label.err" ;;
        *)      fail "$label" "internal: bad stream $stream"; return 1 ;;
    esac
    actual="$(jq -r "$filter" <"$src" 2>/dev/null)"
    if [[ "$actual" != "$expected" ]]; then
        fail "$label" "jq($stream) '$filter' got '$actual', expected '$expected'"
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

# Per-principle review-only assertion: walks five principle ids and asserts
# each one's kind is requires-review and status is review on the same JSON.
assert_review_only_for() {
    local label="$1" pid
    shift
    for pid in "$@"; do
        local kind status
        kind="$(jq -r ".findings[] | select(.principle_id==\"$pid\") | .kind" \
            <"$TMPDIR_S06/$label.out" 2>/dev/null)"
        status="$(jq -r ".findings[] | select(.principle_id==\"$pid\") | .status" \
            <"$TMPDIR_S06/$label.out" 2>/dev/null)"
        if [[ "$kind" != "requires-review" ]]; then
            fail "$label" "$pid kind got '$kind', expected 'requires-review'"
            return 1
        fi
        if [[ "$status" != "review" ]]; then
            fail "$label" "$pid status got '$status', expected 'review'"
            return 1
        fi
    done
}

GIT_BIN="/usr/bin/git"
have_git=1
if [[ ! -x "$GIT_BIN" ]]; then
    have_git=0
    echo "INFO: $GIT_BIN not present — git-dependent cases will be skipped." >&2
fi

# ---- Case 1: audit-git-sixteen-real-findings — 16 findings, zero stubs ----
if (( have_git )); then
    run_case audit-git-sixteen-real-findings audit "$GIT_BIN" --output json
    if assert_exit_in audit-git-sixteen-real-findings 0 1; then
        if assert_jq audit-git-sixteen-real-findings '.findings | length' '16'; then
            if assert_jq audit-git-sixteen-real-findings \
                '[.findings[].evidence | contains("no automated check yet")] | any | not | tostring' \
                'true'; then
                pass audit-git-sixteen-real-findings
            fi
        fi
    fi
else
    pass audit-git-sixteen-real-findings-skipped
fi

# ---- Case 2: audit-echo-sixteen-real-findings — same shape against /bin/echo ----
run_case audit-echo-sixteen-real-findings audit /bin/echo --output json
if assert_exit_in audit-echo-sixteen-real-findings 0 1; then
    if assert_jq audit-echo-sixteen-real-findings '.findings | length' '16'; then
        if assert_jq audit-echo-sixteen-real-findings \
            '[.findings[].evidence | contains("no automated check yet")] | any | not | tostring' \
            'true'; then
            pass audit-echo-sixteen-real-findings
        fi
    fi
fi

# ---- Case 3: audit-p2-p5-p9-p11-p12-are-review-only ----
# Reuses the JSON captured in case 2. Every one of these five principles must
# emit kind:requires-review with status:review — the manifest-derived
# rationale lives there and is unit-tested in checks_test.go.
run_case audit-p2-p5-p9-p11-p12-are-review-only audit /bin/echo --output json
if assert_exit_in audit-p2-p5-p9-p11-p12-are-review-only 0 1; then
    if assert_review_only_for audit-p2-p5-p9-p11-p12-are-review-only \
            P2 P5 P9 P11 P12; then
        pass audit-p2-p5-p9-p11-p12-are-review-only
    fi
fi

# ---- Case 4: init-round-trip — scaffold loads cleanly through descriptor.Load ----
INIT_DESC="$TMPDIR_S06/afcli.yaml"
run_case init-round-trip-write init mytool --out "$INIT_DESC"
if assert_exit_in init-round-trip-write 0; then
    if [[ -f "$INIT_DESC" ]]; then
        run_case init-round-trip-audit audit /bin/echo \
            --descriptor "$INIT_DESC" --output json
        if assert_exit_in init-round-trip-audit 0 1; then
            pass init-round-trip
        fi
    else
        fail init-round-trip "expected $INIT_DESC to exist after init"
    fi
fi

# ---- Case 5: init-refuses-overwrite-without-force ----
# A second init against the same path must exit 3 with INIT_FILE_EXISTS on
# stderr and an empty stdout (envelope is the only output channel for the
# refusal — matches the rest of the error envelope contract).
run_case init-refuses-overwrite-without-force init mytool --out "$INIT_DESC"
if assert_exit_in init-refuses-overwrite-without-force 3; then
    if assert_jq init-refuses-overwrite-without-force stderr \
            '.error.code' 'INIT_FILE_EXISTS'; then
        if assert_jq init-refuses-overwrite-without-force stderr \
                '.error.details.path' "$INIT_DESC"; then
            actual_stdout="$(stdout_of init-refuses-overwrite-without-force)"
            if [[ -n "$actual_stdout" ]]; then
                fail init-refuses-overwrite-without-force \
                    "expected empty stdout, got: $actual_stdout"
            else
                pass init-refuses-overwrite-without-force
            fi
        fi
    fi
fi

# ---- Case 6: init-force-overwrites — --force rewrites in place ----
run_case init-force-overwrites init othertool --out "$INIT_DESC" --force
if assert_exit_in init-force-overwrites 0; then
    if grep -q 'target: "othertool"' "$INIT_DESC"; then
        run_case init-force-overwrites-audit audit /bin/echo \
            --descriptor "$INIT_DESC" --output json
        if assert_exit_in init-force-overwrites-audit 0 1; then
            pass init-force-overwrites
        fi
    else
        fail init-force-overwrites \
            "expected 'target: \"othertool\"' in $INIT_DESC after --force; got: $(cat "$INIT_DESC")"
    fi
fi

# ---- Case 7: init-escapes-evil-target — strconv.Quote defuses YAML injection ----
EVIL_DESC="$TMPDIR_S06/evil.yaml"
run_case init-escapes-evil-target init 'evil"name' --out "$EVIL_DESC" --force
if assert_exit_in init-escapes-evil-target 0; then
    run_case init-escapes-evil-target-audit audit /bin/echo \
        --descriptor "$EVIL_DESC" --output json
    if assert_exit_in init-escapes-evil-target-audit 0 1; then
        pass init-escapes-evil-target
    fi
fi

# ---- Case 8: helpschema-lists-init-and-error-code ----
# --help-schema must surface both the new init subcommand and the new error
# code so an agent inspecting the binary can discover them statically.
run_case helpschema-lists-init-and-error-code --help-schema --output json
if assert_exit_in helpschema-lists-init-and-error-code 0; then
    if assert_jq helpschema-lists-init-and-error-code \
            '[.command.subcommands[].name] | index("init") | type' 'number'; then
        if assert_jq helpschema-lists-init-and-error-code \
                '.error_codes | index("INIT_FILE_EXISTS") | type' 'number'; then
            pass helpschema-lists-init-and-error-code
        fi
    fi
fi

# ---- Regression: chain through verify-s05.sh (covers s04→s03→s02→s01) ----
if BIN="$BIN" bash "$(dirname "$0")/verify-s05.sh" \
       >"$TMPDIR_S06/verify-s05.out" 2>"$TMPDIR_S06/verify-s05.err"; then
    pass s05-regression
else
    fail s05-regression "verify-s05.sh failed; output: $(cat "$TMPDIR_S06/verify-s05.out") $(cat "$TMPDIR_S06/verify-s05.err")"
fi

echo
if (( fails > 0 )); then
    echo "verify-s06: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s06: $passes checks passed"
