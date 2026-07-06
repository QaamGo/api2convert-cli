package run

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api2convert "github.com/QaamGo/api2convert-go/v10"
)

// TestWatchConvertsDroppedFile verifies the fsnotify → convert wiring: a file
// created after the watcher starts is converted into the out dir.
func TestWatchConvertsDroppedFile(t *testing.T) {
	fake := &fakeSender{handle: func(req *api2convert.Request) (*api2convert.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL, "/jobs"):
			return jsonResp(200, `{"id":"w1","status":{"code":"created"},"server":"https://up.example","token":"t"}`), nil
		case strings.Contains(req.URL, "up.example"):
			return jsonResp(200, `{"id":"in1"}`), nil
		case req.Method == http.MethodPatch && strings.Contains(req.URL, "/jobs/w1"):
			return jsonResp(200, `{"id":"w1","status":{"code":"completed"}}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/w1"):
			return jsonResp(200, `{"id":"w1","status":{"code":"completed"},"output":[{"uri":"https://dl.example/o.pdf","filename":"o.pdf"}]}`), nil
		case strings.Contains(req.URL, "dl.example"):
			return rawResp(200, []byte("%PDF")), nil
		}
		return jsonResp(404, `{"message":"unexpected"}`), nil
	}}

	c, err := api2convert.New("k", api2convert.WithHTTPSender(fake))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	outdir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan Result, 4)
	go func() {
		_ = Watch(ctx, c, WatchConfig{
			Dir: dir, Target: "pdf", OutDir: outdir,
			Options: Options{OnConflict: "overwrite"},
		}, func(r Result, e error) {
			if e == nil {
				results <- r
			}
		})
	}()

	// Let the watcher register, then drop a file.
	time.Sleep(300 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(dir, "drop.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-results:
		if r.Path == "" {
			t.Fatal("watch produced an empty output path")
		}
		if _, err := os.Stat(r.Path); err != nil {
			t.Fatalf("output not written: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("watch did not convert the dropped file within 6s")
	}
}
