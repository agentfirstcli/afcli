#!/usr/bin/env bash
# verify-s03-m002.sh — slice M002/S03 mechanical contract check.
#
# Proves the goreleaser docker pipeline + the R035 in-container contract for
# the host architecture. Per the slice plan the multi-arch manifest list is
# push-only, so this script asserts the *local* surface only:
#
#   1. preflight — goreleaser, jq, and docker CLI on PATH. `docker buildx
#      version` is *probed* (not required); if it errors or the backend is
#      buildah/podman without a buildx plugin AND check 2 then fails to
#      produce an image, we degrade to a single SKIP banner + exit 0. This
#      mirrors release-snapshot.sh's 127-fallback idiom: the dev box runs
#      podman-emulating-docker without buildx, and S04 owns the buildx-on-CI
#      proof.
#   2. snapshot-with-dockers — `goreleaser release --snapshot --clean` exits 0
#      AND at least one local image tag matching agentfirstcli/afcli:*-<arch>
#      appears in `docker images`. If buildx silently skipped the build
#      (snapshot succeeded but no image), trigger the SKIP path.
#   3. image-size-budget — `docker image inspect ... --format {{.Size}}`
#      (bytes) is under 20 * 1024 * 1024.
#   4. container-version-injected — `docker run --rm <tag> --version` output
#      starts with `afcli `, is not `0.0.0-dev`, and we capture the version.
#   5. container-helpschema-version-matches — `--help-schema | jq -r
#      .afcli_version` inside the container equals check 4's version.
#   6. container-audit-envelope-version-matches — running `audit
#      /work/descriptors/valid-skip-relax.yaml --output json` inside the
#      container with testdata mounted produces an envelope whose
#      afcli_version matches check 4's version. The envelope may go to
#      stderr (S01 forward intelligence), so 2>&1 is mandatory.
#   7. entrypoint-is-pinned-path — `docker image inspect ... --format
#      {{join .Config.Entrypoint " "}}` is exactly `/usr/local/bin/afcli`.
#
# Pass: every check prints PASS; exit 0.
# Skip: buildx unavailable AND no image produced — single SKIP banner; exit 0.
# Fail: at least one check prints FAIL on stderr; exit 1.

set -u

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ---- preflight: required CLIs ----
if ! command -v goreleaser >/dev/null 2>&1; then
    echo "verify-s03-m002: goreleaser not installed — required for slice contract" >&2
    exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
    echo "verify-s03-m002: jq not installed — required for envelope inspection" >&2
    exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
    echo "verify-s03-m002: docker CLI not installed — required for slice contract" >&2
    exit 1
fi

# Probe buildx. Real docker buildx prints `github.com/docker/buildx v…`.
# Podman with the docker shim returns `buildah …`. Treat anything that is
# not a real buildx as "not available" so the SKIP path can fire.
BUILDX_OK=0
buildx_probe="$(docker buildx version 2>&1 || true)"
if [[ "$buildx_probe" == *"github.com/docker/buildx"* ]]; then
    BUILDX_OK=1
fi

case "$(uname -m)" in
    x86_64|amd64)   goarch=amd64 ;;
    aarch64|arm64)  goarch=arm64 ;;
    *)
        echo "verify-s03-m002: unsupported host arch: $(uname -m)" >&2
        exit 1
        ;;
esac

TMPDIR_S03M2="$(mktemp -d)"

cleanup() {
    rm -rf "$TMPDIR_S03M2"
}
trap cleanup EXIT

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

skip_exit() {
    echo
    echo "SKIP verify-s03-m002: $1" >&2
    echo "verify-s03-m002: skipped (buildx not available locally; manifest-list assertions deferred to S04)"
    exit 0
}

# ---- Case 1: preflight ----
# Preflight passes when the required CLIs exist (already enforced above).
# Buildx is probed but optional — its absence is signalled by BUILDX_OK=0
# and resolved by check 2's image-presence test, not preflight itself.
pass preflight

# ---- Case 2: snapshot-with-dockers ----
snapshot_status=0
goreleaser release --snapshot --clean >"$TMPDIR_S03M2/snapshot.out" 2>&1 || snapshot_status=$?

# Did goreleaser actually tag a host-arch image?
host_image="$(docker images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null \
    | grep -E "^agentfirstcli/afcli:.*-${goarch}\$" \
    | head -n 1 || true)"

if (( snapshot_status != 0 )); then
    if (( BUILDX_OK == 0 )); then
        skip_exit "buildx not available locally; goreleaser snapshot failed (status=$snapshot_status). Tail:
$(tail -n 20 "$TMPDIR_S03M2/snapshot.out")"
    fi
    fail snapshot-with-dockers "goreleaser snapshot failed (status=$snapshot_status); tail:
$(tail -n 40 "$TMPDIR_S03M2/snapshot.out")"
    echo
    echo "verify-s03-m002: aborting — no snapshot to check" >&2
    echo "verify-s03-m002: $fails failed, $passes passed"
    exit 1
fi

