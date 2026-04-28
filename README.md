# afcli

`afcli` audits a CLI against the [Agent-First CLI](https://agentfirstcli.github.io) manifest — 16 principles spanning output, errors & exit, behavior, contracts & stability, discoverability, and safety. It produces a structured report mapping each principle to `pass | fail | skip | review`, with severity, evidence, and a concrete recommendation.

JSON is the default output. Agents and CI are first-class consumers; `--output text|markdown` renders the same data for humans.

> Status: pre-1.0 (`0.0.0-dev`), manifest `0.0.1`. M001/S06 complete — every audit emits all 16 findings. M001/S07 (CI keystone, self-audit, performance budget) is in progress.

## Install

### Homebrew (macOS)

```sh
brew install agentfirstcli/afcli/afcli
```

### Docker (Linux, CI)

```sh
docker run --rm agentfirstcli/afcli:0.1.0 audit /target
```

### From source (Go 1.23+)

```sh
go install github.com/agentfirstcli/afcli/cmd/afcli@latest
```

## Quickstart

Black-box audit (the default — uses `--help` introspection and static checks only):

```sh
afcli audit /usr/bin/git
afcli audit /usr/bin/git --output text
afcli audit /usr/bin/git --output markdown > git-audit.md
```

Scaffold a descriptor for self-attested mode:

```sh
afcli init /usr/bin/git --out git.afcli.yaml
```

Audit with a descriptor (skip principles, relax severities, authorize probes):

```sh
afcli audit /usr/bin/git --descriptor git.afcli.yaml
```

Behavioral probing requires both `--probe` and explicit authorization in `commands.safe[]`. Destructive commands declared in `commands.destructive[]` are never invoked — only their declared presence is audited.

```sh
afcli audit /usr/bin/git --descriptor git.afcli.yaml --probe
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Audit ran; no findings at or above `--fail-on` |
| 1 | Audit ran; findings at or above `--fail-on` (default `high`) |
| 2 | Usage error (bad flag, bad args) |
| 3 | Could not audit (target missing, descriptor invalid, probe denied) |
| 4 | Internal error |
| 130 | Interrupted (SIGINT/SIGTERM); a partial 16-finding report is still emitted |

`--fail-on=low|medium|high|critical|never` controls the threshold. `never` keeps exit 0 regardless of findings.

## Discoverability

`afcli` is itself agent-first: every command supports `--help-schema`, which emits a machine-parseable JSON descriptor of the command tree, flags, exit codes, and stable error codes. The schema is validated against `testdata/help-schema.schema.json` and is byte-identical across invocations.

```sh
afcli --help-schema
afcli audit --help-schema
```

`--deterministic` (or `AFCLI_DETERMINISTIC=1`) zeroes timestamps and durations, normalizes paths, and stabilizes ordering — useful for golden tests and snapshot diffs.

## Descriptor

`afcli.yaml` (loaded via `--descriptor <path>`):

```yaml
format_version: 1
target: "/usr/bin/git"
commands:
  safe: []         # argvs afcli is authorized to invoke under --probe
  destructive: []  # presence-only; afcli will never invoke these
env: {}
skip_principles: []                # e.g. ["P11"] for principles that do not apply
relax_principles: {}               # e.g. {P15: low} to cap severity
```

The parser is strict: unknown keys reject with an envelope carrying `details.line` and `details.key`. `skip_principles` short-circuits to a synthetic skip finding before the registered check runs. `relax_principles` is a severity ceiling.

## Output contract

The wire schema is frozen at `testdata/report.schema.json` (strict, `additionalProperties: false`). A typical report:

```json
{
  "manifest_version": "0.0.1",
  "afcli_version": "0.0.0-dev",
  "target": "/usr/bin/git",
  "duration_ms": 4,
  "findings": [
    {
      "principle_id": "P15",
      "title": "Machine-Readable Help",
      "category": "Discoverability",
      "severity": "high",
      "status": "review",
      "kind": "requires-review",
      "evidence": "no machine-readable help affordance found in --help",
      "recommendation": "expose --help-schema or --output json for machine consumption",
      "hint": "https://agentfirstcli.github.io/principles/machine-readable-help/"
    }
  ]
}
```

Every report contains exactly 16 findings — one per principle — even on cancellation or probe failure. Adding a wire field is a deliberate `manifest_version` bump.

## Development

```sh
go vet ./...
go test ./...
./scripts/verify-s01.sh   # protocol shape from each milestone slice
./scripts/verify-s06.sh   # current head: 16-finding invariant + init contract
```

Tests pin three contract surfaces hard: `TestBuildHelpSchemaRoot`, `TestHelpSchemaErrorCodesExactSet`, `TestDefaultEngineRegistersAllRealChecks`. They are intentional fail-loud guards — update them in lockstep with any contract change.

## License

Apache License 2.0 — see [LICENSE](LICENSE). Copyright 2026 Wladi Mitzel.
