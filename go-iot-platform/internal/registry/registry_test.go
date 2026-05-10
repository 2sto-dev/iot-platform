package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validYAML — happy path DD with all required fields.
const validYAML = `
schema_version: "1.0"
id: test_device
name: "Test Device"
vendor: testco
protocol: mqtt
identification:
  topic_match:
    - pattern: "tele/+/STATE"
      extract:
        device_id: "$1"
parser:
  type: json
capabilities:
  - relay
`

func writeTempYAML(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return dir
}

func TestLoadValid(t *testing.T) {
	dir := writeTempYAML(t, "test_device.yaml", validYAML)
	reg, errs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected non-fatal errors: %v", errs)
	}
	if reg.Count() != 1 {
		t.Fatalf("expected 1 def loaded, got %d", reg.Count())
	}
	dd := reg.Get("test_device")
	if dd == nil {
		t.Fatal("device not found by ID")
	}
	if dd.Name != "Test Device" || dd.Vendor != "testco" {
		t.Errorf("fields not parsed correctly: %+v", dd)
	}
	if dd.SourcePath == "" {
		t.Error("SourcePath not populated")
	}
}

func TestLoadMissingDir(t *testing.T) {
	_, _, err := LoadDir("/nonexistent/path/blah")
	if err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestRejectMalformedYAML(t *testing.T) {
	dir := writeTempYAML(t, "bad.yaml", "this is: not: valid: yaml: at all: [unbalanced")
	reg, errs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir should not fatal on per-file error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected non-fatal error for malformed yaml")
	}
	if reg.Count() != 0 {
		t.Fatalf("expected 0 defs (file rejected), got %d", reg.Count())
	}
}

func TestRejectUnsupportedSchemaVersion(t *testing.T) {
	bad := strings.Replace(validYAML, `schema_version: "1.0"`, `schema_version: "0.5"`, 1)
	dir := writeTempYAML(t, "v05.yaml", bad)
	_, errs, _ := LoadDir(dir)
	if len(errs) == 0 {
		t.Fatal("expected error for unsupported schema_version")
	}
	if !strings.Contains(errs[0].Error(), "schema_version") {
		t.Errorf("error doesn't mention schema_version: %v", errs[0])
	}
}

func TestRejectMissingRequiredFields(t *testing.T) {
	cases := map[string]string{
		"missing id":             strings.Replace(validYAML, "id: test_device", "", 1),
		"missing name":           strings.Replace(validYAML, `name: "Test Device"`, "", 1),
		"missing protocol":       strings.Replace(validYAML, "protocol: mqtt", "", 1),
		"missing identification": strings.Replace(validYAML, "identification:\n  topic_match:\n    - pattern: \"tele/+/STATE\"\n      extract:\n        device_id: \"$1\"\n", "", 1),
		"missing parser":         strings.Replace(validYAML, "parser:\n  type: json\n", "", 1),
		"missing capabilities":   strings.Replace(validYAML, "capabilities:\n  - relay\n", "", 1),
	}
	for name, yamlContent := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeTempYAML(t, "missing.yaml", yamlContent)
			_, errs, _ := LoadDir(dir)
			if len(errs) == 0 {
				t.Fatalf("expected validation error for %q", name)
			}
		})
	}
}

func TestRejectInvalidID(t *testing.T) {
	cases := []string{"Test_Device", "test-device", "test device", "TEST", "1test", ""}
	for _, badID := range cases {
		t.Run(badID, func(t *testing.T) {
			y := strings.Replace(validYAML, "id: test_device", "id: \""+badID+"\"", 1)
			dir := writeTempYAML(t, "badid.yaml", y)
			_, errs, _ := LoadDir(dir)
			if len(errs) == 0 {
				t.Errorf("expected error for invalid id %q", badID)
			}
		})
	}
}

func TestRejectUnknownProtocol(t *testing.T) {
	y := strings.Replace(validYAML, "protocol: mqtt", "protocol: bacnet", 1)
	dir := writeTempYAML(t, "bad-proto.yaml", y)
	_, errs, _ := LoadDir(dir)
	if len(errs) == 0 {
		t.Fatal("expected error for unknown protocol")
	}
}

func TestRejectUnsupportedParserType(t *testing.T) {
	y := strings.Replace(validYAML, "parser:\n  type: json", "parser:\n  type: protobuf", 1)
	dir := writeTempYAML(t, "bad-parser.yaml", y)
	_, errs, _ := LoadDir(dir)
	if len(errs) == 0 {
		t.Fatal("expected error for unsupported parser.type")
	}
}