if [[ -z "$host_image" ]]; then
    if (( BUILDX_OK == 0 )); then
        skip_exit "goreleaser snapshot exited 0 but produced no agentfirstcli/afcli:*-${goarch} image — buildx silently skipped without a real plugin"
    fi
    fail snapshot-with-dockers "goreleaser snapshot exited 0 but no agentfirstcli/afcli:*-${goarch} image appeared in 'docker images'; tail:
$(tail -n 40 "$TMPDIR_S03M2/snapshot.out")"
    echo
    echo "verify-s03-m002: aborting — no host-arch image to inspect" >&2
    echo "verify-s03-m002: $fails failed, $passes passed"
    exit 1
fi
pass snapshot-with-dockers
echo "host-image: ${host_image}"

# ---- Case 3: image-size-budget ----
size_bytes="$(docker image inspect "$host_image" --format '{{.Size}}' 2>/dev/null || true)"
if [[ -z "$size_bytes" || ! "$size_bytes" =~ ^[0-9]+$ ]]; then
    fail image-size-budget "could not read image size for $host_image (got '$size_bytes')"
elif (( size_bytes >= 20 * 1024 * 1024 )); then
    fail image-size-budget "image size ${size_bytes} bytes exceeds 20MiB budget (20971520)"
else
    pass image-size-budget
    echo "image-size: ${size_bytes} bytes"
fi

# ---- Case 4: container-version-injected ----
version_line="$(docker run --rm "$host_image" --version 2>/dev/null || true)"
echo "container-version: ${version_line}"
SNAPSHOT_VERSION=""
if [[ "$version_line" != "afcli "* ]]; then
    fail container-version-injected "expected output starting with 'afcli '; got '$version_line'"
elif [[ "$version_line" == *"0.0.0-dev"* ]]; then
    fail container-version-injected "expected non-dev version (snapshot ldflags); got '$version_line'"
else
    # Extract the second whitespace-separated field as the version token.
    # `afcli <version> <commit> <date>` is the layout established in S01.
    SNAPSHOT_VERSION="$(awk '{print $2}' <<<"$version_line")"
    if [[ -z "$SNAPSHOT_VERSION" ]]; then
        fail container-version-injected "could not extract version token from '$version_line'"
    else
        pass container-version-injected
    fi
fi

# ---- Case 5: container-helpschema-version-matches ----
helpschema_version="$(docker run --rm "$host_image" --help-schema 2>/dev/null | jq -r '.afcli_version' 2>/dev/null || true)"
echo "container-helpschema-version: ${helpschema_version}"
if [[ -z "$helpschema_version" || "$helpschema_version" == "null" ]]; then
    fail container-helpschema-version-matches "--help-schema afcli_version empty/null"
elif [[ -z "$SNAPSHOT_VERSION" ]]; then
    fail container-helpschema-version-matches "no SNAPSHOT_VERSION captured from check 4 — cannot compare"
elif [[ "$helpschema_version" != "$SNAPSHOT_VERSION" ]]; then
    fail container-helpschema-version-matches "--help-schema '$helpschema_version' != --version token '$SNAPSHOT_VERSION'"
else
    pass container-helpschema-version-matches
fi

# ---- Case 6: container-audit-envelope-version-matches ----
# Mount testdata read-only so the container can read the descriptor. A YAML
# descriptor is non-executable, so afcli exits 3 with the envelope on stderr;
# 2>&1 harvests it whichever stream goreleaser/distroless lands on.
audit_version="$(docker run --rm \
        -v "$REPO_ROOT/testdata:/work:ro" \
        "$host_image" \
        audit /work/descriptors/valid-skip-relax.yaml --output json 2>&1 \
    | jq -r '.afcli_version' 2>/dev/null || true)"
echo "container-audit-envelope-version: ${audit_version}"
if [[ -z "$audit_version" || "$audit_version" == "null" ]]; then
    fail container-audit-envelope-version-matches "audit afcli_version empty/null"
elif [[ -z "$SNAPSHOT_VERSION" ]]; then
    fail container-audit-envelope-version-matches "no SNAPSHOT_VERSION captured from check 4 — cannot compare"
elif [[ "$audit_version" != "$SNAPSHOT_VERSION" ]]; then
    fail container-audit-envelope-version-matches "audit envelope '$audit_version' != --version token '$SNAPSHOT_VERSION'"
else
    pass container-audit-envelope-version-matches
fi

# ---- Case 7: entrypoint-is-pinned-path ----
entrypoint="$(docker image inspect "$host_image" --format '{{join .Config.Entrypoint " "}}' 2>/dev/null || true)"
echo "container-entrypoint: ${entrypoint}"
if [[ "$entrypoint" != "/usr/local/bin/afcli" ]]; then
    fail entrypoint-is-pinned-path "expected exactly '/usr/local/bin/afcli'; got '$entrypoint'"
else
    pass entrypoint-is-pinned-path
fi

echo
if (( fails > 0 )); then
    echo "verify-s03-m002: $fails failed, $passes passed"
    exit 1
fi
echo "verify-s03-m002: $passes checks passed"
