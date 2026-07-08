//go:build live

// Package live_test is the CLI's live conformance suite. Unlike the hermetic
// unit tests (which inject a fake api2convert.HttpSender and never touch the
// network), this suite builds the real `api2convert` binary and drives it
// end-to-end against the live api2convert API, so it consumes quota and is
// gated two ways:
//
//   - The build tag `live` keeps it out of the default `go test ./...` (and out
//     of `go vet`'s default build). Run it explicitly with:
//
//     go test -tags live -timeout 600s ./live/...
//
//   - Even then, every test skips unless API2CONVERT_API_KEY is set. Export the
//     shared test-account key first (the same key the sibling SDK suites use):
//
//     API2CONVERT_API_KEY=<key> go test -tags live -timeout 600s ./live/...
//
// An optional API2CONVERT_BASE_URL selects a non-prod environment; it passes
// through to the binary via the process environment, exactly as an end user
// would set it.
//
// Never commit a real key — it is read only from the environment, and the
// negative tests assert the CLI never echoes a key back into its output.
//
// Each test mirrors one documented example guide (the same catalog every
// api2convert SDK implements), but exercises it through the *command line*:
// argv in, exit code + stdout/stderr out. Where the SDK conformance suites
// assert on typed errors, this suite asserts on the CLI's stable exit codes
// (see internal/clierr) and its --json envelopes. The file doubles as an
// executable tour of the CLI.
package live_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Remote fixtures — small, stable public files served by online-convert, the
// same set the SDK conformance suites use. Remote-URL inputs skip the upload
// handshake (see TestUploadLocalFile for the multipart path).
const (
	remotePDF  = "https://example-files.online-convert.com/document/pdf/example.pdf"
	remotePNG  = "https://example-files.online-convert.com/raster%20image/png/example.png"
	remoteJPG  = "https://example-files.online-convert.com/raster%20image/jpg/example.jpg"
	remoteDOCX = "https://example-files.online-convert.com/document/docx/example.docx"
)

// onePxPNG is a minimal valid 1x1 PNG, written to disk so the upload test
// exercises the real multipart upload handshake.
var onePxPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
	0x00, 0x00, 0x03, 0x01, 0x01, 0x00, 0x18, 0xDD, 0x8D, 0xB0, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45,
	0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}

// CLI exit codes, mirrored from internal/clierr on purpose: the suite asserts
// the *documented* contract, so it hard-codes the numbers rather than importing
// the internal package. A drift here means the contract changed.
const (
	exitOK         = 0
	exitUsage      = 2
	exitAuth       = 3
	exitValidation = 5
	exitConversion = 6
)

var (
	binPath  string // the built binary; empty when the suite is skipping (no key)
	testHome string // an isolated HOME/config/cache dir shared by all runs
)

// TestMain builds the binary once (when a key is present) and shares an isolated
// config/cache home across the suite so the on-disk catalog is fetched once.
func TestMain(m *testing.M) {
	// No key → nothing to build; every test skips. Still run so the skips are
	// reported and so the package compiles in CI without a secret present.
	if os.Getenv("API2CONVERT_API_KEY") == "" {
		os.Exit(m.Run())
	}

	dir, err := os.MkdirTemp("", "a2c-cli-live-")
	if err != nil {
		panic(err)
	}
	testHome = filepath.Join(dir, "home")
	if err := os.MkdirAll(testHome, 0o755); err != nil {
		panic(err)
	}

	binPath = filepath.Join(dir, binName())
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = repoRoot()
	// buildvcs off keeps the build working in shallow/odd checkouts (matches the
	// documented Docker build in AGENTS.md).
	build.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("building api2convert binary: " + err.Error())
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// repoRoot resolves the module root (the parent of this live/ directory) from
// the compile-time path of this source file, independent of the test's cwd.
func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve caller for repo root")
	}
	return filepath.Dir(filepath.Dir(file))
}

func binName() string {
	if runtime.GOOS == "windows" {
		return "api2convert.exe"
	}
	return "api2convert"
}

// result captures one CLI invocation.
type result struct {
	stdout string
	stderr string
	code   int
}

