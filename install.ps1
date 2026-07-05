# api2convert installer for Windows PowerShell.
#
#   irm https://raw.githubusercontent.com/QaamGo/api2convert-cli/main/install.ps1 | iex
#
# Pin a version with:  $env:API2CONVERT_VERSION = "v1.2.3"; irm ... | iex
$ErrorActionPreference = "Stop"

$repo = "QaamGo/api2convert-cli"
$bin  = "api2convert"

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}

$tag = $env:API2CONVERT_VERSION
if (-not $tag) {
    Write-Host "> Resolving the latest release..."
    $rel = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
    $tag = $rel.tag_name
}
$ver = $tag.TrimStart("v")

$asset = "${bin}_${ver}_windows_${arch}.zip"
$base  = "https://github.com/$repo/releases/download/$tag"
$tmp   = Join-Path $env:TEMP ("a2c-" + [System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp | Out-Null

Write-Host "> Downloading $asset"
Invoke-WebRequest "$base/$asset" -OutFile "$tmp\$asset"
Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt"

Write-Host "> Verifying checksum"
$want = (Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match [regex]::Escape($asset) + "$" }) -split '\s+' | Select-Object -First 1
$got  = (Get-FileHash "$tmp\$asset" -Algorithm SHA256).Hash.ToLower()
if ($want -and ($got -ne $want.ToLower())) { throw "checksum verification failed" }

Expand-Archive -Path "$tmp\$asset" -DestinationPath $tmp -Force

$dest = Join-Path $env:LOCALAPPDATA "Programs\api2convert"
New-Item -ItemType Directory -Path $dest -Force | Out-Null
Copy-Item "$tmp\$bin.exe" (Join-Path $dest "$bin.exe") -Force
Remove-Item $tmp -Recurse -Force

# Add to the user PATH if missing.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dest*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$dest", "User")
    Write-Host "Added $dest to your PATH (restart your terminal to use it)."
}

Write-Host ""
Write-Host "Installed api2convert to $dest"
Write-Host "Next: api2convert login"
