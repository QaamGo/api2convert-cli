package clierr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestClassify(t *testing.T) {
	if _, c := Classify(nil); c != ExitOK {
		t.Error("nil → ok")
	}
	if _, c := Classify(&UsageError{Err: errors.New("bad flag")}); c != ExitUsage {
		t.Error("usage → 2")
	}
	if _, c := Classify(&Coded{Code: ExitQuota}); c != ExitQuota {
		t.Error("coded passes its code")
	}
	if _, c := Classify(context.Canceled); c != ExitInterrupted {
		t.Error("cancel → 130")
	}
	if m, c := Classify(errors.New("boom")); c != ExitGeneric || m != "boom" {
		t.Errorf("generic → 1 with message, got %q/%d", m, c)
	}
}

func TestWriteJSONError(t *testing.T) {
	var buf bytes.Buffer
	WriteJSONError(&buf, errors.New("boom"), ExitGeneric)

	var env struct {
		OK    bool `json:"ok"`
		Error struct {
			Message  string `json:"message"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.OK || env.Error.Message != "boom" || env.Error.ExitCode != int(ExitGeneric) {
		t.Fatalf("bad envelope: %s", buf.String())
	}
}
