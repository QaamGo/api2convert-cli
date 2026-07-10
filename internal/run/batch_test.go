package run

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	api2convert "github.com/QaamGo/api2convert-go/v10"

	"github.com/QaamGo/api2convert-cli/internal/ui"
)

// TestBatchPartialFailure verifies fail-soft aggregation: one input succeeds,
// one fails, and both are recorded (batch never aborts without --fail-fast).
func TestBatchPartialFailure(t *testing.T) {
	fake := &fakeSender{handle: func(req *api2convert.Request) (*api2convert.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL, "/jobs"):
			if strings.Contains(string(req.Body), "bad") {
				return jsonResp(200, `{"id":"jbad","status":{"code":"queued"}}`), nil
			}
			return jsonResp(200, `{"id":"jok","status":{"code":"queued"}}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/jbad"):
			return jsonResp(200, `{"id":"jbad","status":{"code":"failed"},"errors":[{"code":6004,"message":"cannot convert"}]}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/jok"):
			return jsonResp(200, `{"id":"jok","status":{"code":"completed"},"output":[{"uri":"https://dl.example/ok.pdf","filename":"ok.pdf"}]}`), nil
		case strings.Contains(req.URL, "dl.example"):
			return rawResp(200, []byte("%PDF")), nil
		}
		return jsonResp(404, `{"message":"unexpected"}`), nil
	}}

	c, err := api2convert.New("k", api2convert.WithHTTPSender(fake))
	if err != nil {
		t.Fatal(err)
	}
	items := []Item{
		{Input: "https://ok.example/a.txt", Target: "pdf"},
		{Input: "https://bad.example/b.txt", Target: "pdf"},
	}
	sum := BatchItems(context.Background(), c, items, t.TempDir()+"/", 2, Options{OnConflict: "overwrite"}, ui.NewProgress(nil, false), false)

	if len(sum.Results) != 1 || len(sum.Errors) != 1 {
		t.Fatalf("want 1 success + 1 error, got %d/%d", len(sum.Results), len(sum.Errors))
	}
	if sum.Total() != 2 {
		t.Fatalf("total = %d", sum.Total())
	}
	if !strings.Contains(sum.Errors[0].Input, "bad") {
		t.Fatalf("wrong input failed: %s", sum.Errors[0].Input)
	}
}

// TestBatchItemsSortedByInput locks in the F-13 fix: workers finish in
// nondeterministic order, but the summary must be sorted by input so --json
// output is stable and diffable.
func TestBatchItemsSortedByInput(t *testing.T) {
	fake := &fakeSender{handle: func(req *api2convert.Request) (*api2convert.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL, "/jobs"):
			return jsonResp(200, `{"id":"j","status":{"code":"queued"}}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/j"):
			return jsonResp(200, `{"id":"j","status":{"code":"completed"},"output":[{"uri":"https://dl.example/o.pdf","filename":"o.pdf"}]}`), nil
		case strings.Contains(req.URL, "dl.example"):
			return rawResp(200, []byte("%PDF")), nil
		}
		return jsonResp(404, `{"message":"unexpected"}`), nil
	}}

	c, err := api2convert.New("k", api2convert.WithHTTPSender(fake))
	if err != nil {
		t.Fatal(err)
	}
	// Inputs given out of order; rename avoids collisions on the shared basename.
	items := []Item{
		{Input: "https://x/c.txt", Target: "pdf"},
		{Input: "https://x/a.txt", Target: "pdf"},
		{Input: "https://x/b.txt", Target: "pdf"},
	}
	sum := BatchItems(context.Background(), c, items, t.TempDir()+string(os.PathSeparator), 3, Options{OnConflict: "rename"}, ui.NewProgress(nil, false), false)

	if len(sum.Results) != 3 {
		t.Fatalf("want 3 results, got %d (errors=%v)", len(sum.Results), sum.Errors)
	}
	for i := 1; i < len(sum.Results); i++ {
		if sum.Results[i-1].Input > sum.Results[i].Input {
			t.Fatalf("results not sorted by input: %q before %q", sum.Results[i-1].Input, sum.Results[i].Input)
		}
	}
}
