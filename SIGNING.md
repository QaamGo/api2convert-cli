# Code signing (macOS & Windows)

**Status: not yet wired.** The current release pipeline ships **unsigned** binaries
(Homebrew/Scoop/the install script sidestep most OS warnings; direct browser
downloads still hit Gatekeeper/SmartScreen). This document is the playbook for
turning on signing once the certificates are provisioned — you have an **Apple
Developer** account and a **Windows signing key**, which are exactly what's needed.

## Why enable it

- **macOS:** removes the "cannot be opened because the developer cannot be
  verified" Gatekeeper block. Also lets us **delete the quarantine-stripping
  `postflight`** from the Homebrew cask (that hook only exists because the binary
  is unsigned).
- **Windows:** reduces/removes the SmartScreen "unrecognized app" prompt and
  **unlocks `winget`** submission. An **EV** cert grants instant SmartScreen
  reputation; a standard **OV** cert (incl. Azure Trusted Signing) builds
  reputation over time.

---

## Secrets checklist (GitHub Actions repository secrets)

**macOS (Developer ID Application + notarization):**

| Secret | What it is |
|---|---|
| `MACOS_SIGN_P12_BASE64` | Developer ID Application cert exported as `.p12`, base64-encoded |
| `MACOS_SIGN_P12_PASSWORD` | password for that `.p12` |
| `MACOS_NOTARY_KEY_BASE64` | App Store Connect API key (`AuthKey_XXXX.p8`), base64 |
| `MACOS_NOTARY_KEY_ID` | the key's Key ID |
| `MACOS_NOTARY_ISSUER_ID` | App Store Connect Issuer ID |
| `APPLE_TEAM_ID` | your 10-char Team ID |

**Windows — pick ONE path:**

