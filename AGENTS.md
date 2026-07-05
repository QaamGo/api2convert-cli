# AGENTS.md — api2convert-cli

Official command-line tool for the api2convert API. Go, single self-contained
binary, wraps the `github.com/QaamGo/api2convert-go` SDK.

## Layout

- `main.go`, `version.go` — entrypoint + injected build metadata (`var version`).
- `internal/cli/` — cobra command tree (one file per command group).
- `internal/run/` — conversion orchestration: `ConvertOne`, `Batch`/`BatchItems`,
  `Merge`, `Watch`, output-path resolution and conflict policy.
- `internal/config/` — TOML config, `os.UserConfigDir`, precedence flag>env>file.
- `internal/client/` — the single `api2convert.New(...)` construction.
- `internal/clierr/` — SDK-error → friendly message + stable exit code + `--json` envelope.
- `internal/catalog/` — cached conversions catalog (targets/categories/option schemas).
- `internal/tasks/` — high-level verb registry (compress/merge/ocr/…).
- `internal/ui/`, `internal/output/` — TTY/spinner/prompts and human/JSON rendering.
- `internal/selfupdate/` — checksum-verified GitHub-release self-replace.

## Build & test

The toolchain is **Go 1.25+** (a current dependency requires it). Locally the
SDK is a sibling checkout, wired via a `replace` in `go.mod`:

```
replace github.com/QaamGo/api2convert-go => ../api2convert-go
```

With Go installed:

```sh
go build ./... && go test ./... && go vet ./...
```

Without a local Go toolchain, build in Docker with the workspace mounted so the
`replace` resolves:

```sh
docker run --rm -v <workspace>:/work -w /work/api2convert-cli \
  -e GOFLAGS=-buildvcs=false golang:1.25-bookworm \
  sh -c 'go build ./... && go test ./...'
```

Tests are offline/hermetic: they inject a fake `api2convert.HttpSender` (see
`internal/run/e2e_test.go`) — no API key needed.

## Releasing

- Version lives in `version.go` (`var version`). Bump it, tag `vX.Y.Z`.
- CI (`.github/workflows/release.yml`) asserts the tag equals `version.go` **and
  that no `replace` directive is present**, then runs GoReleaser.
- **Prerequisite:** for CI/release the SDK module must be resolvable (published /
  tagged, or vendored / a `go.work`), because the `replace` is stripped. Local
  development keeps the `replace`.
- Cross-repo publishing to `QaamGo/homebrew-tap` and `QaamGo/scoop-bucket` needs
  a `TAP_GITHUB_TOKEN` secret (contents:write on those repos).
- Builds are currently **unsigned**. To enable macOS notarization (Apple Developer
  ID) and Windows Authenticode/Azure Trusted Signing, follow [SIGNING.md](SIGNING.md)
  — it lists the required secrets, the GoReleaser hooks, and the cleanups (drop the
  cask quarantine hook, remove the SmartScreen FAQ, add winget).

## Conventions

- Never print or log the API key. Diagnostics/spinners go to **stderr**; stdout
  carries only results (path or `--json`).
- Exit codes are stable (see `internal/clierr`). New SDK error types get a case there.
- High-level verbs are thin presets over the convert pipeline, grounded in the
  live catalog: `compress` (same-format), `merge` (target `pdf` + `merge:true`),
  `thumbnail` and `resize` (`operation/*` targets, catalog-gated via `CapTarget`).
  There is no `ocr`/`capture`/`watermark` operation in the api2convert catalog,
  so those verbs are intentionally absent.
