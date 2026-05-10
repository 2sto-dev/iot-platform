package registry

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadDir scans `dir` for *.yaml / *.yml files and loads each as a DeviceDefinition.
//
// Returnează:
//   - registry populat cu toate DD-urile valide
//   - lista erorilor non-fatale (per fișier — fișiere malformate sunt skip-uite,
//     dar continuăm încărcarea celorlalte)
//   - eroare fatală dacă directorul nu există sau nu poate fi citit
//
// Pentru CI / startup strict: caller-ul poate inspecta erorile și să oprească
// procesul dacă > 0 (recomandat în production via `MUST_LOAD_ALL_DD=true`).
func LoadDir(dir string) (*Registry, []error, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("registry: stat %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("registry: %q is not a directory", dir)
	}

	reg := NewRegistry()
	var errs []error

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// Skip metafiles (ex: _schema.json) și non-YAML
		base := d.Name()
		if strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".") {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(base))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		dd, err := loadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			return nil // continuă cu celelalte fișiere
		}

		if existing := reg.Get(dd.ID); existing != nil {
			errs = append(errs, fmt.Errorf("%s: duplicate id %q (already loaded from %s)",
				path, dd.ID, existing.SourcePath))
			return nil
		}

		reg.defs[dd.ID] = dd
		return nil
	})

	if walkErr != nil {
		return reg, errs, fmt.Errorf("registry: walk %q: %w", dir, walkErr)
	}

	return reg, errs, nil
}

// loadFile parses a single YAML file into a DeviceDefinition + validates it.
func loadFile(path string) (*DeviceDefinition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var dd DeviceDefinition
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true) // reject unknown fields → catch typos early
	if err := dec.Decode(&dd); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}

	dd.SourcePath = path
	dd.LoadedAt = time.Now().UTC()

	if err := dd.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	return &dd, nil
}

// LoadDirOrLog wraps LoadDir cu logging structured.
// Folosit la startup în cmd/main.go ca să nu duplicăm boilerplate.
//
// Comportament:
//   - dacă dir nu există → log warn + returnează registry gol (nu fatal pt dev)
//   - dacă unele fișiere eșuează validation → log warn per fișier + continuă
//   - dacă requireNonEmpty == true și 0 DD-uri → eroare returnată
func LoadDirOrLog(dir string, requireNonEmpty bool) (*Registry, error) {
	reg, errs, err := LoadDir(dir)
	if err != nil {
		log.Printf("registry: load %q failed: %v", dir, err)
		if requireNonEmpty {
			return nil, err
		}
		return NewRegistry(), nil
	}

	for _, e := range errs {
		log.Printf("registry: WARN %v", e)
	}

	count := reg.Count()
	log.Printf("registry: loaded %d device definition(s) from %s (errors: %d)",
		count, dir, len(errs))

	if requireNonEmpty && count == 0 {
		return nil, fmt.Errorf("registry: no device definitions loaded from %q", dir)
	}

	return reg, nil
}
