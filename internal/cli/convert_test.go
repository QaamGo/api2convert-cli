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
