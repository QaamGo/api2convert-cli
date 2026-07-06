package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestUniqueDownloadPathContainsTraversal verifies that a hostile server-supplied
// output filename or id can never make the download escape the target directory.
// Regression test for the path-traversal fix in uniqueDownloadPath.
func TestUniqueDownloadPathContainsTraversal(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Clean(dir) + string(filepath.Separator)

	cases := []struct {
		name     string
		filename string
		id       string
		want     string // expected basename of the resolved path
	}{
		{"traversal id, empty filename", "", "../../../etc/cron.d/evil", "evil"},
		{"traversal id, dot filename", ".", "../../../etc/cron.d/evil", "evil"},
		{"traversal filename", "../../../etc/passwd", "someid", "passwd"},
		{"nul byte in id", "", "../evil\x00.sh", "evil.sh"},
		{"backslash traversal id", "", `..\..\evil.exe`, "evil.exe"},
		{"both unusable falls back to output", "..", ".", "output"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := uniqueDownloadPath(dir, tc.filename, tc.id, map[string]bool{})

			// The resolved path must be a direct child of dir — no escape.
			cleaned := filepath.Clean(got)
			if !strings.HasPrefix(cleaned, prefix) {
				t.Fatalf("resolved path %q escaped dir %q", cleaned, dir)
			}
			if d := filepath.Dir(cleaned); d != filepath.Clean(dir) {
				t.Fatalf("resolved path parent = %q, want %q", d, filepath.Clean(dir))
			}
			if base := filepath.Base(cleaned); base != tc.want {
				t.Fatalf("basename = %q, want %q", base, tc.want)
			}
		})
	}
}
