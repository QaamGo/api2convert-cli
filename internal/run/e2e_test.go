package run

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	api2convert "github.com/QaamGo/api2convert-go/v10"

	"github.com/QaamGo/api2convert-cli/internal/ui"
)

// fakeSender routes SDK requests to canned responses, exercising the full
// create → poll → download lifecycle offline.
type fakeSender struct {
	handle func(req *api2convert.Request) (*api2convert.Response, error)
}

func (f *fakeSender) Send(_ context.Context, req *api2convert.Request) (*api2convert.Response, error) {
	return f.handle(req)
}

func jsonResp(status int, body string) *api2convert.Response {
	return &api2convert.Response{
		Status: status,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(body)),
	}
}

func rawResp(status int, body []byte) *api2convert.Response {
	return &api2convert.Response{
		Status: status,
		Header: http.Header{},
		Body:   io.NopCloser(bytes.NewReader(body)),
	}
}

func TestConvertURLEndToEnd(t *testing.T) {
	const payload = "PNG-BYTES"
	fake := &fakeSender{handle: func(req *api2convert.Request) (*api2convert.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL, "/jobs"):
			return jsonResp(200, `{"id":"job1","status":{"code":"queued"}}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/job1"):
			return jsonResp(200, `{"id":"job1","status":{"code":"completed"},"output":[{"id":"o1","uri":"https://dl.example/out.png","filename":"out.png"}]}`), nil
		case strings.Contains(req.URL, "dl.example"):
			return rawResp(200, []byte(payload)), nil
		}
		return jsonResp(404, `{"message":"unexpected `+req.Method+" "+req.URL+`"}`), nil
	}}

	c, err := api2convert.New("testkey", api2convert.WithHTTPSender(fake))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	res, err := ConvertOne(context.Background(), c, "https://example.com/a.jpg", "png", dir+string(os.PathSeparator), Options{OnConflict: "overwrite"}, ui.NewProgress(nil, false))
	if err != nil {
		t.Fatalf("ConvertOne: %v", err)
	}

	want := filepath.Join(dir, "out.png")
	if res.Path != want {
		t.Fatalf("path = %q, want %q", res.Path, want)
	}
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("content = %q, want %q", got, payload)
	}
}

// TestConvertOneCleansPlaceholderOnSaveFailure locks in the F-04 fix: when the
// download (res.Save) fails, the empty O_EXCL claim placeholder must be removed,
// not stranded — otherwise a later run treats it as "already done" (skip) or as a
// conflict (error).
func TestConvertOneCleansPlaceholderOnSaveFailure(t *testing.T) {
	fake := &fakeSender{handle: func(req *api2convert.Request) (*api2convert.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL, "/jobs"):
			return jsonResp(200, `{"id":"j1","status":{"code":"queued"}}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/j1"):
			return jsonResp(200, `{"id":"j1","status":{"code":"completed"},"output":[{"uri":"https://dl.example/out.png","filename":"out.png"}]}`), nil
		case strings.Contains(req.URL, "dl.example"):
			return jsonResp(500, `{"message":"download boom"}`), nil // the download fails
		}
		return jsonResp(404, `{"message":"unexpected"}`), nil
	}}

	c, err := api2convert.New("k", api2convert.WithHTTPSender(fake))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// A non-overwrite policy claims the path up front (creates the placeholder).
	_, cerr := ConvertOne(context.Background(), c, "https://example.com/a.jpg", "png", dir+string(os.PathSeparator), Options{OnConflict: "error"}, ui.NewProgress(nil, false))
	if cerr == nil {
		t.Fatal("expected the failed download to surface an error")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out.png")); !os.IsNotExist(statErr) {
		t.Fatalf("claim placeholder was not cleaned up after Save failure (stat err = %v)", statErr)
	}
}
