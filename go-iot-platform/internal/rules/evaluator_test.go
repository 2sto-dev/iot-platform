package rules

import (
	"encoding/json"
	"testing"
)

func TestEvaluateLeaf(t *testing.T) {
	data := map[string]interface{}{
		"power_w":     float64(1500),
		"temperature": float64(75),
		"relay_state": "on",
		"error":       nil,
	}

	tests := []struct {
		name      string
		condition string
		want      bool
	}{
		{"gt true", `{"field":"power_w","op":"gt","value":1000}`, true},
		{"gt false", `{"field":"power_w","op":"gt","value":2000}`, false},
		{"gte equal", `{"field":"power_w","op":"gte","value":1500}`, true},
		{"lt true", `{"field":"temperature","op":"lt","value":100}`, true},
		{"lte true", `{"field":"temperature","op":"lte","value":75}`, true},
		{"eq string", `{"field":"relay_state","op":"eq","value":"on"}`, true},
		{"ne string", `{"field":"relay_state","op":"ne","value":"off"}`, true},
		{"is_null true", `{"field":"error","op":"is_null"}`, true},
		{"is_not_null false", `{"field":"error","op":"is_not_null"}`, false},
		{"is_not_null true", `{"field":"power_w","op":"is_not_null"}`, true},
		{"in true", `{"field":"relay_state","op":"in","value":["on","off"]}`, true},
		{"not_in true", `{"field":"relay_state","op":"not_in","value":["off","unknown"]}`, true},
		{"contains true", `{"field":"relay_state","op":"contains","value":"on"}`, true},
		{"regex true", `{"field":"relay_state","op":"regex","value":"^on$"}`, true},
		{"regex false", `{"field":"relay_state","op":"regex","value":"^off$"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node ConditionNode
			if err := json.Unmarshal([]byte(tt.condition), &node); err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := Evaluate(node, data, nil)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateBranch(t *testing.T) {
	data := map[string]interface{}{
		"power_w":     float64(1500),
		"temperature": float64(75),
	}

	t.Run("AND both true", func(t *testing.T) {
		cond := `{"operator":"AND","conditions":[
			{"field":"power_w","op":"gt","value":1000},
			{"field":"temperature","op":"lt","value":100}
		]}`
		var node ConditionNode
		json.Unmarshal([]byte(cond), &node)
		if !Evaluate(node, data, nil) {
			t.Error("expected true")
		}
	})

	t.Run("AND one false", func(t *testing.T) {
		cond := `{"operator":"AND","conditions":[
			{"field":"power_w","op":"gt","value":1000},
			{"field":"temperature","op":"gt","value":100}
		]}`
		var node ConditionNode
		json.Unmarshal([]byte(cond), &node)
		if Evaluate(node, data, nil) {
			t.Error("expected false")
		}
	})

	t.Run("OR one true", func(t *testing.T) {
		cond := `{"operator":"OR","conditions":[
			{"field":"power_w","op":"gt","value":9999},
			{"field":"temperature","op":"lt","value":100}
		]}`
		var node ConditionNode
		json.Unmarshal([]byte(cond), &node)
		if !Evaluate(node, data, nil) {
			t.Error("expected true")
		}
	})

	t.Run("NOT inverts", func(t *testing.T) {
		cond := `{"operator":"NOT","condition":{"field":"power_w","op":"gt","value":9999}}`
		var node ConditionNode
		json.Unmarshal([]byte(cond), &node)
		if !Evaluate(node, data, nil) {
			t.Error("expected true")
		}
	})

	t.Run("nested AND OR", func(t *testing.T) {
		cond := `{"operator":"AND","conditions":[
			{"field":"power_w","op":"gt","value":1000},
			{"operator":"OR","conditions":[
				{"field":"temperature","op":"gt","value":100},
				{"field":"temperature","op":"lt","value":80}
			]}
		]}`
		var node ConditionNode
		json.Unmarshal([]byte(cond), &node)
		if !Evaluate(node, data, nil) {
			t.Error("expected true")
		}
	})
}

func TestEvaluateChanged(t *testing.T) {
	data := map[string]interface{}{"relay": "off"}
	prev := map[string]interface{}{"relay": "on"}

	cond := ConditionNode{Field: "relay", Op: "changed"}
	if !Evaluate(cond, data, prev) {
		t.Error("expected changed=true when value differs")
	}

	prevSame := map[string]interface{}{"relay": "off"}
	if Evaluate(cond, data, prevSame) {
		t.Error("expected changed=false when value same")
	}
}

func TestFieldPath(t *testing.T) {
	data := map[string]interface{}{
		"relay": map[string]interface{}{"state": "on"},
		"measurements": []interface{}{
			map[string]interface{}{"key": "active_power_kw", "value": float64(5.2)},
			map[string]interface{}{"key": "battery_soc_pct", "value": float64(85)},
		},
	}

	t.Run("nested dot", func(t *testing.T) {
		v := ExtractField(data, "relay.state")
		if v != "on" {
			t.Errorf("got %v", v)
		}
	})

	t.Run("array index", func(t *testing.T) {
		v := ExtractField(data, "measurements.0.value")
		if v.(float64) != 5.2 {
			t.Errorf("got %v", v)
		}
	})

	t.Run("array filter", func(t *testing.T) {
		elem := ExtractField(data, "measurements[key=active_power_kw]")
		m := elem.(map[string]interface{})
		if m["value"].(float64) != 5.2 {
			t.Errorf("got %v", m)
		}
	})

	t.Run("array filter then sub-field", func(t *testing.T) {
		v := ExtractField(data, "measurements[key=battery_soc_pct].value")
		if v.(float64) != 85 {
			t.Errorf("got %v", v)
		}
	})

	t.Run("missing field returns nil", func(t *testing.T) {
		v := ExtractField(data, "nonexistent.deep")
		if v != nil {
			t.Errorf("expected nil, got %v", v)
		}
	})
}

func TestMatchesStream(t *testing.T) {
	tests := []struct {
		pattern string
		stream  string
		want    bool
	}{
		{"*", "telemetry", true},
		{"*", "emeter", true},
		{"telemetry", "telemetry", true},
		{"telemetry", "emeter", false},
		{"telemetry,emeter", "emeter", true},
		{"telemetry,emeter", "STATE", false},
		{"", "anything", true},
	}
	for _, tt := range tests {
		rule := Rule{TriggerStreamPattern: tt.pattern}
		got := MatchesStream(rule, tt.stream)
		if got != tt.want {
			t.Errorf("pattern=%q stream=%q: got %v want %v", tt.pattern, tt.stream, got, tt.want)
		}
	}
}

func TestParseTopic(t *testing.T) {
	tid, serial, stream, ok := ParseTopic("tenants/2/devices/39371381/up/telemetry")
	if !ok || tid != 2 || serial != "39371381" || stream != "telemetry" {
		t.Errorf("got tid=%d serial=%s stream=%s ok=%v", tid, serial, stream, ok)
	}

	_, _, _, ok2 := ParseTopic("invalid/topic")
	if ok2 {
		t.Error("expected ok=false for invalid topic")
	}
}

func TestRenderTemplate(t *testing.T) {
	ctx := map[string]interface{}{
		"serial":    "DEV001",
		"tenant_id": int64(2),
		"power_w":   float64(1500),
	}
	result := RenderTemplate("Device {{serial}} power={{power_w}}W tenant={{tenant_id}}", ctx)
	want := "Device DEV001 power=1500W tenant=2"
	if result != want {
		t.Errorf("got %q want %q", result, want)
	}
}
