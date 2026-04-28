#!/usr/bin/env bash
# verify-s01-m002.sh — slice M002/S01 demo integration check.
#
# Proves the goreleaser snapshot contract holds end-to-end:
#   1. snapshot-fresh — goreleaser release --snapshot --clean produces dist/.
#   2. host-archive-extracts — host-OS/arch tar.gz extracts a runnable afcli.
#   3. version-flag-injected — `--version` reports a real version, never the
#      0.0.0-dev sentinel that vanilla `go build` produces.
#   4. helpschema-version-matches — `--help-schema | jq .afcli_version` ==
#      the version printed by --version.
#   5. audit-envelope-version-matches — `audit <descriptor>` envelope's
#      afcli_version field equals the same injected version.
#   6. helpschema-shape-byte-identical — vanilla `go build` and goreleaser
#      snapshot emit byte-identical --help-schema after both afcli_version
#      fields are rewritten to a placeholder. Locks the help-schema surface
#      to the source tree, not the build mode.
#   7. m001-contract-still-holds — verify-s07.sh exits 0 against the snapshot
#      binary (BIN=<extracted>/afcli). The snapshot's audit envelope reports
#      its injected version, so the testdata/golden-self-audit.json baseline
#      is patched in-place to the snapshot version for the run and restored
#      on exit. Every other byte must still match — that's the contract.
#
# Pass: every check prints PASS; exit 0.
# Fail: at least one check prints FAIL on stderr; exit 1.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v goreleaser >/dev/null 2>&1; then
    echo "verify-s01-m002: goreleaser not installed — required for slice contract" >&2
    exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
    echo "verify-s01-m002: jq not installed — required for envelope inspection" >&2
    exit 1
fi

TMPDIR_S01M2="$(mktemp -d)"
EXTRACT_DIR="$TMPDIR_S01M2/extract"
mkdir -p "$EXTRACT_DIR"

GOLDEN_PATH="$REPO_ROOT/testdata/golden-self-audit.json"
GOLDEN_BACKUP="$TMPDIR_S01M2/golden-self-audit.json.orig"
BUILT_LOCAL_AFCLI=0

cleanup() {
    if [[ -f "$GOLDEN_BACKUP" ]]; then
        cp "$GOLDEN_BACKUP" "$GOLDEN_PATH"
    fi
    if (( BUILT_LOCAL_AFCLI )); then
        rm -f "$REPO_ROOT/afcli"
    fi
    rm -rf "$TMPDIR_S01M2"
}
trap cleanup EXIT

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

# ---- Case 1: snapshot-fresh ----
if goreleaser release --snapshot --clean >"$TMPDIR_S01M2/snapshot.out" 2>&1; then
    pass snapshot-fresh
else
    fail snapshot-fresh "goreleaser snapshot failed; tail:
$(tail -n 40 "$TMPDIR_S01M2/snapshot.out")"
    echo
    echo "verify-s01-m002: aborting — no snapshot to check" >&2
    exit 1
fi

# ---- Case 2: host-archive-extracts ----
goos="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
    x86_64|amd64)   goarch=amd64 ;;
    aarch64|arm64)  goarch=arm64 ;;
    *)
        fail host-archive-extracts "unsupported host arch: $(uname -m)"
        echo
        echo "verify-s01-m002: aborting — cannot select host archive" >&2
        exit 1
        ;;
esac

