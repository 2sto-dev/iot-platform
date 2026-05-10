package registry

import (
	"fmt"
	"regexp"
	"strings"
)

// Validate verifies that a single DeviceDefinition is structurally well-formed.
// Returns the first violation found (fail-fast); aggregated check is in ValidateAll.
//
// Reguli aplicate:
//
//   - schema_version == CurrentSchemaVersion
//   - id non-empty, lowercase, doar [a-z0-9_]
//   - name non-empty
//   - protocol in SupportedProtocols (and enabled)
//   - identification.topic_match non-empty cu min 1 pattern valid
//   - parser.type in SupportedParserTypes
//   - capabilities non-empty
//   - normalized_fields[*].source non-empty când e prezent
//   - commands[*].topic / payload non-empty
func (dd *DeviceDefinition) Validate() error {
	if dd.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("schema_version %q unsupported (current: %q)",
			dd.SchemaVersion, CurrentSchemaVersion)
	}

	if dd.ID == "" {
		return fmt.Errorf("id required")
	}
	if !idPattern.MatchString(dd.ID) {
		return fmt.Errorf("id %q invalid (must match %s)", dd.ID, idPattern.String())
	}

	if dd.Name == "" {
		return fmt.Errorf("name required")
	}

	if dd.Protocol == "" {
		return fmt.Errorf("protocol required")
	}
	enabled, known := SupportedProtocols[dd.Protocol]
	if !known {
		return fmt.Errorf("protocol %q unknown", dd.Protocol)
	}
	if !enabled {
		return fmt.Errorf("protocol %q known but not yet implemented (planned for later phase)", dd.Protocol)
	}

	if len(dd.Identification.TopicMatch) == 0 {
		return fmt.Errorf("identification.topic_match required (at least 1 pattern)")
	}
	for i, tm := range dd.Identification.TopicMatch {
		if err := validateTopicMatch(tm); err != nil {
			return fmt.Errorf("identification.topic_match[%d]: %w", i, err)
		}
	}

	if dd.Parser.Type == "" {
		return fmt.Errorf("parser.type required")
	}
	if !SupportedParserTypes[dd.Parser.Type] {
		return fmt.Errorf("parser.type %q unknown (valid: %v)",
			dd.Parser.Type, parserTypesList())
	}
	if dd.Parser.Type == "json_with_measurements_array" && dd.Parser.PayloadPath == "" {
		return fmt.Errorf("parser.payload_path required for json_with_measurements_array")
	}

	if len(dd.Capabilities) == 0 {
		return fmt.Errorf("capabilities required (at least 1)")
	}

	for k, nf := range dd.NormalizedFields {
		if nf.Source == "" {
			return fmt.Errorf("normalized_fields[%s].source required", k)
		}
	}

	for cmdName, cmd := range dd.Commands {
		if cmd.Topic == "" {
			return fmt.Errorf("commands[%s].topic required", cmdName)
		}
		// Payload poate fi empty string explicit (ex: cmnd/.../State fără payload)
		// dar topic e mereu obligatoriu.
	}

	return nil
}

// idPattern — id-ul DD-ului trebuie să fie kebab/snake_case strict.
// Regex: începe cu literă, apoi litere mici / cifre / underscore.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// validateTopicMatch verifică un singur pattern.
// Pentru regex (prefix `~`), încearcă compilarea.
// Pentru wildcard MQTT, doar verifică că pattern e non-empty.
func validateTopicMatch(tm TopicMatchSpec) error {
	if tm.Pattern == "" {
		return fmt.Errorf("pattern required")
	}
	if strings.HasPrefix(tm.Pattern, "~") {
		// Regex pattern; compile to validate
		raw := strings.TrimPrefix(tm.Pattern, "~")
		if _, err := regexp.Compile(raw); err != nil {
			return fmt.Errorf("invalid regex %q: %w", raw, err)
		}
	}
	// MQTT wildcard validation:
	// `+` and `#` valid; reject if `#` is not at end or has chars after
	parts := strings.Split(tm.Pattern, "/")
	for i, p := range parts {
		if strings.Contains(p, "#") && p != "#" {
			return fmt.Errorf("invalid MQTT wildcard `#` mixed with chars in segment %q", p)
		}
		if p == "#" && i != len(parts)-1 {
			return fmt.Errorf("`#` wildcard must be the last segment")
		}
	}
	return nil
}

// parserTypesList — helper pentru error messages.
func parserTypesList() []string {
	out := make([]string, 0, len(SupportedParserTypes))
	for k := range SupportedParserTypes {
		out = append(out, k)
	}
	return out
}
