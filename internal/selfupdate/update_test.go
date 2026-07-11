package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"aead.dev/minisign"
)

// TestEmbeddedPublicKeyParses guards the embedded release key: once a key is set,
// it must be a valid minisign public key — otherwise every self-update would fail
// signature verification. Skips (rather than fails) while no key is embedded.
func TestEmbeddedPublicKeyParses(t *testing.T) {
	if minisignPublicKey == "" {
		t.Skip("no signing key embedded yet")
	}
	var pub minisign.PublicKey
	if err := pub.UnmarshalText([]byte(minisignPublicKey)); err != nil {
		t.Fatalf("embedded minisignPublicKey does not parse as a minisign public key: %v", err)
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"1.2.0", "1.1.0", true},      // normal upgrade
		{"1.1.0", "1.1.0", false},     // same version → no update
		{"1.0.0", "1.1.0", false},     // older "latest" (e.g. after a yank) → no downgrade
		{"1.2.0", "1.2.0-rc.1", true}, // stable is newer than its prerelease
		{"1.2.0-rc.1", "1.2.0", false},
		{"1.0.0", "dev", true}, // unparseable current sorts below any real release
	}
	for _, c := range cases {
		if got := IsNewer(c.latest, c.current); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("archive-bytes")
	sum := sha256.Sum256(data)
	hexsum := hex.EncodeToString(sum[:])
	name := "api2convert_1.0.0_linux_amd64.tar.gz"
	sums := hexsum + "  " + name + "\nffffffff  other_file.zip\n"

	if err := verifyChecksum(data, name, []byte(sums)); err != nil {
		t.Fatalf("valid checksum should pass: %v", err)
	}
	// Checksums file may list hashes in uppercase.
	if err := verifyChecksum(data, name, []byte(strings.ToUpper(hexsum)+"  "+name)); err != nil {
		t.Errorf("uppercase checksum should pass: %v", err)
	}
	// Tampered payload → mismatch.
	if err := verifyChecksum([]byte("tampered"), name, []byte(sums)); err == nil {
		t.Error("tampered data should fail the checksum")
	}
	// Asset not listed → fail closed, never a silent pass.
	if err := verifyChecksum(data, "not_listed.tar.gz", []byte(sums)); err == nil {
		t.Error("a missing checksum entry must fail closed")
	}
}

func TestVerifyMinisignSignature(t *testing.T) {
	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubText, _ := pub.MarshalText()
	sums := []byte("deadbeef  api2convert_1.0.0_linux_amd64.tar.gz\n")
	sig := minisign.Sign(priv, sums)

	if err := verifyMinisignSignature(string(pubText), sums, sig); err != nil {
		t.Fatalf("a valid signature should verify: %v", err)
	}
	// Tampered message.
	if err := verifyMinisignSignature(string(pubText), []byte("tampered"), sig); err == nil {
		t.Error("a tampered message must fail verification")
	}
	// Signature from a different key.
	otherPub, _, _ := minisign.GenerateKey(rand.Reader)
	otherText, _ := otherPub.MarshalText()
	if err := verifyMinisignSignature(string(otherText), sums, sig); err == nil {
		t.Error("a signature from an untrusted key must fail")
	}
	// Garbage signature bytes.
	if err := verifyMinisignSignature(string(pubText), sums, []byte("not a signature")); err == nil {
		t.Error("a malformed signature must fail")
	}
	// Malformed public key.
	if err := verifyMinisignSignature("not-a-key", sums, sig); err == nil {
		t.Error("a malformed public key must error")
	}
}