// requireLive skips the calling test unless the suite is live (key present and
// binary built).
func requireLive(t *testing.T) {
	t.Helper()
	if binPath == "" {
		t.Skip("live tests require API2CONVERT_API_KEY (export the shared test key to run)")
	}
}

// runCLI executes the built binary with args in an isolated config/cache HOME so
// a developer's real ~/.config/api2convert never bleeds into the assertions. The
// API key and optional base URL pass through from the ambient environment,
// exactly as an end user configures them.
func runCLI(t *testing.T, args ...string) result {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = testEnv()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("running %v failed to start: %v", args, err)
	}
	return result{stdout: out.String(), stderr: errb.String(), code: cmd.ProcessState.ExitCode()}
}

// testEnv keeps API2CONVERT_API_KEY and API2CONVERT_BASE_URL from the ambient
// environment, drops any other API2CONVERT_* so a stray developer setting can't
// skew a test, and redirects every OS config/cache lookup into the isolated
// home (covers Linux XDG, macOS $HOME/Library, and Windows %AppData%).
func testEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch {
		case key == "API2CONVERT_API_KEY" || key == "API2CONVERT_BASE_URL":
			env = append(env, kv)
		case strings.HasPrefix(key, "API2CONVERT_"):
			// drop: don't let output=json etc. from the environment change assertions
		case key == "HOME" || key == "XDG_CONFIG_HOME" || key == "XDG_CACHE_HOME" ||
			key == "APPDATA" || key == "LOCALAPPDATA":
			// dropped; overridden below
		default:
			env = append(env, kv)
		}
	}
	return append(env,
		"HOME="+testHome,
		"XDG_CONFIG_HOME="+filepath.Join(testHome, "config"),
		"XDG_CACHE_HOME="+filepath.Join(testHome, "cache"),
		"APPDATA="+testHome,
		"LOCALAPPDATA="+testHome,
	)
}

// --- assertion helpers ----------------------------------------------------

func mustExit(t *testing.T, r result, want int) {
	t.Helper()
	if r.code != want {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, want, r.stdout, r.stderr)
	}
}

func mustNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if fi.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
}

// decodeJSON parses the whole stdout of a --json invocation into T.
func decodeJSON[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, s)
	}
	return v
}

// firstJobID pulls jobs[0].job_id from an async --json envelope.
func firstJobID(t *testing.T, stdout string) string {
	t.Helper()
	env := decodeJSON[map[string]any](t, stdout)
	jobs, _ := env["jobs"].([]any)
	if len(jobs) == 0 {
		t.Fatalf("no started jobs in envelope:\n%s", stdout)
	}
	j0, _ := jobs[0].(map[string]any)
	id, _ := j0["job_id"].(string)
	if id == "" {
		t.Fatalf("started job has no id:\n%s", stdout)
	}
	return id
}

// ============================ positive: the guided tour ====================

// 1. quickstart / convert-files — convert a remote JPG to PNG, download it. ---
func TestConvertRemoteToPNG(t *testing.T) {
	requireLive(t)
	out := filepath.Join(t.TempDir(), "out.png")
	r := runCLI(t, "convert", remoteJPG, "--to", "png", "-o", out)
	mustExit(t, r, exitOK)
	mustNonEmptyFile(t, out)
}

// 2. the --json envelope of a successful single conversion. -------------------
func TestConvertJSONEnvelope(t *testing.T) {
	requireLive(t)
	out := filepath.Join(t.TempDir(), "out.png")
	r := runCLI(t, "convert", remoteJPG, "--to", "png", "-o", out, "--json")
	mustExit(t, r, exitOK)
	env := decodeJSON[map[string]any](t, r.stdout)
	if env["ok"] != true {
		t.Fatalf("ok != true: %v", env)
	}
	if p, _ := env["output_path"].(string); p == "" {
		t.Fatalf("missing output_path: %v", env)
	}
	mustNonEmptyFile(t, out)
}

