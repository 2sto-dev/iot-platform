package matcher

import (
	"os"
	"testing"

	"go-iot-platform/internal/registry"
	"gopkg.in/yaml.v3"
)

// writeYAML serializează un DD la YAML pe disk pentru test fixtures.
// Folosește yaml.v3 (același ca loader-ul) ca să garanteze round-trip fidel.
//
// Parametrul `_idx` nu e folosit currently; rezervat pentru ordering în viitoare
// teste cu multiple DD-uri în același director.
func writeYAML(t *testing.T, path string, dd *registry.DeviceDefinition, _idx int) {
	t.Helper()
	bytes, err := yaml.Marshal(dd)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	if err := os.WriteFile(path, bytes, 0644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