// TestUpdateClientTimeouts guards the self-update client's shape: a server that
// connects then stalls must be bounded by dial/TLS/response-header timeouts on
// the Transport, while the body download stays uncapped (io.LimitReader already
// bounds size; a whole-request Client.Timeout would abort a slow large download).
func TestUpdateClientTimeouts(t *testing.T) {
	c := newUpdateClient()
	if c.Timeout != 0 {
		t.Errorf("Client.Timeout = %v, want 0 (must not cap the body download)", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport = %T, want *http.Transport", c.Transport)
	}
	if tr.DialContext == nil {
		t.Error("DialContext must be set to bound connect")
	}
	if tr.TLSHandshakeTimeout <= 0 {
		t.Error("TLSHandshakeTimeout must be bounded")
	}
	if tr.ResponseHeaderTimeout <= 0 {
		t.Error("ResponseHeaderTimeout must be bounded so a stalled server can't hang")
	}
}

func TestArchiveName(t *testing.T) {
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	want := "api2convert_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH + "." + ext
	if got := archiveName("1.2.3"); got != want {
		t.Errorf("archiveName = %q, want %q", got, want)
	}
}

func TestReleaseAssetURL(t *testing.T) {
	rel := release{Assets: []asset{
		{Name: "checksums.txt", URL: "https://x/checksums.txt"},
		{Name: "api2convert_1.0.0_linux_amd64.tar.gz", URL: "https://x/archive"},
	}}
	if got := rel.assetURL("api2convert_1.0.0_linux_amd64.tar.gz"); got != "https://x/archive" {
		t.Errorf("assetURL(match) = %q", got)
	}
	if got := rel.assetURL("missing.zip"); got != "" {
		t.Errorf("assetURL(miss) = %q, want empty", got)
	}
}

func TestExtractFromTarGz(t *testing.T) {
	content := []byte("UNIX-BINARY")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	writeTar(t, tw, "README.md", []byte("readme")) // decoy before the binary
	writeTar(t, tw, binName, content)
	mustClose(t, tw, gz)

	got, err := extractFromTarGz(buf.Bytes())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("extracted %q, want %q", got, content)
	}

	// No binary in the archive → error, not empty bytes.
	var buf2 bytes.Buffer
	gz2 := gzip.NewWriter(&buf2)
	tw2 := tar.NewWriter(gz2)
	writeTar(t, tw2, "README.md", []byte("only docs"))
	mustClose(t, tw2, gz2)
	if _, err := extractFromTarGz(buf2.Bytes()); err == nil {
		t.Error("a tar.gz without the binary should error")
	}
}

func TestExtractFromZip(t *testing.T) {
	content := []byte("WIN-BINARY")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(binName + ".exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := extractFromZip(buf.Bytes())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("extracted %q, want %q", got, content)
	}
}

func TestPMCommand(t *testing.T) {
	if got := pmCommand("homebrew"); got != "brew upgrade api2convert" {
		t.Errorf("homebrew: %q", got)
	}
	if got := pmCommand("scoop"); got != "scoop update api2convert" {
		t.Errorf("scoop: %q", got)
	}
	if pmCommand("apt") == "" {
		t.Error("unknown manager should still return a non-empty hint")
	}
}

