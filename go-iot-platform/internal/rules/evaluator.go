package rules

import (
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Evaluate recursively evaluates a ConditionNode against data.
// prevState: map of "field_path" → previous value (for "changed" operator).
func Evaluate(node ConditionNode, data map[string]interface{}, prevState map[string]interface{}) bool {
	op := strings.ToUpper(node.Operator)
	switch op {
	case "AND":
		for _, child := range node.Conditions {
			if !Evaluate(child, data, prevState) {
				return false
			}
		}
		return len(node.Conditions) > 0
	case "OR":
		for _, child := range node.Conditions {
			if Evaluate(child, data, prevState) {
				return true
			}
		}
		return false
	case "NOT":
		if node.Condition == nil {
			return false
		}
		return !Evaluate(*node.Condition, data, prevState)
	default:
		// Leaf condition
		if node.Field == "" {
			return false
		}
		current := ExtractField(data, node.Field)
		return compareLeaf(current, node.Op, node.Value, prevState[node.Field])
	}
}

// compareLeaf evaluates a leaf condition: current <op> value.
func compareLeaf(current interface{}, op string, value interface{}, prev interface{}) bool {
	switch op {
	case "is_null":
		return current == nil
	case "is_not_null":
		return current != nil
	case "changed":
		return !reflect.DeepEqual(current, prev)
	case "eq":
		return valuesEqual(current, value)
	case "ne":
		return !valuesEqual(current, value)
	case "gt":
		a, b, ok := toFloats(current, value)
		return ok && a > b
	case "gte":
		a, b, ok := toFloats(current, value)
		return ok && a >= b
	case "lt":
		a, b, ok := toFloats(current, value)
		return ok && a < b
	case "lte":
		a, b, ok := toFloats(current, value)
		return ok && a <= b
	case "in":
		return inList(current, value)
	case "not_in":
		return !inList(current, value)
	case "contains":
		return strings.Contains(toString(current), toString(value))
	case "not_contains":
		return !strings.Contains(toString(current), toString(value))
	case "regex":
		pattern, ok := value.(string)
		if !ok {
			return false
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		return re.MatchString(toString(current))
	}
	return false
}

func valuesEqual(a, b interface{}) bool {
	// Try numeric comparison first to handle int vs float64 mismatches from JSON.
	fa, fb, ok := toFloats(a, b)
	if ok {
		return math.Abs(fa-fb) < 1e-9
	}
	return reflect.DeepEqual(a, b)
}

func toFloats(a, b interface{}) (float64, float64, bool) {
	fa, oka := toFloat(a)
	fb, okb := toFloat(b)
	return fa, fb, oka && okb
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}

func inList(val interface{}, list interface{}) bool {
	lst, ok := list.([]interface{})
	if !ok {
		return false
	}
	for _, item := range lst {
		if valuesEqual(val, item) {
			return true
		}
	}
	return false
}
