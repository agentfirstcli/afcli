#!/usr/bin/env bash
# verify-s04-m002.sh — slice M002/S04 mechanical contract check.
#
# Three regimes selected by env vars:
#
#   Regime A — pre-tag checks (always run, no env vars required):
#     1. release.yml-yaml-valid
#     2. release.yml-trigger-on-tags
#     3. release.yml-secrets-referenced
#     4. release.yml-goreleaser-action-pinned
#     5. goreleaser-disable-flipped
#     6. goreleaser-check-clean
#     7. readme-install-rows-present
#   These gate merge of S04 task outputs and run on the dev host pre-tag.
#
#   Regime B — post-tag checks (only when S04_TAG=v0.x.y is set):
#     8. gh-release-published
#     9. tap-formula-points-at-tag
#    10. docker-manifest-multiarch
#   These gate the slice's "shipped" verdict — run after tag-triggered
#   release.yml goes green.
#
#   Regime C — postship-binary smoke (only when BIN=/path/to/afcli is set):
#    11. m001-contract-still-holds-against-installed-binary
#   Replays the M001 11-case contract via scripts/verify-s07.sh against a
#   real installed binary (e.g. brew-installed afcli on macOS arm64). The
#   golden file's afcli_version is sed-patched in place to the binary's
#   reported version, with a trap restoring the original on exit. Concurrent
#   invocations against the same checkout are not supported (golden is
#   shared mutable state during the run window).
#
# Pass: every executed check prints PASS; exit 0.
# Fail: at least one check prints FAIL on stderr; exit 1.
# Warn: a soft-degraded check prints WARN on stderr; counts as pass.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

RELEASE_YML=".github/workflows/release.yml"
GORELEASER_YAML=".goreleaser.yaml"
README_PATH="README.md"
GOLDEN_PATH="$REPO_ROOT/testdata/golden-self-audit.json"

TMPDIR_S04M2="$(mktemp -d)"
GOLDEN_BACKUP="$TMPDIR_S04M2/golden-self-audit.json.orig"
GOLDEN_PATCHED=0