*Path A — certificate file (`.pfx`/PKCS#12):*

| Secret | What it is |
|---|---|
| `WINDOWS_PFX_BASE64` | code-signing cert `.pfx`, base64 |
| `WINDOWS_PFX_PASSWORD` | password for the `.pfx` |

*Path B — Azure Trusted Signing (Microsoft's managed signing):*

| Secret | What it is |
|---|---|
| `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` | service principal with the *Trusted Signing Certificate Profile Signer* role |
| `TRUSTED_SIGNING_ENDPOINT` | e.g. `https://eus.codesigning.azure.net` |
| `TRUSTED_SIGNING_ACCOUNT` | your Trusted Signing account name |
| `TRUSTED_SIGNING_CERT_PROFILE` | the certificate profile name |

> **How to tell which Windows path you have:** if you were issued a `.pfx`/`.p12`
> file (or a token from a CA like Sectigo/DigiCert), use **Path A**. If you set it
> up in the Azure portal ("Trusted Signing" / "Azure Code Signing"), use **Path B**.

---

## macOS

Sign each Mach-O binary with the hardened runtime, then notarize. Two ways:

### Option A — cross-platform, no Mac needed (recommended): `quill`

[anchore/quill](https://github.com/anchore/quill) signs **and** notarizes Mach-O
binaries from Linux, so the whole release stays on the existing Linux runner.

Sign the darwin binaries in a GoReleaser **post-build hook** (guarded to darwin,
so it runs before archiving and the archive contains the signed binary):

```yaml
# .goreleaser.yaml  (builds[].hooks)
builds:
  - id: api2convert
    # ...existing build config...
    hooks:
      post:
        - cmd: sh -c 'test "{{ .Os }}" != "darwin" || quill sign-and-notarize "{{ .Path }}"'
          env:
            - QUILL_SIGN_P12={{ .Env.QUILL_SIGN_P12 }}          # path to the .p12
            - QUILL_SIGN_PASSWORD={{ .Env.MACOS_SIGN_P12_PASSWORD }}
            - QUILL_NOTARY_KEY={{ .Env.QUILL_NOTARY_KEY }}       # path to the .p8
            - QUILL_NOTARY_KEY_ID={{ .Env.MACOS_NOTARY_KEY_ID }}
            - QUILL_NOTARY_ISSUER={{ .Env.MACOS_NOTARY_ISSUER_ID }}
          output: true
```

Note: a bare CLI binary can't be *stapled*, so Gatekeeper verifies the
notarization online on first run — fine for a CLI. For fully offline-robust
verification, ship a stapled `.dmg`/`.pkg` (Option B).

### Option B — macOS CI runner (`codesign` + `notarytool` + `stapler`)

Use a `macos-latest` job:

```sh
# import the cert into a temp keychain, then:
codesign --timestamp --options runtime \
  --sign "Developer ID Application: <NAME> (<TEAMID>)" ./api2convert

# notarize (zip the binary or, better, a .dmg/.pkg):
ditto -c -k --keepParent ./api2convert a2c.zip
xcrun notarytool submit a2c.zip \
  --key AuthKey.p8 --key-id "$MACOS_NOTARY_KEY_ID" --issuer "$MACOS_NOTARY_ISSUER_ID" --wait

# for a .dmg/.pkg you can staple (offline-robust):
xcrun stapler staple ./api2convert.dmg
```

GoReleaser Pro also has a native `notarize:` block; OSS uses the hook/steps above.

---

## Windows

Sign `api2convert.exe` **before archiving** (a GoReleaser post-build hook guarded
to windows), so the `.zip` and any installer contain the signed exe.

### Path A — `.pfx` with `osslsigncode` (signs PE from Linux)

```yaml
builds:
  - id: api2convert
    hooks:
      post:
        - cmd: sh -c 'test "{{ .Os }}" != "windows" || osslsigncode sign \
            -pkcs12 "$WINDOWS_PFX" -pass "$WINDOWS_PFX_PASSWORD" \
            -n "api2convert" -i "https://www.api2convert.com" \
            -ts "http://timestamp.digicert.com" \
            -in "{{ .Path }}" -out "{{ .Path }}.signed" && mv "{{ .Path }}.signed" "{{ .Path }}"'
          output: true
```

Always include an RFC-3161 timestamp (`-ts …`) so signatures stay valid after the
cert expires.

### Path B — Azure Trusted Signing

Simplest on GitHub Actions via the official action (run before archiving, or sign
the extracted exe in a dedicated job):

```yaml
- uses: azure/trusted-signing-action@v0
  with:
    azure-tenant-id: ${{ secrets.AZURE_TENANT_ID }}
    azure-client-id: ${{ secrets.AZURE_CLIENT_ID }}
    azure-client-secret: ${{ secrets.AZURE_CLIENT_SECRET }}
    endpoint: ${{ secrets.TRUSTED_SIGNING_ENDPOINT }}
    trusted-signing-account-name: ${{ secrets.TRUSTED_SIGNING_ACCOUNT }}
    certificate-profile-name: ${{ secrets.TRUSTED_SIGNING_CERT_PROFILE }}
    files-folder: dist
    files-folder-filter: exe
```

(Equivalent CLI: `azuresigntool sign -kvu <endpoint> ... -tr http://timestamp.acs.microsoft.com api2convert.exe`.)

---

## release.yml additions (sketch)

Before the GoReleaser step, materialize the secrets to files and put the tools on
PATH:

```yaml
      - name: Prepare signing material
        run: |
          echo "${{ secrets.MACOS_SIGN_P12_BASE64 }}" | base64 -d > /tmp/ds.p12
          echo "${{ secrets.MACOS_NOTARY_KEY_BASE64 }}" | base64 -d > /tmp/notary.p8
          echo "${{ secrets.WINDOWS_PFX_BASE64 }}" | base64 -d > /tmp/win.pfx   # Path A only
      - run: |          # install signing tools
          go install github.com/anchore/quill/cmd/quill@latest
          sudo apt-get update && sudo apt-get install -y osslsigncode   # Path A only
      - uses: goreleaser/goreleaser-action@v6
        with: { version: "~> v2", args: release --clean }
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          TAP_GITHUB_TOKEN: ${{ secrets.TAP_GITHUB_TOKEN }}
          QUILL_SIGN_P12: /tmp/ds.p12
          MACOS_SIGN_P12_PASSWORD: ${{ secrets.MACOS_SIGN_P12_PASSWORD }}
          QUILL_NOTARY_KEY: /tmp/notary.p8
          MACOS_NOTARY_KEY_ID: ${{ secrets.MACOS_NOTARY_KEY_ID }}
          MACOS_NOTARY_ISSUER_ID: ${{ secrets.MACOS_NOTARY_ISSUER_ID }}
          WINDOWS_PFX: /tmp/win.pfx
          WINDOWS_PFX_PASSWORD: ${{ secrets.WINDOWS_PFX_PASSWORD }}
```

---

## Cleanups to make once signing is live

1. **Homebrew cask:** delete the `postflight … xattr -dr com.apple.quarantine`
   hook in `.goreleaser.yaml` — a notarized binary no longer needs it.
2. **README:** remove the "unidentified developer / SmartScreen bypass" FAQ.
3. **winget:** add a GoReleaser `winget:` publisher (now that the exe is signed) to
   open a PR to `microsoft/winget-pkgs`.
4. Optionally ship a signed/notarized `.dmg`/`.pkg` (mac) and a signed `.msi` (win)
   for the click-to-install crowd.

## Verifying a signed build

- macOS: `codesign --verify --verbose ./api2convert` and `spctl -a -vv ./api2convert`
  (should report "accepted / Notarized Developer ID").
- Windows: right-click → Properties → *Digital Signatures*, or
  `signtool verify /pa /v api2convert.exe` (or `osslsigncode verify api2convert.exe`).
