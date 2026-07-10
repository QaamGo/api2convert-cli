# Security Policy

## Reporting a vulnerability

Please report security issues privately via GitHub's **Security → Report a
vulnerability** on this repository, or email security@qaamgo.com. Do not open a
public issue for security reports.

We aim to acknowledge reports within a few business days.

## Handling of secrets

- Your API key is read from `--api-key`, the `API2CONVERT_API_KEY` environment
  variable, or the config file (written with `0600` permissions).
- The key is never printed or logged; `api2convert config get api-key` shows only
  the last four characters.
- The underlying SDK never places secrets (API key, upload token, download
  password) in request URLs, redirects or error messages.

## Release integrity

Release archives are published with a `checksums.txt` (SHA-256). The install
scripts and `api2convert self-update` verify this checksum before installing, and
fail closed if a matching entry is missing.

Note the current guarantee: because `checksums.txt` is fetched from the same
GitHub release as the archive, the checksum protects against **corrupted or
truncated downloads**, not against a tampered or compromised release. Release
authenticity is additionally provided by a detached **minisign** signature over
`checksums.txt`, which `self-update` verifies against a public key embedded in the
binary — refusing to update on a missing or invalid signature. See
[SIGNING.md](SIGNING.md) for the signing setup and current status.

The installed binaries are also OS-code-signed (Windows Authenticode, macOS
Developer ID + notarization); see [SIGNING.md](SIGNING.md).
