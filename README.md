# api2convert

[![CI](https://github.com/QaamGo/api2convert-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/QaamGo/api2convert-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/QaamGo/api2convert-cli)](https://github.com/QaamGo/api2convert-cli/releases)
![License](https://img.shields.io/badge/license-MIT-green)

Convert, compress and transform files from the command line — a single,
self-contained binary for the [api2convert](https://www.api2convert.com) API. No
Node, Python or Java runtime to install.

![api2convert CLI demo — signing in, converting a single file, and batch-converting a folder](demo/demo.gif)

- **One download, run it** — static binaries for Windows, macOS and Linux.
- **Guided wizard** for non-technical users, plus a full flag interface for scripts.
- **Automation-ready** — batch, recursive, folder watch, `--json`, stable exit codes.
- **Everything the API does** — convert, compress, merge, OCR, jobs, async + webhooks.

## Install

### Windows

**Download and run — no installer, no runtime.**

1. Grab **`api2convert_<version>_windows_amd64.zip`** from the
   [Releases](https://github.com/QaamGo/api2convert-cli/releases) page
   (use the `arm64` zip on ARM PCs).
2. Unzip it and run **`api2convert.exe`** — double-click for the guided wizard
   (it walks you through signing in on first run), or call it from PowerShell / CMD.

Prefer a one-liner that also adds `api2convert` to your `PATH`? Run in PowerShell:

```powershell
irm https://raw.githubusercontent.com/QaamGo/api2convert-cli/main/install.ps1 | iex
```

<sub>Also on [Scoop](https://scoop.sh): `scoop bucket add qaamgo https://github.com/QaamGo/scoop-bucket && scoop install api2convert`</sub>

### macOS (Homebrew)

```sh
brew install qaamgo/tap/api2convert
```

### Linux

```sh
curl -fsSL https://raw.githubusercontent.com/QaamGo/api2convert-cli/main/install.sh | sh
```

Prefer native packages? Download the `.deb` / `.rpm` from the
[Releases](https://github.com/QaamGo/api2convert-cli/releases) page:

```sh
sudo dpkg -i api2convert_<version>_linux_amd64.deb   # Debian/Ubuntu
sudo rpm -i  api2convert_<version>_linux_amd64.rpm   # Fedora/RHEL
```

<sub>Homebrew works on Linux too: `brew install qaamgo/tap/api2convert`</sub>

## Quick start

```sh
api2convert login                          # save your API key (or set API2CONVERT_API_KEY)
api2convert convert report.docx --to pdf   # → report.pdf
```

Run `api2convert` with no arguments in a terminal to launch the interactive wizard.

## Examples

```sh
# Convert from a URL or stdin
api2convert convert https://example-files.online-convert.com/raster%20image/jpg/example_small.jpg --to pdf -o out/
cat scan.tiff | api2convert convert - --to pdf -o scan.pdf

# Bulk + recursive, with per-format options (subfolders are preserved under --out-dir)
api2convert batch ./images --to webp --option quality=80 --out-dir web/ --recursive

# Force a value to stay a literal string (key:=value skips number/bool coercion)
api2convert convert in.png --to png --option zip_password:=080

# High-level verbs
api2convert compress big-scan.pdf
api2convert merge a.pdf b.pdf c.pdf --to pdf -o combined.pdf
api2convert thumbnail photo.png --option width=320
api2convert resize banner.png --option width=1200

# Watch a folder and convert anything dropped into it
api2convert watch ./inbox --to pdf --out-dir ./done

# Discover what's possible
api2convert formats search word
api2convert options pdf

# Automation
api2convert convert movie.mov --to mp4 --async        # prints a job id
api2convert jobs wait <job-id>
api2convert convert report.docx --to pdf --json --quiet
api2convert credits
```

## Configuration

Settings resolve in the order **flag → environment → config file → default**.

- API key: `--api-key`, `API2CONVERT_API_KEY`, or `api2convert login`.
- Config file: `api2convert config path` (0600; `~/.config/api2convert/config.toml`
  on Linux, `%AppData%\api2convert` on Windows, `~/Library/Application Support`
  on macOS).

## Updating

Upgrade in place, verifying the download's checksum and signature:

```sh
api2convert self-update
```

In an interactive terminal the CLI also checks for a newer release at most **once
every 7 days** and asks whether to update; just press Enter to keep going. Silence
the check with `--no-update-check` or `API2CONVERT_NO_UPDATE_CHECK=1`. Homebrew and
Scoop installs are upgraded through the package manager (`brew upgrade api2convert`
/ `scoop update api2convert`) instead.

## Exit codes

Every command exits with a stable, documented status — handy for scripting. In
`--json` mode the same code is also in the error envelope (`exit_code`).

| Code | Meaning |
|-----:|---------|
| `0` | Success |
| `1` | Generic error |
| `2` | Usage error (unknown/invalid flag or argument) |
| `3` | Authentication — missing or rejected API key |
| `4` | Quota — out of credits, or rate-limited |
| `5` | Validation — bad target/option, or not found |
| `6` | Conversion failed |
| `7` | Timeout — job still running server-side (check `jobs status`) |
| `8` | Network — couldn't reach api2convert |
| `130` | Interrupted (Ctrl-C / SIGTERM) |

## Shell completion

```sh
api2convert completion bash|zsh|fish|powershell
```

`--to`, `--category` and option keys complete from the live formats catalog.

## Why a single binary?

No runtime to install, nothing to keep updated — download one file and run it.
You get an interactive wizard, progress bars, shell completion, async/webhooks and
full job control out of the box, on every platform.

## License

MIT — see [LICENSE](LICENSE).