cleanup() {
    if (( GOLDEN_PATCHED )) && [[ -f "$GOLDEN_BACKUP" ]]; then
        cp "$GOLDEN_BACKUP" "$GOLDEN_PATH"
    fi
    rm -rf "$TMPDIR_S04M2"
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

skip_exit() {
    echo
    echo "SKIP verify-s04-m002: $1" >&2
    exit 0
}

# ----------------------------------------------------------------------
# Regime banner
# ----------------------------------------------------------------------
echo "verify-s04-m002: Regime A: pre-tag (always)"
if [[ -n "${S04_TAG:-}" ]]; then
    echo "verify-s04-m002: Regime B: post-tag (S04_TAG=${S04_TAG})"
fi
if [[ -n "${BIN:-}" ]]; then
    echo "verify-s04-m002: Regime C: postship-binary (BIN=${BIN})"
fi
echo

# ======================================================================
# Regime A — pre-tag checks
# ======================================================================

# ---- Case 1: release.yml-yaml-valid ----
if [[ ! -f "$RELEASE_YML" ]]; then
    fail release.yml-yaml-valid "missing $RELEASE_YML"
elif command -v python3 >/dev/null 2>&1; then
    if python3 -c "import yaml; yaml.safe_load(open('$RELEASE_YML'))" \
            >"$TMPDIR_S04M2/yaml.out" 2>&1; then
        pass release.yml-yaml-valid
    else
        # python3 present but PyYAML may be missing — degrade to a key-string
        # grep so the check still detects gross corruption.
        if grep -q "ImportError\|ModuleNotFoundError" "$TMPDIR_S04M2/yaml.out"; then
            if grep -qE '^name:\s*release' "$RELEASE_YML" \
               && grep -qE '^on:' "$RELEASE_YML" \
               && grep -qE '^jobs:' "$RELEASE_YML"; then
                warn release.yml-yaml-valid "PyYAML not installed; fell back to key-string grep (name/on/jobs all present)"
            else
                fail release.yml-yaml-valid "PyYAML missing AND key-string fallback found no name/on/jobs structure"
            fi
        else
            fail release.yml-yaml-valid "python3 yaml.safe_load failed: $(cat "$TMPDIR_S04M2/yaml.out")"
        fi
    fi
else
    if grep -qE '^name:\s*release' "$RELEASE_YML" \
       && grep -qE '^on:' "$RELEASE_YML" \
       && grep -qE '^jobs:' "$RELEASE_YML"; then
        warn release.yml-yaml-valid "python3 not installed; fell back to key-string grep (name/on/jobs all present)"
    else
        fail release.yml-yaml-valid "python3 missing AND key-string fallback found no name/on/jobs structure"
    fi
fi

# ---- Case 2: release.yml-trigger-on-tags ----
# Match `tags: ['v*']` or `tags: [v*]` — quotes optional, asterisk literal.
if [[ ! -f "$RELEASE_YML" ]]; then
    fail release.yml-trigger-on-tags "missing $RELEASE_YML"
elif grep -qE "tags:\s*\['?v\*'?\]" "$RELEASE_YML"; then
    pass release.yml-trigger-on-tags
else
    fail release.yml-trigger-on-tags "no tags: ['v*'] / [v*] entry found in $RELEASE_YML"
fi

# ---- Case 3: release.yml-secrets-referenced ----
if [[ ! -f "$RELEASE_YML" ]]; then
    fail release.yml-secrets-referenced "missing $RELEASE_YML"
else
    missing=()
    for secret in HOMEBREW_TAP_TOKEN DOCKER_USERNAME DOCKER_TOKEN; do
        if ! grep -q "$secret" "$RELEASE_YML"; then
            missing+=("$secret")
        fi
    done
    if (( ${#missing[@]} == 0 )); then
        pass release.yml-secrets-referenced
    else
        fail release.yml-secrets-referenced "missing secret reference(s): ${missing[*]}"
    fi
fi

# ---- Case 4: release.yml-goreleaser-action-pinned ----
if [[ ! -f "$RELEASE_YML" ]]; then
    fail release.yml-goreleaser-action-pinned "missing $RELEASE_YML"
elif ! grep -qE 'goreleaser/goreleaser-action@v[0-9]+' "$RELEASE_YML"; then
    fail release.yml-goreleaser-action-pinned "no goreleaser/goreleaser-action@v<digit> reference found"
elif ! grep -qE '^[[:space:]]*version:[[:space:]]*"?[^"]*[0-9]' "$RELEASE_YML"; then
    fail release.yml-goreleaser-action-pinned "goreleaser-action present but no 'version:' line containing a digit (toolchain unpinned)"
else
    pass release.yml-goreleaser-action-pinned
fi

# ---- Case 5: goreleaser-disable-flipped ----
# `release: { disable: true }` is what blocks publish. Use python (or python
# fallback) to parse the YAML rather than a literal substring grep, because
# `disable: true` legitimately appears under `changelog:` and possibly other
# keys; substring-grep would falsely fail after this slice ships.
if [[ ! -f "$GORELEASER_YAML" ]]; then
    fail goreleaser-disable-flipped "missing $GORELEASER_YAML"
elif command -v python3 >/dev/null 2>&1; then
    py_out="$(python3 - "$GORELEASER_YAML" <<'PY' 2>&1
import sys
try:
    import yaml
except ImportError:
    print("NO_YAML")
    sys.exit(0)
with open(sys.argv[1]) as f:
    data = yaml.safe_load(f) or {}
release = data.get("release", {}) or {}
if release.get("disable") is True:
    print("DISABLED")
else:
    print("ENABLED")
PY
)"
    case "$py_out" in
        ENABLED)
            pass goreleaser-disable-flipped
            ;;
        DISABLED)
            fail goreleaser-disable-flipped "release.disable is still true in $GORELEASER_YAML"
            ;;
        NO_YAML)
            # PyYAML missing — fall back to a multiline-aware awk pass that
            # walks top-level blocks and checks the `release:` block only.
            awk_out="$(awk '
                BEGIN { in_release = 0; disabled = 0 }
                /^[a-zA-Z_]+:/ { in_release = 0 }
                /^release:/ { in_release = 1; next }
                in_release && /^[[:space:]]+disable:[[:space:]]*true/ { disabled = 1 }
                END {
                    if (disabled) print "DISABLED"
                    else print "ENABLED"
                }
            ' "$GORELEASER_YAML")"
            if [[ "$awk_out" == "ENABLED" ]]; then
                warn goreleaser-disable-flipped "PyYAML not installed; awk fallback says release.disable is not true"
            else
                fail goreleaser-disable-flipped "release.disable still true (awk fallback)"
            fi
            ;;
        *)
            fail goreleaser-disable-flipped "python3 yaml probe error: $py_out"
            ;;
    esac
