# Demo GIF

`demo.gif` (shown in the top-level README) is recorded with
[VHS](https://github.com/charmbracelet/vhs). It drives the **real** `api2convert`
binary against a tiny **local mock API** (`mock_api.py`), so the recording shows
genuine CLI behaviour — masked login, the live spinner, real output formatting —
with **no API key and no quota used**.

## Files

| File | Purpose |
|------|---------|
| `demo.tape`    | VHS script: the commands, timing, theme and dimensions. |
| `mock_api.py`  | stdlib HTTP server mimicking the api2convert v2 endpoints the SDK calls (`/contracts`, `/jobs`, `/upload-file/{id}`, poll, download). |
| `setup.sh`     | Hidden pre-roll: starts the mock, waits for it, creates the demo inputs. |
| `make-files.sh`| Creates the stand-in inputs (`report.docx`, `images/*`). |
| `generate.sh`  | Runs VHS in Docker against a supplied binary. |

## Regenerate

```sh
# 1. Build a linux/amd64 binary
docker run --rm -v "$PWD":/src -w /src golang:1.25 go build -o /src/api2convert .

# 2. Record the GIF
API2CONVERT_BIN="$PWD/api2convert" demo/generate.sh
```

The GIF is written to `demo/demo.gif`. Tweak the theme, size, typing speed or the
scripted commands in `demo.tape`.
