# api2convert

Convert, compress and transform files from the command line — a single,
self-contained binary for the [api2convert](https://www.api2convert.com) API. No
Node, Python or Java runtime to install.

- **One download, run it** — static binaries for Windows, macOS and Linux.
- **Guided wizard** for non-technical users, plus a full flag interface for scripts.
- **Automation-ready** — batch, recursive, folder watch, `--json`, stable exit codes.
- **Everything the API does** — convert, compress, merge, OCR, jobs, async + webhooks.

## Install

### macOS / Linux (Homebrew)

```sh
brew install qaamgo/tap/api2convert
```

### Windows (Scoop)

```powershell
scoop bucket add qaamgo https://github.com/QaamGo/scoop-bucket
scoop install api2convert
```

### Any OS (install script)

```sh
curl -fsSL https://raw.githubusercontent.com/QaamGo/api2convert-cli/main/install.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/QaamGo/api2convert-cli/main/install.ps1 | iex
```

### Linux packages

Download the `.deb` / `.rpm` from the [Releases](https://github.com/QaamGo/api2convert-cli/releases) page:

```sh
sudo dpkg -i api2convert_<version>_linux_amd64.deb   # Debian/Ubuntu
sudo rpm -i  api2convert-<version>.x86_64.rpm         # Fedora/RHEL
```

> **Unsigned builds (for now):** on macOS a browser-downloaded binary may show
> "cannot be opened because the developer cannot be verified" — right-click →
> **Open**, or run `xattr -d com.apple.quarantine ./api2convert`. On Windows,
> SmartScreen may warn — **More info → Run anyway**. Homebrew, Scoop and the
> install scripts avoid this.

## Quick start

```sh
api2convert login                          # save your API key (or set API2CONVERT_API_KEY)
api2convert convert report.docx --to pdf   # → report.pdf
```

Run `api2convert` with no arguments in a terminal to launch the interactive wizard.

## Examples

```sh
# Convert from a URL or stdin
api2convert convert https://example.com/deck.pptx --to pdf -o out/
cat scan.tiff | api2convert convert - --to pdf -o scan.pdf

# Bulk + recursive, with per-format options
api2convert batch ./images --to webp --option quality=80 --out-dir web/ --recursive

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
