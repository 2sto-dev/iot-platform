package capabilities

import "fmt"

// inheritanceMap leaga capability composite la parinții lor (capabilities derivate).
//
// Reguli:
//   - Cheia (composite) "expune" toate capabilities din slice-ul valoare
//   - Parinții pot avea proprii lor parinți (recursive resolve)
//   - Cycles sunt detectate la Resolve() runtime
//
// Adaugare entry nou = ADR amendment + add to Vocabulary + add here.
var inheritanceMap = map[Capability][]Capability{
	SmartPlug:      {Relay, PowerMeter},
	HybridInverter: {Inverter, Battery},
	ClimateSensor:  {TemperatureSensor, HumiditySensor},
	EVCharger:      {Relay, PowerMeter},
	SmartMeter:     {PowerMeter},
	Light:          {Dimmer},
}

// Resolve expandeaza o lista de capabilities declared in lista completa
// (incluzand parintii inherited).
//
// Exemplu:
//   Resolve(["smart_plug", "wifi"]) -> ["smart_plug", "relay", "power_meter", "wifi"]
//
// Algoritm: BFS pe inheritance map. Cycles sunt blocate de "seen" set;
// dacă apare ciclu, ResolveStrict() returneaza eroare. Resolve() doar îl ignoră.
//
// Output:
//   - ordine deterministă (declared first, apoi BFS expand)
//   - duplicate eliminate
func Resolve(declared []Capability) []Capability {
	out, _ := resolveBFS(declared, false)
	return out
}

// ResolveStrict ca Resolve(), dar returneaza eroare dacă detectează:
//   - capability necunoscut (nu e în Vocabulary)
//   - inheritance cycle
func ResolveStrict(declared []Capability) ([]Capability, error) {
	return resolveBFS(declared, true)
}

func resolveBFS(declared []Capability, strict bool) ([]Capability, error) {
	seen := make(map[Capability]bool, len(declared)*2)
	var ordered []Capability
	queue := append([]Capability(nil), declared...)

	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if seen[c] {
			continue
		}
		if strict && !IsKnown(c) {
			return nil, fmt.Errorf("capability %q not in vocabulary", c)
		}
		seen[c] = true
		ordered = append(ordered, c)

		parents, ok := inheritanceMap[c]
		if !ok {
			continue
		}
		queue = append(queue, parents...)
	}

	return ordered, nil
}

// Inherits returneaza true daca `child` deriva (direct sau prin lant) din `parent`.
//
// Exemple:
//   Inherits("smart_plug", "relay")        // true
//   Inherits("smart_plug", "power_meter")  // true
//   Inherits("smart_plug", "battery")      // false
//   Inherits("relay", "smart_plug")        // false (direction matters)
func Inherits(child, parent Capability) bool {
	resolved := Resolve([]Capability{child})
	for _, c := range resolved {
		if c == parent {
			return true
		}
	}
	return false
}
