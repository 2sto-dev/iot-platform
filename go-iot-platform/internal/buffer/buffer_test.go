package buffer

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendAndReadBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "buffer.log")

	b, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := b.Append("tenants/1/devices/d1/up/state", []byte(`{"power":"ON"}`), errors.New("influx down")); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := b.Append("shellies/abc/emeter/0/power", []byte(`123.4`), errors.New("timeout")); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("unmarshal line %d: %v", count, err)
		}
		if e.Topic == "" || e.Payload == "" || e.Error == "" {
			t.Errorf("entry has empty fields: %+v", e)
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 lines, got %d", count)
	}
}

func TestNilReceiverIsSafe(t *testing.T) {
	var b *FileBuffer
	if err := b.Append("t", []byte("p"), errors.New("e")); err != nil {
		t.Errorf("nil append should be no-op, got: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Errorf("nil close should be no-op, got: %v", err)
	}
}
