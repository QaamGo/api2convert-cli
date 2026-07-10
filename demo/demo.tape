# VHS tape — records demo/demo.gif for the README.
#
# It drives the REAL api2convert binary against a local mock API
# (demo/mock_api.py) so the recording shows genuine CLI behaviour — masked
# login, live spinner, real output formatting — with no API key and no quota.
#
# Regenerate with:  API2CONVERT_BIN=<linux-amd64 binary> demo/generate.sh

Output demo.gif

Set Shell bash
Set FontSize 17
Set Width 1180
Set Height 400
Set Padding 24
Set Theme "Catppuccin Mocha"
Set TypingSpeed 55ms

# ── Hidden setup: start the mock API, point the CLI at it, make demo files ──
Hide
Type "export PS1='$ '" Enter
Type "export API2CONVERT_BASE_URL=http://127.0.0.1:8080 HOME=/demo" Enter
Type "mkdir -p /demo && cd /demo" Enter
Type "bash /demo-src/setup.sh /demo" Enter
Type "clear" Enter
Show

# ── 1. Sign in (the API key is masked as you type) ──
Type "api2convert login" Sleep 400ms Enter
Sleep 1.2s
Type "demo-key-DO-NOT-USE-abc123" Sleep 500ms Enter
Sleep 2s

# ── 2. Convert a single file ──
Type "api2convert convert report.docx --to pdf" Sleep 300ms Enter
Sleep 3s

# ── 3. Batch-convert a whole folder ──
Type "api2convert batch ./images --to webp --out-dir web/ --recursive" Sleep 300ms Enter
Sleep 3.5s

Sleep 2s
