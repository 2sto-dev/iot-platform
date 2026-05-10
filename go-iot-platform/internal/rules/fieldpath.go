package rules

import (
	"strconv"
	"strings"
)

// ExtractField extracts a value from a nested map using dot notation.
//
// Supported syntax:
//   - "power_w"                         → top-level field
//   - "relay.state"                     → nested field
//   - "measurements.0.value"            → array index
//   - "measurements[key=active_power_kw].value" → array filter by sub-key
func ExtractField(data map[string]interface{}, path string) interface{} {
	parts := splitPath(path)
	var current interface{} = data
	for _, part := range parts {
		if current == nil {
			return nil
		}
		switch v := current.(type) {
		case map[string]interface{}:
			if strings.Contains(part, "[") {
				current = extractArrayFilter(v, part)
			} else {
				current = v[part]
			}
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil
			}
			current = v[idx]
		default:
			return nil
		}
	}
	return current
}

// splitPath splits "a.b[k=v].c" into ["a", "b[k=v]", "c"].
// Does not split inside brackets.
func splitPath(path string) []string {
	var parts []string
	var buf strings.Builder
	depth := 0
	for _, ch := range path {
		switch ch {
		case '[':
			depth++
			buf.WriteRune(ch)
		case ']':
			depth--
			buf.WriteRune(ch)
		case '.':
			if depth == 0 {
				parts = append(parts, buf.String())
				buf.Reset()
			} else {
				buf.WriteRune(ch)
			}
		default:
			buf.WriteRune(ch)
		}
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

// extractArrayFilter handles "measurements[key=active_power_kw]".
func extractArrayFilter(m map[string]interface{}, part string) interface{} {
	bracketIdx := strings.Index(part, "[")
	arrKey := part[:bracketIdx]
	filter := strings.TrimSuffix(part[bracketIdx+1:], "]")

	arr, ok := m[arrKey].([]interface{})
	if !ok {
		return nil
	}

	// filter format: "key=value"
	eqIdx := strings.Index(filter, "=")
	if eqIdx < 0 {
		// numeric fallback: measurements[0]
		idx, err := strconv.Atoi(filter)
		if err != nil || idx < 0 || idx >= len(arr) {
			return nil
		}
		return arr[idx]
	}

	filterKey := filter[:eqIdx]
	filterVal := filter[eqIdx+1:]

	for _, elem := range arr {
		if em, ok := elem.(map[string]interface{}); ok {
			if v, ok := em[filterKey]; ok {
				if toString(v) == filterVal {
					return em
				}
			}
		}
	}
	return nil
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}
