package run

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api2convert "github.com/QaamGo/api2convert-go/v10"

	"github.com/QaamGo/api2convert-cli/internal/clierr"
	"github.com/QaamGo/api2convert-cli/internal/ui"
)

// TestMergeForwardsMergeOption locks in the fix: run.Merge must send the
// conversion options (notably merge:true) in the create payload, and download
// the single merged output.
func TestMergeForwardsMergeOption(t *testing.T) {
	var createBody string
	fake := &fakeSender{handle: func(req *api2convert.Request) (*api2convert.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL, "/jobs"):
			createBody = string(req.Body)
			return jsonResp(200, `{"id":"m1","status":{"code":"created"},"server":"https://up.example","token":"tok"}`), nil
		case strings.Contains(req.URL, "up.example"):
			return jsonResp(200, `{"id":"in1"}`), nil
		case req.Method == http.MethodPatch && strings.Contains(req.URL, "/jobs/m1"):
			return jsonResp(200, `{"id":"m1","status":{"code":"completed"}}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/m1"):
			return jsonResp(200, `{"id":"m1","status":{"code":"completed"},"output":[{"id":"o","uri":"https://dl.example/merged.pdf","filename":"merged.pdf"}]}`), nil
		case strings.Contains(req.URL, "dl.example"):
			return rawResp(200, []byte("%PDF-merged")), nil
		}
		return jsonResp(404, `{"message":"unexpected `+req.Method+" "+req.URL+`"}`), nil
	}}

	c, err := api2convert.New("k", api2convert.WithHTTPSender(fake))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	_ = os.WriteFile(a, []byte("A"), 0o644)
	_ = os.WriteFile(b, []byte("B"), 0o644)

	res, err := Merge(context.Background(), c, []string{a, b}, "pdf", filepath.Join(dir, "out.pdf"),
		Options{ConversionOptions: map[string]any{"merge": true}, OnConflict: "overwrite"}, ui.NewProgress(nil, false))
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !strings.Contains(createBody, `"merge":true`) {
		t.Fatalf("create payload missing merge:true: %s", createBody)
	}
	got, _ := os.ReadFile(res.Path)
	if string(got) != "%PDF-merged" {
		t.Fatalf("merged content = %q", got)
	}
}

// TestMergeOutputIndexOutOfRange locks in the F-15 fix: an out-of-range
// --output-index must return a usage error, not silently clamp to output 0.
func TestMergeOutputIndexOutOfRange(t *testing.T) {
	fake := &fakeSender{handle: func(req *api2convert.Request) (*api2convert.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL, "/jobs"):
			return jsonResp(200, `{"id":"m1","status":{"code":"created"},"server":"https://up.example","token":"tok"}`), nil
		case strings.Contains(req.URL, "up.example"):
			return jsonResp(200, `{"id":"in1"}`), nil
		case req.Method == http.MethodPatch && strings.Contains(req.URL, "/jobs/m1"):
			return jsonResp(200, `{"id":"m1","status":{"code":"completed"}}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/m1"):
			return jsonResp(200, `{"id":"m1","status":{"code":"completed"},"output":[{"id":"o","uri":"https://dl.example/merged.pdf","filename":"merged.pdf"}]}`), nil
		}
		return jsonResp(404, `{"message":"unexpected"}`), nil
	}}

	c, err := api2convert.New("k", api2convert.WithHTTPSender(fake))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	_ = os.WriteFile(a, []byte("A"), 0o644)
	_ = os.WriteFile(b, []byte("B"), 0o644)

	// Job produces 1 output; index 5 is out of range.
	_, err = Merge(context.Background(), c, []string{a, b}, "pdf", filepath.Join(dir, "out.pdf"),
		Options{OutputIndex: 5, OnConflict: "overwrite"}, ui.NewProgress(nil, false))
	if err == nil {
		t.Fatal("expected an out-of-range --output-index to error, got nil")
	}
	var ue *clierr.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("want *clierr.UsageError, got %T: %v", err, err)
	}
	// And nothing should have been written.
	if _, statErr := os.Stat(filepath.Join(dir, "out.pdf")); !os.IsNotExist(statErr) {
		t.Fatalf("no output should be written on an index error (stat err = %v)", statErr)
	}
}
