// Package matcher routes MQTT topics la Device Definitions încărcate din registry.
//
// Înlocuiește lanțul `strings.Contains` / `strings.HasSuffix` din cmd/main.go cu
// un mecanism declarativ bazat pe patterns YAML.
//
// Vezi: docs/adr/ADR-002-topic-matcher.md
package matcher

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go-iot-platform/internal/registry"
)

// Match — rezultatul unei interogări matcher.Match().
//
// Definition — DD-ul al cărui topic_match a prins
// Pattern    — pattern-ul care a prins (pentru debug / log)
// Stream     — câmpul `stream:` din TopicMatchSpec (telemetry/state/sensor/cmd_ack/...)
// Extracted  — variabilele extrase per `extract:` mapping
type Match struct {
	Definition *registry.DeviceDefinition
	Pattern    string
	Stream     string
	Extracted  map[string]string
}

// Matcher — engine compilat dintr-un Registry; thread-safe pentru read after compile.
type Matcher struct {
	patterns []compiled
}

// compiled — un pattern pre-compilat la New(). Stocăm regex-ul ca să nu re-compilăm
// la fiecare match (perf-critical pe ingest hot path).
type compiled struct {
	dd      *registry.DeviceDefinition
	spec    *registry.TopicMatchSpec
	regex   *regexp.Regexp
	mqttPos []string // dacă pattern-ul e MQTT-stil, nume sintetice "$1","$2"... pentru fiecare `+`
}

// New construiește un Matcher peste Registry-ul dat. Toate patterns sunt
// compile-uite o singură dată; erorile de compilare sunt agregate și returnate.
//
// Comportament:
//   - dacă compilarea unui pattern eșuează, pattern-ul e SKIP-uit (nu blocăm întregul matcher),
//     dar eroarea apare în slice-ul returnat
//   - dacă registry e gol → Matcher gol, fără erori
func New(reg *registry.Registry) (*Matcher, []error) {
	if reg == nil {
		return &Matcher{}, nil
	}

	m := &Matcher{}
	var errs []error

	// Sort DD-uri pe ID pentru ordine deterministică (priority by load order).
	all := reg.All()
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })

	for _, dd := range all {
		for i := range dd.Identification.TopicMatch {
			spec := &dd.Identification.TopicMatch[i]
			cp, err := compile(dd, spec)
			if err != nil {
				errs = append(errs, fmt.Errorf("dd=%s pattern=%q: %w", dd.ID, spec.Pattern, err))
				continue
			}
			m.patterns = append(m.patterns, cp)
		}
	}

	return m, errs
}

// Match returnează primul DD al cărui pattern matchează `topic`, sau nil dacă niciunul.
//
// Algoritm: linear scan prin patterns compile-uite (în ordinea dată de New).
// Pentru < 1000 patterns e sub 50µs pe procesor modern.
func (m *Matcher) Match(topic string) *Match {
	for _, cp := range m.patterns {
		groups := cp.regex.FindStringSubmatch(topic)
		if groups == nil {
			continue
		}
		return &Match{
			Definition: cp.dd,
			Pattern:    cp.spec.Pattern,
			Stream:     cp.spec.Stream,
			Extracted:  extract(cp.spec.Extract, groups, cp.regex.SubexpNames(), cp.mqttPos),
		}
	}
	return nil
}

// Count — câte patterns sunt compile-uite (util pentru debug / health check).
func (m *Matcher) Count() int { return len(m.patterns) }

// ── compile + helpers ─────────────────────────────────────────────────────

func compile(dd *registry.DeviceDefinition, spec *registry.TopicMatchSpec) (compiled, error) {
	if strings.HasPrefix(spec.Pattern, "~") {
		raw := strings.TrimPrefix(spec.Pattern, "~")
		rx, err := regexp.Compile(raw)
		if err != nil {
			return compiled{}, fmt.Errorf("regex compile: %w", err)
		}
		return compiled{dd: dd, spec: spec, regex: rx}, nil
	}

	// MQTT wildcard pattern → regex
	rx, mqttPos, err := mqttToRegex(spec.Pattern)
	if err != nil {
		return compiled{}, err
	}
	return compiled{dd: dd, spec: spec, regex: rx, mqttPos: mqttPos}, nil
}

// mqttToRegex transformă un MQTT topic filter în regex Go anchorat.
//
// Reguli:
//   - "+" segment → "([^/]+)"  (capture group)
//   - "#" terminal → "(.*)"     (capture group, multi-segment, REQUIRES la final)
//   - alte segmente → escape regex literal
//   - prefix "^", suffix "$" pentru match strict
//
// Returnează regex compile-uit + lista de nume sintetice per capture group ("$1","$2"...).
func mqttToRegex(pattern string) (*regexp.Regexp, []string, error) {
	if pattern == "" {
		return nil, nil, fmt.Errorf("empty pattern")
	}

	parts := strings.Split(pattern, "/")
	var sb strings.Builder
	sb.WriteString("^")
	var posNames []string
	groupIdx := 0

	for i, part := range parts {
		if i > 0 {
			sb.WriteString("/")
		}
		switch {
		case part == "+":
			groupIdx++
			sb.WriteString("([^/]+)")
			posNames = append(posNames, fmt.Sprintf("$%d", groupIdx))
		case part == "#":
			if i != len(parts)-1 {
				return nil, nil, fmt.Errorf("`#` must be last segment in %q", pattern)
			}
			groupIdx++
			sb.WriteString("(.*)")
			posNames = append(posNames, fmt.Sprintf("$%d", groupIdx))
		case strings.Contains(part, "+") || strings.Contains(part, "#"):
			return nil, nil, fmt.Errorf("wildcard mixed with literal in segment %q", part)
		default:
			sb.WriteString(regexp.QuoteMeta(part))
		}
	}
	sb.WriteString("$")

	rx, err := regexp.Compile(sb.String())
	if err != nil {
		return nil, nil, fmt.Errorf("compile generated regex %q: %w", sb.String(), err)
	}
	return rx, posNames, nil
}

// extract aplică spec.Extract asupra grupurilor matched.
//
// Reguli:
//   - valoare "$N" → grupul N (pozițional, începe de la 1)
//   - valoare "<name>" — caută grupul named în regex; dacă nu există, lookup în
//     mqttPos (variabile MQTT sintetice "$1"..."$N")
//
// Pentru regex (~) → SubexpNames() are nume; pentru MQTT (mqttPos populat) → fără
// nume, doar poziții. Funcția gestionează ambele cazuri.
func extract(spec map[string]string, groups []string, regexNames []string, mqttPos []string) map[string]string {
	if len(spec) == 0 {
		return nil
	}
	out := make(map[string]string, len(spec))
	for varName, ref := range spec {
		val := resolveRef(ref, groups, regexNames, mqttPos)
		if val != "" {
			out[varName] = val
		}
	}
	return out
}

func resolveRef(ref string, groups []string, regexNames []string, mqttPos []string) string {
	// Pozițional "$N"
	if strings.HasPrefix(ref, "$") {
		var n int
		if _, err := fmt.Sscanf(ref, "$%d", &n); err != nil {
			return ""
		}
		if n < 1 || n >= len(groups) {
			return ""
		}
		return groups[n]
	}
	// Named (regex): caută în SubexpNames
	for i, name := range regexNames {
		if name == ref && i < len(groups) {
			return groups[i]
		}
	}
	// MQTT sintetic: caută în mqttPos
	for i, name := range mqttPos {
		if name == ref && i+1 < len(groups) {
			return groups[i+1]
		}
	}
	return ""
}
