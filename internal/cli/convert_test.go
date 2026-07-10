package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoerce(t *testing.T) {
	if coerce("true") != true || coerce("false") != false {
		t.Error("bool coercion")
	}
	if coerce("85") != 85 {
		t.Error("int coercion")
	}
	if coerce("1.5") != 1.5 {
		t.Error("float coercion")
	}
	if coerce("h264") != "h264" {
		t.Error("string passthrough")
	}
}

func TestResolveOnConflict(t *testing.T) {
	if v, _ := resolveOnConflict("error", true, false); v != "overwrite" {
		t.Error("--force → overwrite")
	}
	if v, _ := resolveOnConflict("error", false, true); v != "skip" {
		t.Error("--no-clobber → skip")
	}
	if _, err := resolveOnConflict("error", true, true); err == nil {
		t.Error("force+no-clobber should error")
	}
	if v, _ := resolveOnConflict("rename", false, false); v != "rename" {
		t.Error("passthrough rename")
	}
	if _, err := resolveOnConflict("bogus", false, false); err == nil {
		t.Error("invalid policy should error")
	}
}

func TestInferTarget(t *testing.T) {
	if inferTarget("report.PDF") != "pdf" {
		t.Error("extension should lowercase")
	}
	if inferTarget("") != "" {
		t.Error("empty out → empty target")
	}
}

func TestParseOptions(t *testing.T) {
	opts, err := parseOptions([]string{"quality=80", "strip=true", "codec=h264"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if opts["quality"] != 80 || opts["strip"] != true || opts["codec"] != "h264" {
		t.Fatalf("bad opts: %#v", opts)
	}
	if _, err := parseOptions([]string{"noequals"}, ""); err == nil {
		t.Error("missing = should error")
	}
}

func TestParseOptionsRawString(t *testing.T) {
	// key:=value forces a literal string; key=value still coerces.
	opts, err := parseOptions([]string{"pw:=080", "n=080", "flag:=true"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if opts["pw"] != "080" {
		t.Errorf("pw should stay string %q, got %#v", "080", opts["pw"])
	}
	if opts["n"] != 80 {
		t.Errorf("n should coerce to int 80, got %#v", opts["n"])
	}
	if opts["flag"] != "true" {
		t.Errorf("flag should stay string \"true\", got %#v", opts["flag"])
	}
}

func TestItemForPreservesTree(t *testing.T) {
	walked := resolvedInput{path: filepath.Join("photos", "2021", "a.jpg"), rel: filepath.Join("2021", "a.jpg")}

	// Under an --out-dir: mirror the subtree, swap the extension.
	if it := itemFor(walked, "webp", "web"+string(os.PathSeparator)); it.Out != filepath.Join("web", "2021", "a.webp") {
		t.Errorf("out-dir tree: got %q", it.Out)
	}
	// No destination: write alongside the source, preserving the tree.
	if it := itemFor(walked, "webp", ""); it.Out != filepath.Join("photos", "2021", "a.webp") {
		t.Errorf("alongside source: got %q", it.Out)
	}
	// Non-walked input (rel == "") keeps default naming — no per-item override.
	if it := itemFor(resolvedInput{path: "a.jpg"}, "webp", "web"+string(os.PathSeparator)); it.Out != "" {
		t.Errorf("non-walked should not set Out, got %q", it.Out)
	}
	// Explicit --out FILE (a single walked input) is respected — no override.
	if it := itemFor(walked, "webp", "only.webp"); it.Out != "" {
		t.Errorf("explicit out file should not be overridden, got %q", it.Out)
	}
}

func TestExpandInputsCarriesRel(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := expandInputs([]string{dir}, true)
	if err != nil || len(got) != 1 {
		t.Fatalf("walk: %v %v", got, err)
	}
	if got[0].rel != filepath.Join("sub", "b.txt") {
		t.Errorf("rel = %q, want %q", got[0].rel, filepath.Join("sub", "b.txt"))
	}
	// Passthrough inputs carry no rel.
	pt, _ := expandInputs([]string{"https://x/y", "-"}, false)
	for _, in := range pt {
		if in.rel != "" {
			t.Errorf("passthrough %q should have empty rel, got %q", in.path, in.rel)
		}
	}
}

func TestExpandInputs(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	sub := filepath.Join(dir, "sub")
	_ = os.MkdirAll(sub, 0o755)
	b := filepath.Join(sub, "b.txt")
	_ = os.WriteFile(a, []byte("x"), 0o644)
	_ = os.WriteFile(b, []byte("y"), 0o644)

	// URL and stdin pass through.
	got, err := expandInputs([]string{"https://x/y", "-"}, false)
	if err != nil || len(got) != 2 {
		t.Fatalf("passthrough: %v %v", got, err)
	}

	// Directory without --recursive errors.
	if _, err := expandInputs([]string{dir}, false); err == nil {
		t.Error("dir without --recursive should error")
	}

	// Directory with --recursive walks files.
	got, err = expandInputs([]string{dir}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("recursive walk got %v", got)
	}
}