func TestRejectInvalidRegexInTopicMatch(t *testing.T) {
	y := strings.Replace(validYAML,
		`pattern: "tele/+/STATE"`,
		`pattern: "~^[unclosed"`, 1)
	dir := writeTempYAML(t, "bad-regex.yaml", y)
	_, errs, _ := LoadDir(dir)
	if len(errs) == 0 {
		t.Fatal("expected error for invalid regex in identification.topic_match")
	}
}

func TestRejectInvalidWildcardPlacement(t *testing.T) {
	// `#` must be the last segment in MQTT wildcards
	y := strings.Replace(validYAML,
		`pattern: "tele/+/STATE"`,
		`pattern: "foo/#/bar"`, 1)
	dir := writeTempYAML(t, "bad-hash.yaml", y)
	_, errs, _ := LoadDir(dir)
	if len(errs) == 0 {
		t.Fatal("expected error for invalid `#` wildcard placement")
	}
}

func TestRejectDuplicateID(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(validYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(validYAML), 0644); err != nil {
		t.Fatal(err)
	}
	reg, errs, _ := LoadDir(dir)
	if reg.Count() != 1 {
		t.Errorf("expected only 1 def loaded (dup rejected), got %d", reg.Count())
	}
	if len(errs) == 0 {
		t.Fatal("expected duplicate-id error")
	}
	dupErr := errs[0].Error()
	if !strings.Contains(dupErr, "duplicate") {
		t.Errorf("error doesn't mention duplicate: %v", dupErr)
	}
}

func TestRejectUnknownYAMLFields(t *testing.T) {
	// KnownFields(true) → typo-uri în YAML rejection.
	y := validYAML + "\nbogus_field: 42\n"
	dir := writeTempYAML(t, "typo.yaml", y)
	_, errs, _ := LoadDir(dir)
	if len(errs) == 0 {
		t.Fatal("expected error for unknown YAML field")
	}
}

func TestSkipUnderscoreFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_schema.yaml"), []byte("not-a-dd"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "valid.yaml"), []byte(validYAML), 0644); err != nil {
		t.Fatal(err)
	}
	reg, errs, _ := LoadDir(dir)
	if reg.Count() != 1 {
		t.Errorf("expected 1 def loaded (underscore skipped), got %d", reg.Count())
	}
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestSkipNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("docs"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "valid.yaml"), []byte(validYAML), 0644); err != nil {
		t.Fatal(err)
	}
	reg, _, _ := LoadDir(dir)
	if reg.Count() != 1 {
		t.Errorf("expected 1 def loaded (.md skipped), got %d", reg.Count())
	}
}

func TestByCapability(t *testing.T) {
	dir := t.TempDir()
	yaml1 := validYAML // capabilities: [relay]
	yaml2 := strings.Replace(validYAML, "id: test_device", "id: test_device2", 1)
	yaml2 = strings.Replace(yaml2, "capabilities:\n  - relay", "capabilities:\n  - relay\n  - power_meter", 1)

	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(yaml1), 0644)
	os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(yaml2), 0644)

	reg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	relays := reg.ByCapability("relay")
	if len(relays) != 2 {
		t.Errorf("expected 2 devices with capability=relay, got %d", len(relays))
	}
	meters := reg.ByCapability("power_meter")
	if len(meters) != 1 {
		t.Errorf("expected 1 device with capability=power_meter, got %d", len(meters))
	}
	missing := reg.ByCapability("nonexistent")
	if len(missing) != 0 {
		t.Errorf("expected 0 for unknown capability, got %d", len(missing))
	}
}

// TestLoadProductionConfigs verifică că cele 4 YAML-uri reale din configs/devices/
// se încarcă fără erori — guard contra regression.
func TestLoadProductionConfigs(t *testing.T) {
	// Calea relativă de la go-iot-platform/internal/registry/ la repo root.
	prodDir := filepath.Join("..", "..", "..", "configs", "devices")
	if _, err := os.Stat(prodDir); os.IsNotExist(err) {
		t.Skip("configs/devices/ not present in test env")
	}
	reg, errs, err := LoadDir(prodDir)
	if err != nil {
		t.Fatalf("LoadDir on production configs failed: %v", err)
	}
	if len(errs) > 0 {
		t.Fatalf("production configs have errors:\n%v", errs)
	}
	if reg.Count() < 4 {
		t.Errorf("expected ≥4 production DD-uri, got %d", reg.Count())
	}

	// Spot-checks pe ID-urile cunoscute
	for _, id := range []string{"huawei_sun2000_3phase", "nous_a1t", "shelly_em", "zigbee_temperature"} {
		if reg.Get(id) == nil {
			t.Errorf("expected DD %q to be loaded", id)
		}
	}
}