// TestRunEndToEnd drives the whole update against a fake GitHub release: resolve
// latest → download archive + checksums + signature → verify signature → verify
// checksum → extract → "apply". The applied bytes must equal the binary packed
// into the archive. No network, no real binary replacement.
func TestRunEndToEnd(t *testing.T) {
	const latest = "1.2.0"
	binContent := []byte("NEW-BINARY-" + runtime.GOOS + "-" + runtime.GOARCH)
	archiveBytes := buildArchive(t, binContent)
	archive := archiveName(latest)

	sum := sha256.Sum256(archiveBytes)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + archive + "\n")

	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sig := minisign.Sign(priv, sums)
	pubText, _ := pub.MarshalText()

	srv := newReleaseServer(t, "v"+latest, map[string][]byte{
		archive:                 archiveBytes,
		"checksums.txt":         sums,
		"checksums.txt.minisig": sig,
	})
	applied := wireSeams(t, srv, string(pubText))

	res, err := Run(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Updated || res.From != "1.0.0" || res.To != latest {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !bytes.Equal(*applied, binContent) {
		t.Fatalf("applied binary = %q, want %q", *applied, binContent)
	}
}

// TestRunRejectsTamperedChecksum: a correctly-signed checksums.txt that lists the
// wrong hash for the archive must still block the update (checksum gate).
func TestRunRejectsTamperedChecksum(t *testing.T) {
	const latest = "1.2.0"
	archiveBytes := buildArchive(t, []byte("BIN"))
	archive := archiveName(latest)
	sums := []byte(strings.Repeat("0", 64) + "  " + archive + "\n") // wrong hash

	pub, priv, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sig := minisign.Sign(priv, sums) // validly signed, but the hash is bogus
	pubText, _ := pub.MarshalText()

	srv := newReleaseServer(t, "v"+latest, map[string][]byte{
		archive:                 archiveBytes,
		"checksums.txt":         sums,
		"checksums.txt.minisig": sig,
	})
	applied := wireSeams(t, srv, string(pubText))

	if _, err := Run(context.Background(), "1.0.0"); err == nil {
		t.Fatal("a checksum mismatch must fail the update")
	}
	if len(*applied) != 0 {
		t.Fatal("no binary must be applied when the checksum fails")
	}
}

// TestRunRejectsUntrustedSignature: the checksum is correct, but the signature
// was made by a key the binary does not trust → the update must abort.
func TestRunRejectsUntrustedSignature(t *testing.T) {
	const latest = "1.2.0"
	archiveBytes := buildArchive(t, []byte("BIN"))
	archive := archiveName(latest)
	sum := sha256.Sum256(archiveBytes)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + archive + "\n")

	_, signingKey, err := minisign.GenerateKey(rand.Reader) // key A signs
	if err != nil {
		t.Fatal(err)
	}
	sig := minisign.Sign(signingKey, sums)
	trustedPub, _, err := minisign.GenerateKey(rand.Reader) // binary trusts key B
	if err != nil {
		t.Fatal(err)
	}
	trustedText, _ := trustedPub.MarshalText()

	srv := newReleaseServer(t, "v"+latest, map[string][]byte{
		archive:                 archiveBytes,
		"checksums.txt":         sums,
		"checksums.txt.minisig": sig,
	})
	applied := wireSeams(t, srv, string(trustedText))

	if _, err := Run(context.Background(), "1.0.0"); err == nil {
		t.Fatal("a signature from an untrusted key must fail the update")
	}
	if len(*applied) != 0 {
		t.Fatal("no binary must be applied when the signature fails")
	}
}

// TestRunNoUpdateWhenNotNewer: when the latest release is not newer than the
// running version, Run returns cleanly without downloading or applying anything.
func TestRunNoUpdateWhenNotNewer(t *testing.T) {
	srv := newReleaseServer(t, "v1.0.0", nil)
	applied := wireSeams(t, srv, minisignPublicKey)

	res, err := Run(context.Background(), "1.0.0")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Updated {
		t.Fatal("must not update when current == latest")
	}
	if len(*applied) != 0 {
		t.Fatal("nothing should be applied when there is no newer release")
	}
}

// --- helpers --------------------------------------------------------------

// newReleaseServer serves a GitHub "latest release" plus the named asset bodies.
// The release JSON is built per-request so asset URLs can point back at the
// server's own address.
func newReleaseServer(t *testing.T, tag string, assets map[string][]byte) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, _ *http.Request) {
		rel := release{TagName: tag}
		for name := range assets {
			rel.Assets = append(rel.Assets, asset{Name: name, URL: srv.URL + "/dl/" + name})
		}
		_ = json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		body, ok := assets[strings.TrimPrefix(r.URL.Path, "/dl/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// wireSeams points the package at the test server and captures the binary that
// Run would apply, restoring every global on cleanup. Tests using it must not run
// in parallel (they share these package vars).
func wireSeams(t *testing.T, srv *httptest.Server, pubKey string) *[]byte {
	t.Helper()
	setVar(t, &githubAPIBase, srv.URL)
	setVar(t, &httpClient, srv.Client())
	setVar(t, &minisignPublicKey, pubKey)
	applied := new([]byte)
	setVar(t, &applyBinary, func(r io.Reader) error {
		b, err := io.ReadAll(r)
		*applied = b
		return err
	})
	return applied
}

func setVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	t.Cleanup(func() { *p = old })
	*p = v
}

func buildArchive(t *testing.T, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(binName + ".exe")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	writeTar(t, tw, binName, content)
	mustClose(t, tw, gz)
	return buf.Bytes()
}

func writeTar(t *testing.T, tw *tar.Writer, name string, body []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
}

func mustClose(t *testing.T, tw *tar.Writer, gz *gzip.Writer) {
	t.Helper()
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}