// 3. uploading-files — upload a local file (real multipart handshake). --------
func TestUploadLocalFile(t *testing.T) {
	requireLive(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "pixel.png")
	if err := os.WriteFile(src, onePxPNG, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.png")
	r := runCLI(t, "convert", src, "--to", "png", "-o", out)
	mustExit(t, r, exitOK)
	mustNonEmptyFile(t, out)
}

// 4. convert-files — browse the catalog as machine JSON. ----------------------
func TestFormatsListJSON(t *testing.T) {
	requireLive(t)
	r := runCLI(t, "formats", "list", "--json")
	mustExit(t, r, exitOK)
	if len(decodeJSON[[]map[string]any](t, r.stdout)) == 0 {
		t.Fatal("the catalog should be non-empty")
	}
}

// 5. catalog filters — categories and search return non-empty results. --------
func TestFormatsBrowse(t *testing.T) {
	requireLive(t)
	t.Run("categories", func(t *testing.T) {
		r := runCLI(t, "formats", "categories", "--json")
		mustExit(t, r, exitOK)
		if len(decodeJSON[[]string](t, r.stdout)) == 0 {
			t.Fatal("expected at least one category")
		}
	})
	t.Run("search", func(t *testing.T) {
		r := runCLI(t, "formats", "search", "png", "--json")
		mustExit(t, r, exitOK)
		if len(decodeJSON[[]map[string]any](t, r.stdout)) == 0 {
			t.Fatal("expected at least one match for png")
		}
	})
}

// 6. options — the option schema for a target is a JSON object. ---------------
func TestOptionsSchema(t *testing.T) {
	requireLive(t)
	r := runCLI(t, "options", "png", "--json")
	mustExit(t, r, exitOK)
	// May be an empty object for some targets, but it must parse as one.
	_ = decodeJSON[map[string]any](t, r.stdout)
}

// 7. compress-files — re-encode a JPG to a JPG (same-format verb). ------------
func TestCompressVerb(t *testing.T) {
	requireLive(t)
	out := filepath.Join(t.TempDir(), "compressed.jpg")
	// A remote URL carries no extension for the same-format verb to infer, so the
	// target is given explicitly — this still exercises the compress preset.
	r := runCLI(t, "compress", remoteJPG, "--to", "jpg", "-o", out)
	mustExit(t, r, exitOK)
	mustNonEmptyFile(t, out)
}

// 8. create-thumbnails — render a PDF page to a PNG thumbnail (gated verb). ----
func TestThumbnailVerb(t *testing.T) {
	requireLive(t)
	out := filepath.Join(t.TempDir(), "thumb.png")
	r := runCLI(t, "thumbnail", remotePDF,
		"--option", "thumbnail_target=png",
		"--option", "width=300",
		"--option", "pages=first",
		"--option", "dpi=150",
		"-o", out)
	mustExit(t, r, exitOK)
	mustNonEmptyFile(t, out)
}

// 9. image-operations — resize a JPG (gated resize-image verb). ---------------
func TestResizeVerb(t *testing.T) {
	requireLive(t)
	out := filepath.Join(t.TempDir(), "resized.jpg")
	r := runCLI(t, "resize", remoteJPG,
		"--option", "width=800",
		"--option", "height=600",
		"--option", "resize_by=px",
		"--option", "resize_handling=keep_aspect_ratio_crop",
		"-o", out)
	mustExit(t, r, exitOK)
	mustNonEmptyFile(t, out)
}

// 10. merge — combine two remote PDFs into one PDF. ---------------------------
func TestMergeVerb(t *testing.T) {
	requireLive(t)
	out := filepath.Join(t.TempDir(), "merged.pdf")
	r := runCLI(t, "merge", remotePDF, remotePDF, "--to", "pdf", "-o", out)
	mustExit(t, r, exitOK)
	mustNonEmptyFile(t, out)
}

// 11. job-lifecycle (async) — start without waiting; get a job id back. -------
func TestConvertAsyncJSON(t *testing.T) {
	requireLive(t)
	r := runCLI(t, "convert", remoteJPG, "--to", "png", "--async", "--json")
	mustExit(t, r, exitOK)
	_ = firstJobID(t, r.stdout)
}

// 12. webhooks — an async convert with a callback starts and returns an id. ---
//
// A webhook receipt is not testable here, so we assert only that --callback
// implies async and yields a started job with an id.
func TestConvertWithCallback(t *testing.T) {
	requireLive(t)
	r := runCLI(t, "convert", remoteDOCX, "--to", "pdf",
		"--callback", "https://your-app.example.com/api2convert/webhook", "--json")
	mustExit(t, r, exitOK)
	env := decodeJSON[map[string]any](t, r.stdout)
	if env["async"] != true {
		t.Fatalf("--callback should imply async: %v", env)
	}
	_ = firstJobID(t, r.stdout)
}

// 13. authentication — an authenticated jobs list returns a JSON array. -------
func TestJobsListJSON(t *testing.T) {
	requireLive(t)
	r := runCLI(t, "jobs", "list", "--json")
	mustExit(t, r, exitOK)
	_ = decodeJSON[[]map[string]any](t, r.stdout) // an empty list is valid
}

// 14. full lifecycle — start async, status, wait, download. -------------------
func TestJobLifecycle(t *testing.T) {
	requireLive(t)
	start := runCLI(t, "convert", remoteJPG, "--to", "png", "--async", "--json")
	mustExit(t, start, exitOK)
	id := firstJobID(t, start.stdout)

	// status: the job exists and reports some status code.
	st := runCLI(t, "jobs", "status", id, "--json")
	mustExit(t, st, exitOK)
	sv := decodeJSON[map[string]any](t, st.stdout)
	if sv["id"] != id {
		t.Fatalf("status id = %v, want %s", sv["id"], id)
	}
	if s, _ := sv["status"].(string); s == "" {
		t.Fatalf("status is missing a status code: %v", sv)
	}

	// wait: block until it finishes; it must complete.
	w := runCLI(t, "jobs", "wait", id, "--json")
	mustExit(t, w, exitOK)
	if wv := decodeJSON[map[string]any](t, w.stdout); wv["status"] != "completed" {
		t.Fatalf("waited job status = %v, want completed", wv["status"])
	}

	// download: at least one output file lands in a fresh directory.
	dir := t.TempDir()
	dl := runCLI(t, "jobs", "download", id, "-o", dir)
	mustExit(t, dl, exitOK)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("download wrote no files")
	}
}

