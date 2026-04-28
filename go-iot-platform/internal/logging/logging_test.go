package logging

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
)

func TestEmitJSON(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(log.New(&buf, "", 0))

	Drop("device-tenant mismatch", Fields{
		"device_id":     "dev-001",
		"topic_tenant":  1,
		"device_tenant": 2,
	})

	out := strings.TrimSpace(buf.String())
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, out)
	}
	for _, key := range []string{"ts", "level", "msg", "device_id", "topic_tenant", "device_tenant"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing key %q in output: %v", key, got)
		}
	}
	if got["level"] != "drop" {
		t.Errorf("level = %v, want drop", got["level"])
	}
	if got["msg"] != "device-tenant mismatch" {
		t.Errorf("msg = %v", got["msg"])
	}
}

func TestNilFieldsIsSafe(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(log.New(&buf, "", 0))
	Info("hello", nil)
	out := strings.TrimSpace(buf.String())
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if got["msg"] != "hello" || got["level"] != "info" {
		t.Errorf("unexpected output: %v", got)
	}
}