shopt -s nullglob
host_archive_candidates=( "$REPO_ROOT/dist/afcli_"*"_${goos}_${goarch}.tar.gz" )
shopt -u nullglob
if (( ${#host_archive_candidates[@]} != 1 )); then
    fail host-archive-extracts \
         "expected exactly one dist/afcli_*_${goos}_${goarch}.tar.gz; got ${#host_archive_candidates[@]}: ${host_archive_candidates[*]}"
    echo
    exit 1
fi
HOST_ARCHIVE="${host_archive_candidates[0]}"

if tar -xzf "$HOST_ARCHIVE" -C "$EXTRACT_DIR"; then
    if [[ -x "$EXTRACT_DIR/afcli" ]]; then
        pass host-archive-extracts
    else
        fail host-archive-extracts "extracted tree missing afcli executable at $EXTRACT_DIR/afcli"
        echo
        exit 1
    fi
else
    fail host-archive-extracts "tar -xzf $HOST_ARCHIVE failed"
    echo
    exit 1
fi

EXTRACTED_BIN="$EXTRACT_DIR/afcli"

# ---- Case 3: version-flag-injected ----
version_line="$("$EXTRACTED_BIN" --version 2>/dev/null || true)"
echo "version-flag-injected: ${version_line}"
if [[ "$version_line" != "afcli "* ]]; then
    fail version-flag-injected "expected output starting with 'afcli '; got '$version_line'"
elif [[ "$version_line" == *"0.0.0-dev"* ]]; then
    fail version-flag-injected "expected non-dev version (snapshot ldflags); got '$version_line'"
else
    pass version-flag-injected
fi

# ---- Case 4: helpschema-version-matches ----
SNAPSHOT_VERSION="$("$EXTRACTED_BIN" --help-schema 2>/dev/null | jq -r '.afcli_version' 2>/dev/null || true)"
echo "helpschema-version: ${SNAPSHOT_VERSION}"
if [[ -z "$SNAPSHOT_VERSION" || "$SNAPSHOT_VERSION" == "null" ]]; then
    fail helpschema-version-matches "--help-schema afcli_version empty/null"
elif [[ "$SNAPSHOT_VERSION" == "0.0.0-dev" ]]; then
    fail helpschema-version-matches "expected injected version, got dev sentinel '$SNAPSHOT_VERSION'"
elif [[ "$version_line" != *"$SNAPSHOT_VERSION"* ]]; then
    fail helpschema-version-matches "--version line '$version_line' does not contain --help-schema version '$SNAPSHOT_VERSION'"
else
    pass helpschema-version-matches
fi

# ---- Case 5: audit-envelope-version-matches ----
# A YAML descriptor isn't executable, so afcli exits 3 with the envelope
# on stderr. The version field is identical on both streams, so 2>&1 is
# sufficient to harvest it.
audit_version="$("$EXTRACTED_BIN" audit testdata/descriptors/valid-skip-relax.yaml --output json 2>&1 | jq -r '.afcli_version' 2>/dev/null || true)"
echo "audit-envelope-version: ${audit_version}"
if [[ "$audit_version" != "$SNAPSHOT_VERSION" ]]; then
    fail audit-envelope-version-matches \
         "audit afcli_version '$audit_version' != help-schema '$SNAPSHOT_VERSION'"
else
    pass audit-envelope-version-matches
fi

# ---- Case 6: helpschema-shape-byte-identical ----
VANILLA_BIN="$TMPDIR_S01M2/afcli-vanilla"
if ! CGO_ENABLED=0 go build -o "$VANILLA_BIN" ./cmd/afcli >"$TMPDIR_S01M2/vanilla-build.out" 2>&1; then
    fail helpschema-shape-byte-identical "vanilla go build failed; tail:
$(tail -n 20 "$TMPDIR_S01M2/vanilla-build.out")"
else
    "$VANILLA_BIN"   --help-schema 2>/dev/null \
        | jq '.afcli_version = "<PLACEHOLDER>"' >"$TMPDIR_S01M2/help-schema.vanilla.json"
    "$EXTRACTED_BIN" --help-schema 2>/dev/null \
        | jq '.afcli_version = "<PLACEHOLDER>"' >"$TMPDIR_S01M2/help-schema.snapshot.json"
    if diff -q "$TMPDIR_S01M2/help-schema.vanilla.json" "$TMPDIR_S01M2/help-schema.snapshot.json" >/dev/null 2>&1; then
        pass helpschema-shape-byte-identical
    else
        diff -u "$TMPDIR_S01M2/help-schema.vanilla.json" "$TMPDIR_S01M2/help-schema.snapshot.json" \
            >"$TMPDIR_S01M2/help-schema.diff" 2>&1 || true
        fail helpschema-shape-byte-identical \
             "vanilla vs snapshot --help-schema diverge after version normalization; first 60 lines:
$(head -n 60 "$TMPDIR_S01M2/help-schema.diff")"
    fi
fi

# ---- Case 7: m001-contract-still-holds ----
# verify-s07.sh expects ./afcli at the repo root (golden case audits it) and
# uses BIN to choose the auditor. We:
#   - build a stripped ./afcli that matches the binary used to mint the
#     golden (vanilla `go build -ldflags='-s -w'` per verify-s07.sh:53),
#   - patch the golden's afcli_version to the snapshot's injected version
#     (the only legitimately-changed byte when a release binary audits the
#     vanilla artifact),
#   - run verify-s07.sh with BIN=$EXTRACTED_BIN.
# trap restores both on exit.
if [[ ! -x "$REPO_ROOT/afcli" ]]; then
    if CGO_ENABLED=0 go build -ldflags='-s -w' -o "$REPO_ROOT/afcli" ./cmd/afcli >"$TMPDIR_S01M2/local-build.out" 2>&1; then
        BUILT_LOCAL_AFCLI=1
    else
        fail m001-contract-still-holds "local stripped build failed; tail:
$(tail -n 20 "$TMPDIR_S01M2/local-build.out")"
        echo
        if (( fails > 0 )); then
            echo "verify-s01-m002: $fails failed, $passes passed"
            exit 1
        fi
        echo "verify-s01-m002: $passes checks passed"
        exit 0
    fi
fi

cp "$GOLDEN_PATH" "$GOLDEN_BACKUP"
# Rewrite via sed, not jq: jq normalizes JSON output and would de-escape
# & → & in non-ASCII-only fields, breaking byte-identity. sed mutates
# only the afcli_version line and leaves the rest of the bytes untouched.
# SNAPSHOT_VERSION is a release identifier (alnum + . + -), no sed-meta-chars.
sed "s|\"afcli_version\": \"[^\"]*\"|\"afcli_version\": \"${SNAPSHOT_VERSION}\"|" \
    "$GOLDEN_BACKUP" >"$GOLDEN_PATH"

if BIN="$EXTRACTED_BIN" bash "$REPO_ROOT/scripts/verify-s07.sh" \
       >"$TMPDIR_S01M2/verify-s07.out" 2>"$TMPDIR_S01M2/verify-s07.err"; then
    pass m001-contract-still-holds
else
    fail m001-contract-still-holds "verify-s07.sh failed against snapshot binary; output:
$(cat "$TMPDIR_S01M2/verify-s07.out")
$(cat "$TMPDIR_S01M2/verify-s07.err")"
fi

echo
if (( fails > 0 )); then
    echo "verify-s01-m002: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s01-m002: $passes checks passed"