else
    awk_out="$(awk '
        BEGIN { in_release = 0; disabled = 0 }
        /^[a-zA-Z_]+:/ { in_release = 0 }
        /^release:/ { in_release = 1; next }
        in_release && /^[[:space:]]+disable:[[:space:]]*true/ { disabled = 1 }
        END {
            if (disabled) print "DISABLED"
            else print "ENABLED"
        }
    ' "$GORELEASER_YAML")"
    if [[ "$awk_out" == "ENABLED" ]]; then
        warn goreleaser-disable-flipped "python3 missing; awk fallback says release.disable is not true"
    else
        fail goreleaser-disable-flipped "release.disable still true (awk fallback)"
    fi
fi

# ---- Case 6: goreleaser-check-clean ----
if ! command -v goreleaser >/dev/null 2>&1; then
    warn goreleaser-check-clean "goreleaser not installed — skipping config validation"
else
    if goreleaser check >"$TMPDIR_S04M2/goreleaser-check.out" 2>&1; then
        pass goreleaser-check-clean
    else
        fail goreleaser-check-clean "goreleaser check failed; tail:
$(tail -n 40 "$TMPDIR_S04M2/goreleaser-check.out")"
    fi
fi

# ---- Case 7: readme-install-rows-present ----
if [[ ! -f "$README_PATH" ]]; then
    fail readme-install-rows-present "missing $README_PATH"
else
    miss_brew=0
    miss_docker=0
    grep -qF 'brew install agentfirstcli/afcli/afcli' "$README_PATH" || miss_brew=1
    grep -qF 'docker run --rm agentfirstcli/afcli:' "$README_PATH"   || miss_docker=1
    if (( miss_brew == 0 && miss_docker == 0 )); then
        pass readme-install-rows-present
    else
        details=()
        (( miss_brew ))   && details+=("brew install row missing")
        (( miss_docker )) && details+=("docker run row missing")
        fail readme-install-rows-present "${details[*]}"
    fi
fi

