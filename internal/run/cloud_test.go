package run

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	api2convert "github.com/QaamGo/api2convert-go/v10"

	"github.com/QaamGo/api2convert-cli/internal/ui"
)

// TestPrepareInputCloudMapping locks in that a run.CloudSource maps to the SDK's
// typed CloudInput with the correct provider, parameters and credentials.
func TestPrepareInputCloudMapping(t *testing.T) {
	o := Options{CloudInput: &CloudSource{
		Provider:    "amazons3",
		Parameters:  map[string]any{"bucket": "my-bucket", "file": "in.docx"},
		Credentials: map[string]any{"accesskeyid": "AK", "secretaccesskey": "SK"},
	}}

	var copts []api2convert.ConvertOption
	in, err := prepareInput(&copts, "cloud:amazons3", "", "pdf", o)
	if err != nil {
		t.Fatalf("prepareInput: %v", err)
	}
	ci, ok := in.(api2convert.CloudInput)
	if !ok {
		t.Fatalf("input is %T, want api2convert.CloudInput", in)
	}
	d := ci.Descriptor()
	if d["type"] != "cloud" {
		t.Fatalf("type = %v, want cloud", d["type"])
	}
	if d["source"] != "amazons3" {
		t.Fatalf("source = %v, want amazons3", d["source"])
	}
	if !reflect.DeepEqual(d["parameters"], map[string]any{"bucket": "my-bucket", "file": "in.docx"}) {
		t.Fatalf("parameters = %v", d["parameters"])
	}
	if !reflect.DeepEqual(d["credentials"], map[string]any{"accesskeyid": "AK", "secretaccesskey": "SK"}) {
		t.Fatalf("credentials = %v", d["credentials"])
	}
}

// TestCloudOutputSkipsLocalSave verifies that when an output target is set the
// conversion delivers to cloud storage and the CLI never attempts a local
// download/save — and that the output target (provider + params + creds) is sent
// on the wire.
func TestCloudOutputSkipsLocalSave(t *testing.T) {
	var createBody []byte
	fake := &fakeSender{handle: func(req *api2convert.Request) (*api2convert.Response, error) {
		switch {
		case req.Method == http.MethodPost && strings.HasSuffix(req.URL, "/jobs"):
			createBody = req.Body
			return jsonResp(200, `{"id":"cj","status":{"code":"queued"}}`), nil
		case req.Method == http.MethodGet && strings.Contains(req.URL, "/jobs/cj"):
			return jsonResp(200, `{"id":"cj","status":{"code":"completed"},"conversion":[{"target":"pdf","output_target":[{"type":"amazons3","status":"completed"}]}]}`), nil
		case strings.Contains(req.URL, "dl.example"):
			// A cloud-delivered job has no local output; the CLI must never try to
			// download one.
			t.Fatalf("cloud output must not trigger a local download: %s", req.URL)
		}
		return jsonResp(404, `{"message":"unexpected `+req.Method+" "+req.URL+`"}`), nil
	}}

	c, err := api2convert.New("k", api2convert.WithHTTPSender(fake))
	if err != nil {
		t.Fatal(err)
	}

	o := Options{
		OnConflict: "overwrite",
		OutputTargets: []CloudTarget{{
			Provider:    "amazons3",
			Parameters:  map[string]any{"bucket": "out-bucket"},
			Credentials: map[string]any{"accesskeyid": "AK", "secretaccesskey": "SK"},
		}},
	}
	// A URL input keeps the test purely offline (no upload); the delivery target
	// is what exercises the cloud-output path.
	res, err := ConvertOne(context.Background(), c, "https://example.com/a.docx", "pdf", "", o, ui.NewProgress(nil, false))
	if err != nil {
		t.Fatalf("ConvertOne: %v", err)
	}
	if !res.Cloud {
		t.Fatalf("expected Cloud result, got %+v", res)
	}
	if res.Path != "" {
		t.Fatalf("cloud result must have no local path, got %q", res.Path)
	}
	if len(res.Deliveries) != 1 || res.Deliveries[0].Provider != "amazons3" || res.Deliveries[0].Status != "completed" {
		t.Fatalf("deliveries = %+v", res.Deliveries)
	}

	// The output target must have been sent on create with its full mapping.
	var payload map[string]any
	if err := json.Unmarshal(createBody, &payload); err != nil {
		t.Fatalf("create body: %v (body=%s)", err, createBody)
	}
	conv := payload["conversion"].([]any)[0].(map[string]any)
	tgt := conv["output_target"].([]any)[0].(map[string]any)
	if tgt["type"] != "amazons3" {
		t.Fatalf("target type = %v", tgt["type"])
	}
	if !reflect.DeepEqual(tgt["parameters"], map[string]any{"bucket": "out-bucket"}) {
		t.Fatalf("target parameters = %v", tgt["parameters"])
	}
	if !reflect.DeepEqual(tgt["credentials"], map[string]any{"accesskeyid": "AK", "secretaccesskey": "SK"}) {
		t.Fatalf("target credentials = %v", tgt["credentials"])
	}
}
