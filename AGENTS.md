# AGENTS.md — api2convert-cli

Official command-line tool for the api2convert API. Go, single self-contained
binary, wraps the `github.com/QaamGo/api2convert-go/v10` SDK.

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

The toolchain is **Go 1.25+** (a current dependency requires it). `go.mod` pins
the **published** SDK (`github.com/QaamGo/api2convert-go/v10`), so a plain build
just works:

```sh
go build ./... && go test ./... && go vet ./...
```

For co-development against a local SDK checkout, use a **gitignored `go.work`**
(never a `replace` in `go.mod` — that must never ship in a release):

```
// go.work  (gitignored)
go 1.25.0
use (
	.
	../api2convert-go
)
```

Without a local Go toolchain, build in Docker (set `GOWORK=off` so a stray
`go.work` can't drag in the sibling; `--network host` lets the module proxy
resolve the published SDK):

```sh
docker run --rm --network host -v "$PWD":/work -w /work \
  -e GOWORK=off -e GOFLAGS=-buildvcs=false golang:1.25 \
  sh -c 'go build ./... && go vet ./... && go test ./...'
```

Tests come in two tiers, mirroring the sibling SDKs:

- **Hermetic (default).** `go test ./...` injects a fake `api2convert.HttpSender`
  (see `internal/run/e2e_test.go`) — offline, no API key needed.
- **Live conformance.** `live/conformance_test.go` (build tag `live`) builds the
  real binary and drives it end-to-end against the live API — one test per
  documented example guide, plus negatives that pin the stable exit-code
  contract. It consumes quota and is gated twice: the `live` build tag keeps it
  out of the default run, and each test skips unless `API2CONVERT_API_KEY` is
  set. `API2CONVERT_BASE_URL` optionally targets a non-prod environment.

  ```sh
  API2CONVERT_API_KEY=<key> go test -tags live -timeout 600s ./live/...
  ```

  CI runs it on the default branch / manual dispatch (`.github/workflows/ci.yml`,
  `live` job). Never commit a real key — it is read only from the environment.

## Releasing

- Version lives in `version.go` (`var version`). Bump it, tag `vX.Y.Z`.
- CI (`.github/workflows/release.yml`) asserts the tag equals `version.go` **and
  that no `replace` directive is present**, then runs GoReleaser.
- **Prerequisite:** the SDK module must be published/tagged so `go.mod` resolves
  it. The local co-dev `go.work` is gitignored and never reaches CI/release.
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
