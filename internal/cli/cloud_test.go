package cli

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	api2convert "github.com/QaamGo/api2convert-go/v10"
)

// TestBuildCloudInputMapping verifies the --input-* flags map to a run.CloudSource
// with the right provider/params/creds, that credentials load from the env var,
// and that output-only providers are rejected as an input.
func TestBuildCloudInputMapping(t *testing.T) {
	t.Setenv("A2C_INPUT_CREDENTIALS", `{"accesskeyid":"AK","secretaccesskey":"SK"}`)

	f := convertFlags{
		inputCloud:  "amazons3",
		inputParams: []string{"bucket=my-bucket", "file=in.docx"},
	}
	src, err := buildCloudInput(f)
	if err != nil {
		t.Fatalf("buildCloudInput: %v", err)
	}
	if src == nil {
		t.Fatal("expected a cloud source")
	}
	if src.Provider != "amazons3" {
		t.Fatalf("provider = %q", src.Provider)
	}
	if !reflect.DeepEqual(src.Parameters, map[string]any{"bucket": "my-bucket", "file": "in.docx"}) {
		t.Fatalf("parameters = %v", src.Parameters)
	}
	if !reflect.DeepEqual(src.Credentials, map[string]any{"accesskeyid": "AK", "secretaccesskey": "SK"}) {
		t.Fatalf("credentials = %v", src.Credentials)
	}

	// gdrive / youtube are output-only and must be rejected as an input.
	for _, p := range []string{"gdrive", "youtube"} {
		if _, err := buildCloudInput(convertFlags{inputCloud: p}); err == nil {
			t.Fatalf("%s should be rejected as an input", p)
		}
	}
	// unknown provider
	if _, err := buildCloudInput(convertFlags{inputCloud: "dropbox"}); err == nil {
		t.Fatal("unknown provider should be rejected")
	}
	// no cloud input flag → nil, no error
	if src, err := buildCloudInput(convertFlags{}); err != nil || src != nil {
		t.Fatalf("empty: got (%v,%v)", src, err)
	}
}

// TestBuildOutputTargetsMapping verifies the --output-* flags map to a
// run.CloudTarget with the right provider/params/creds sourced from the env var.
func TestBuildOutputTargetsMapping(t *testing.T) {
	t.Setenv("A2C_OUTPUT_CREDENTIALS", `{"accesskeyid":"AK","secretaccesskey":"SK"}`)

	f := convertFlags{
		outputTarget: "amazons3",
		outputParams: []string{"bucket=out-bucket"},
	}
	tgts, err := buildOutputTargets(f)
	if err != nil {
		t.Fatalf("buildOutputTargets: %v", err)
	}
	if len(tgts) != 1 {
		t.Fatalf("targets = %d, want 1", len(tgts))
	}
	tg := tgts[0]
	if tg.Provider != "amazons3" {
		t.Fatalf("provider = %q", tg.Provider)
	}
	if !reflect.DeepEqual(tg.Parameters, map[string]any{"bucket": "out-bucket"}) {
		t.Fatalf("parameters = %v", tg.Parameters)
	}
	if !reflect.DeepEqual(tg.Credentials, map[string]any{"accesskeyid": "AK", "secretaccesskey": "SK"}) {
		t.Fatalf("credentials = %v", tg.Credentials)
	}

	// gdrive/youtube ARE valid output targets.
	for _, p := range []string{"gdrive", "youtube", "azure", "ftp", "googlecloud"} {
		if _, err := buildOutputTargets(convertFlags{outputTarget: p}); err != nil {
			t.Fatalf("%s should be a valid output target: %v", p, err)
		}
	}
	// params without a target is a usage error.
	if _, err := buildOutputTargets(convertFlags{outputParams: []string{"bucket=x"}}); err == nil {
		t.Fatal("params without --output-target should error")
	}
}

// TestParseCloudParamsRejectsMalformed ensures a non key=value param errors.
func TestParseCloudParamsRejectsMalformed(t *testing.T) {
	if _, err := parseCloudParams([]string{"bucket"}, "--input-param"); err == nil {
		t.Fatal("expected error for missing '='")
	}
	if _, err := parseCloudParams([]string{"=v"}, "--input-param"); err == nil {
		t.Fatal("expected error for empty key")
	}
}

// TestJobDetailViewOmitsCredentials is the security regression: an output target
// carrying credentials must never surface them in Human or JSON output, while the
// non-secret provider/status still render.
func TestJobDetailViewOmitsCredentials(t *testing.T) {
	const secret = "SUPER-SECRET-KEY"
	job := api2convert.Job{
		ID:     "j1",
		Status: api2convert.Status{Code: "completed"},
		Conversion: []api2convert.Conversion{{
			Target: "pdf",
			OutputTargets: []api2convert.OutputTarget{{
				Type:        "amazons3",
				Status:      "completed",
				Parameters:  map[string]any{"bucket": "out-bucket"},
				Credentials: map[string]any{"secretaccesskey": secret},
			}},
		}},
	}
	v := jobDetailView{job: job}

	var buf bytes.Buffer
	if err := v.Human(&buf); err != nil {
		t.Fatalf("Human: %v", err)
	}
	human := buf.String()
	if strings.Contains(human, secret) {
		t.Fatalf("Human output leaked credentials:\n%s", human)
	}
	if !strings.Contains(human, "amazons3") || !strings.Contains(human, "completed") {
		t.Fatalf("Human output should show provider + status:\n%s", human)
	}

	b, err := json.Marshal(v.JSON())
	if err != nil {
		t.Fatalf("JSON marshal: %v", err)
	}
	js := string(b)
	if strings.Contains(js, secret) {
		t.Fatalf("JSON output leaked credentials:\n%s", js)
	}
	if strings.Contains(js, "credentials") {
		t.Fatalf("JSON output must not contain a credentials field:\n%s", js)
	}
	if !strings.Contains(js, "amazons3") {
		t.Fatalf("JSON output should show provider:\n%s", js)
	}
}
