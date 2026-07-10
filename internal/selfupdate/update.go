// Package selfupdate replaces the running binary with the latest GitHub release
// after verifying its SHA-256 checksum.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"aead.dev/minisign"
	up "github.com/minio/selfupdate"
	"golang.org/x/mod/semver"
)

const (
	repo    = "QaamGo/api2convert-cli"
	binName = "api2convert"
	maxSize = 200 << 20 // 200 MB safety cap
)

// minisignPublicKey is the release signing key. checksums.txt is signed with the
// matching private key at release time (the `signs` block in .goreleaser.yaml),
// and self-update verifies checksums.txt.minisig against this key. A signature —
// unlike the same-origin checksum — proves the release was cut by the key holder,
// not merely that the download wasn't corrupted or a compromised release swapped.
//
// Empty until the keypair is provisioned (see SIGNING.md): in that state
// self-update verifies the SHA-256 checksum only, exactly as before. Once set,
// a missing or invalid signature aborts the update.
//
// Key ID BE5B81218245C1E6. The matching private key is the MINISIGN_PRIVATE_KEY
// release secret; checksums.txt is signed by the `signs` block in .goreleaser.yaml.
//
// A var (not const) only so tests can substitute a throwaway key; it is never
// reassigned outside tests.
var minisignPublicKey = "RWTmwUWCIYFbvtz1yIJF1SeVNqQpHEPD1M9MU68e8LSL9pYlD464IsoC"

// Seams for tests: overridden to point at an httptest server and to capture the
// binary that would be applied, instead of hitting GitHub and replacing the
// running executable. Never changed in production.
var (
	httpClient    = http.DefaultClient
	githubAPIBase = "https://api.github.com"
	applyBinary   = func(r io.Reader) error { return up.Apply(r, up.Options{}) }
)

// Result reports the outcome of a check/update.
type Result struct {
	Updated bool
	From    string
	To      string
}

// Available reports whether a newer release exists (no changes applied).
func Available(ctx context.Context, current string) (Result, error) {
	rel, err := latestRelease(ctx)
	if err != nil {
		return Result{}, err
	}
	return Result{From: current, To: strings.TrimPrefix(rel.TagName, "v")}, nil
}

// Run downloads the latest release for this OS/arch, verifies its checksum and
// atomically replaces the current binary.
func Run(ctx context.Context, current string) (Result, error) {
	rel, err := latestRelease(ctx)
	if err != nil {
		return Result{}, err
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	if !IsNewer(latest, current) {
		return Result{Updated: false, From: current, To: latest}, nil
	}
	if pm, ok := packageManaged(); ok {
		return Result{}, fmt.Errorf("installed via %s — update with '%s' instead", pm, pmCommand(pm))
	}

	asset := archiveName(latest)
	archiveURL := rel.assetURL(asset)
	sumsURL := rel.assetURL("checksums.txt")
	if archiveURL == "" || sumsURL == "" {
		return Result{}, fmt.Errorf("no release asset for %s/%s in %s", runtime.GOOS, runtime.GOARCH, rel.TagName)
	}

	archiveBytes, err := download(ctx, archiveURL)
	if err != nil {
		return Result{}, err
	}
	sums, err := download(ctx, sumsURL)
	if err != nil {
		return Result{}, err
	}
	if err := verifySignature(ctx, rel, sums); err != nil {
		return Result{}, err
	}
	if err := verifyChecksum(archiveBytes, asset, sums); err != nil {
		return Result{}, err
	}
	bin, err := extractBinary(archiveBytes)
	if err != nil {
		return Result{}, err
	}
	if err := applyBinary(bytes.NewReader(bin)); err != nil {
		return Result{}, err
	}
	return Result{Updated: true, From: current, To: latest}, nil
}

// IsNewer reports whether release version latest is strictly newer than the
// installed current. A plain string compare would "update" whenever the strings
// differ — downgrading a newer local/dev build, or re-installing an older
// release after the latest was yanked. semver.Compare needs a leading "v"; an
// unparseable current (e.g. a "dev" build) sorts below any real release, so it
// still updates.
func IsNewer(latest, current string) bool {
	return semver.Compare("v"+latest, "v"+current) > 0
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}

func (r release) assetURL(name string) string {
	for _, a := range r.Assets {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
}

func latestRelease(ctx context.Context) (release, error) {
	url := githubAPIBase + "/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var rel release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return release{}, err
	}
	return rel, nil
}

func archiveName(version string) string {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s.%s", binName, version, runtime.GOOS, runtime.GOARCH, ext)
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxSize))
}

// verifySignature checks the minisign signature over checksums.txt when a
// signing key is embedded. With no key it is a no-op (checksum-only, as before).
// With a key it is mandatory: an unsigned release or a bad signature aborts.
func verifySignature(ctx context.Context, rel release, sums []byte) error {
	if minisignPublicKey == "" {
		return nil
	}
	sigURL := rel.assetURL("checksums.txt.minisig")
	if sigURL == "" {
		return fmt.Errorf("release %s is not signed (checksums.txt.minisig missing)", rel.TagName)
	}
	sig, err := download(ctx, sigURL)
	if err != nil {
		return err
	}
	if err := verifyMinisignSignature(minisignPublicKey, sums, sig); err != nil {
		return fmt.Errorf("checksums.txt %w for %s", err, rel.TagName)
	}
	return nil
}

// verifyMinisignSignature reports whether signature is a valid minisign
// signature over message for the given textual public key.
func verifyMinisignSignature(publicKey string, message, signature []byte) error {
	var pub minisign.PublicKey
	if err := pub.UnmarshalText([]byte(publicKey)); err != nil {
		return fmt.Errorf("invalid signing key: %w", err)
	}
	if !minisign.Verify(pub, message, signature) {
		return errors.New("signature verification failed")
	}
	return nil
}

func verifyChecksum(data []byte, name string, sums []byte) error {
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && path.Base(fields[1]) == name {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum listed for %s", name)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != want {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}

func extractBinary(archiveBytes []byte) ([]byte, error) {
	if runtime.GOOS == "windows" {
		return extractFromZip(archiveBytes)
	}
	return extractFromTarGz(archiveBytes)
}

func extractFromTarGz(b []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if path.Base(hdr.Name) == binName {
			return io.ReadAll(io.LimitReader(tr, maxSize))
		}
	}
	return nil, fmt.Errorf("%s not found in archive", binName)
}

func extractFromZip(b []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if path.Base(f.Name) == binName+".exe" || path.Base(f.Name) == binName {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, maxSize))
		}
	}
	return nil, fmt.Errorf("%s not found in archive", binName)
}

// packageManaged reports whether the running binary lives under a package
// manager's directory, so we can refuse self-update and point at the manager.
func packageManaged() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	p := filepath.ToSlash(exe)
	switch {
	case strings.Contains(p, "/Cellar/"), strings.Contains(p, "/homebrew/"), strings.Contains(p, "/linuxbrew/"):
		return "homebrew", true
	case strings.Contains(p, "/scoop/"):
		return "scoop", true
	}
	return "", false
}

func pmCommand(pm string) string {
	switch pm {
	case "homebrew":
		return "brew upgrade api2convert"
	case "scoop":
		return "scoop update api2convert"
	default:
		return "your package manager"
	}
}