// 15. rate-limits / credits — the contracts call returns a JSON payload. ------
func TestCredits(t *testing.T) {
	requireLive(t)
	r := runCLI(t, "credits", "--json")
	mustExit(t, r, exitOK)
	if v := decodeJSON[any](t, r.stdout); v == nil {
		t.Fatal("empty credits payload")
	}
}

// ============================ negative: the exit-code contract =============

// N1. An unknown target is a validation/conversion failure, and the --json
// error envelope reports ok:false. ------------------------------------------
func TestInvalidTargetExitCode(t *testing.T) {
	requireLive(t)
	out := filepath.Join(t.TempDir(), "x")
	r := runCLI(t, "convert", remoteJPG, "--to", "this-is-not-a-real-target", "-o", out, "--json")
	if r.code != exitValidation && r.code != exitConversion {
		t.Fatalf("exit code = %d, want %d or %d\nstdout:\n%s\nstderr:\n%s",
			r.code, exitValidation, exitConversion, r.stdout, r.stderr)
	}
	if env := decodeJSON[map[string]any](t, r.stdout); env["ok"] != false {
		t.Fatalf("error envelope ok should be false: %v", env)
	}
}

// N2. A bad key is a typed auth failure (exit 3) that never leaks the key. ----
//
// The suite is gated on the real key, but this test authenticates with a bogus
// one via --api-key (which beats the environment) to force the auth path.
func TestBadKeyAuthExitCodeNoLeak(t *testing.T) {
	requireLive(t)
	const bogus = "a2c-invalid-key-for-testing"
	r := runCLI(t, "--api-key", bogus, "jobs", "list")
	mustExit(t, r, exitAuth)
	if strings.Contains(r.stdout, bogus) || strings.Contains(r.stderr, bogus) {
		t.Fatal("the CLI must never echo the API key back to the user")
	}
}

// N3. An unknown flag is a usage error (exit 2), mapped before any network. ---
func TestUnknownFlagIsUsageError(t *testing.T) {
	requireLive(t)
	r := runCLI(t, "convert", remoteJPG, "--to", "png", "--totally-unknown-flag")
	mustExit(t, r, exitUsage)
}
