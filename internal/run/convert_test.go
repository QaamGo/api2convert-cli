package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaimPath(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Missing target: claims it (creates a placeholder), proceeds.
	missing := filepath.Join(dir, "new.pdf")
	if p, skip, err := claimPath(missing, "error"); err != nil || skip || p != missing {
		t.Fatalf("missing: got (%q,%v,%v)", p, skip, err)
	}
	// skip on existing
	if _, skip, err := claimPath(existing, "skip"); err != nil || !skip {
		t.Fatalf("skip: got skip=%v err=%v", skip, err)
	}
	// overwrite returns target, no claim
	if p, skip, err := claimPath(existing, "overwrite"); err != nil || skip || p != existing {
		t.Fatalf("overwrite: got (%q,%v,%v)", p, skip, err)
	}
	// error on existing
	if _, _, err := claimPath(existing, "error"); err == nil {
		t.Fatal("error policy should error on existing file")
	}
	// rename yields a new, actually-created path
	if p, _, err := claimPath(existing, "rename"); err != nil || p == existing {
		t.Fatalf("rename: got (%q,%v)", p, err)
	} else if _, statErr := os.Stat(p); statErr != nil {
		t.Fatalf("rename target not created: %v", statErr)
	}
	// invalid policy
	if _, _, err := claimPath(existing, "bogus"); err == nil {
		t.Fatal("invalid policy should error")
	}
}

// TestClaimPathAtomic verifies two concurrent claims of the same path never both
// succeed (the O_EXCL race fix).
func TestClaimPathAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "same.pdf")
	const n = 8
	type res struct {
		final string
		skip  bool
		err   error
	}
	ch := make(chan res, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			<-start
			f, s, e := claimPath(target, "error")
			ch <- res{f, s, e}
		}()
	}
	close(start)
	wins := 0
	for i := 0; i < n; i++ {
		r := <-ch
		if r.err == nil && !r.skip {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("expected exactly 1 winner claiming the path, got %d", wins)
	}
}

func TestSafeBase(t *testing.T) {
	cases := map[string]string{
		"out.png":          "out.png",
		"../../etc/passwd": "passwd",
		`a\b\c.pdf`:        "c.pdf",
		"":                 "",
		"..":               "",
	}
	for in, want := range cases {
		if got := safeBase(in); got != want {
			t.Errorf("safeBase(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsURLAndLocal(t *testing.T) {
	if !isURL("https://x/y") || !isURL("HTTP://x") {
		t.Error("isURL should match http(s)")
	}
	if isURL("/tmp/a.png") || isURL("-") {
		t.Error("isURL should not match local/stdin")
	}
	if isLocalFile("-") || isLocalFile("https://x") {
		t.Error("isLocalFile should exclude stdin/URL")
	}
	if !isLocalFile("a.png") {
		t.Error("isLocalFile should include plain paths")
	}
}

func TestDisplayName(t *testing.T) {
	if displayName("-") != "stdin" {
		t.Error("stdin")
	}
	if displayName("https://x/y.pdf") != "https://x/y.pdf" {
		t.Error("url passthrough")
	}
	if displayName("/a/b/c.docx") != "c.docx" {
		t.Error("basename")
	}
}