# ======================================================================
# Regime B — post-tag checks (S04_TAG)
# ======================================================================
if [[ -n "${S04_TAG:-}" ]]; then
    TAG="$S04_TAG"
    TAG_NO_V="${TAG#v}"

    if ! command -v gh >/dev/null 2>&1; then
        warn gh-release-published "gh CLI not installed — Regime B checks 8/9 skipped"
        warn tap-formula-points-at-tag "gh CLI not installed"
    else
        # ---- Case 8: gh-release-published ----
        if gh release view "$TAG" -R agentfirstcli/afcli \
                --json tagName,assets >"$TMPDIR_S04M2/release.json" 2>"$TMPDIR_S04M2/release.err"; then
            if command -v jq >/dev/null 2>&1; then
                asset_count="$(jq '[.assets[].name | select(test("\\.tar\\.gz$"))] | length' "$TMPDIR_S04M2/release.json" 2>/dev/null || echo 0)"
                has_checksums="$(jq '[.assets[].name | select(. == "checksums.txt")] | length' "$TMPDIR_S04M2/release.json" 2>/dev/null || echo 0)"
                if [[ "$asset_count" -ge 4 && "$has_checksums" -ge 1 ]]; then
                    pass gh-release-published
                    echo "gh-release-assets: ${asset_count} archives, checksums.txt present"
                else
                    fail gh-release-published "expected >=4 *.tar.gz + checksums.txt; got archives=${asset_count}, checksums.txt=${has_checksums}"
                fi
            else
                warn gh-release-published "release exists but jq missing — cannot verify asset list shape"
            fi
        else
            fail gh-release-published "gh release view $TAG failed: $(cat "$TMPDIR_S04M2/release.err")"
        fi

        # ---- Case 9: tap-formula-points-at-tag ----
        if gh api repos/agentfirstcli/homebrew-afcli/contents/Formula/afcli.rb \
                --jq .content >"$TMPDIR_S04M2/formula.b64" 2>"$TMPDIR_S04M2/formula.err"; then
            if base64 -d <"$TMPDIR_S04M2/formula.b64" >"$TMPDIR_S04M2/afcli.rb" 2>/dev/null; then
                if grep -qF "$TAG_NO_V" "$TMPDIR_S04M2/afcli.rb"; then
                    pass tap-formula-points-at-tag
                else
                    fail tap-formula-points-at-tag "Formula/afcli.rb does not reference '$TAG_NO_V'"
                fi
            else
                fail tap-formula-points-at-tag "could not base64-decode tap formula response"
            fi
        else
            fail tap-formula-points-at-tag "gh api tap fetch failed: $(cat "$TMPDIR_S04M2/formula.err")"
        fi
    fi

    # ---- Case 10: docker-manifest-multiarch ----
    if ! command -v docker >/dev/null 2>&1; then
        warn docker-manifest-multiarch "docker CLI not installed — Regime B check 10 skipped"
    else
        if docker manifest inspect "agentfirstcli/afcli:${TAG_NO_V}" \
                >"$TMPDIR_S04M2/manifest.json" 2>"$TMPDIR_S04M2/manifest.err"; then
            has_amd64=0
            has_arm64=0
            grep -q '"architecture": "amd64"' "$TMPDIR_S04M2/manifest.json" && has_amd64=1
            grep -q '"architecture": "arm64"' "$TMPDIR_S04M2/manifest.json" && has_arm64=1
            if (( has_amd64 && has_arm64 )); then
                pass docker-manifest-multiarch
            else
                fail docker-manifest-multiarch "manifest missing arch entries: amd64=${has_amd64} arm64=${has_arm64}"
            fi
        else
            fail docker-manifest-multiarch "docker manifest inspect failed: $(cat "$TMPDIR_S04M2/manifest.err")"
        fi
    fi
fi

# ======================================================================
# Regime C — postship-binary smoke (BIN)
# ======================================================================
if [[ -n "${BIN:-}" ]]; then
    # ---- Case 11: m001-contract-still-holds-against-installed-binary ----
    if [[ ! -x "$BIN" ]]; then
        fail m001-contract-still-holds-against-installed-binary "BIN='$BIN' is not an executable file"
    else
        bin_version_line="$("$BIN" --version 2>/dev/null || true)"
        BIN_VERSION="$(awk '{print $2}' <<<"$bin_version_line")"
        if [[ -z "$BIN_VERSION" ]]; then
            fail m001-contract-still-holds-against-installed-binary "could not extract version from '$bin_version_line'"
        elif [[ ! -f "$GOLDEN_PATH" ]]; then
            fail m001-contract-still-holds-against-installed-binary "missing golden file at $GOLDEN_PATH"
        else
            cp "$GOLDEN_PATH" "$GOLDEN_BACKUP"
            GOLDEN_PATCHED=1
            # BSD-compatible in-place sed: write to .bak then remove. Works on
            # macOS without GNU sed. BIN_VERSION is a release identifier
            # (alnum + . + -) — no sed-meta-chars expected.
            sed -i.bak "s|\"afcli_version\": \"[^\"]*\"|\"afcli_version\": \"${BIN_VERSION}\"|" \
                "$GOLDEN_PATH"
            rm -f "${GOLDEN_PATH}.bak"

            if BIN="$BIN" bash "$REPO_ROOT/scripts/verify-s07.sh" \
                    >"$TMPDIR_S04M2/verify-s07.out" 2>"$TMPDIR_S04M2/verify-s07.err"; then
                pass m001-contract-still-holds-against-installed-binary
            else
                fail m001-contract-still-holds-against-installed-binary "verify-s07.sh failed against $BIN; output:
$(cat "$TMPDIR_S04M2/verify-s07.out")
$(cat "$TMPDIR_S04M2/verify-s07.err")"
            fi
        fi
    fi
fi

echo
if (( fails > 0 )); then
    echo "verify-s04-m002: $fails failed, $passes passed, $warns warned"
    exit 1
fi
echo "verify-s04-m002: $passes checks passed, $warns warned"
