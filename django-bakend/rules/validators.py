"""Validators for the rule condition DSL and action list."""
from rest_framework.exceptions import ValidationError

LEAF_OPS = {
    "eq", "ne",
    "gt", "gte", "lt", "lte",
    "in", "not_in",
    "contains", "not_contains",
    "regex",
    "is_null", "is_not_null",
    "changed",
}
NO_VALUE_OPS = {"is_null", "is_not_null", "changed"}
ACTION_TYPES = {"downlink", "notify", "webhook", "set_shadow"}


def validate_condition_node(node, path="conditions"):
    if not isinstance(node, dict):
        raise ValidationError({path: "Must be an object."})

    has_operator = "operator" in node
    has_field = "field" in node

    if has_operator and has_field:
        raise ValidationError({path: "Cannot have both 'operator' and 'field'."})
    if not has_operator and not has_field:
        raise ValidationError({path: "Must have 'operator' (branch) or 'field' (leaf)."})

    if has_operator:
        op = node["operator"]
        if op not in ("AND", "OR", "NOT"):
            raise ValidationError({path: f"operator must be AND, OR or NOT (got '{op}')."})
        if op == "NOT":
            if "condition" not in node:
                raise ValidationError({path: "NOT requires a 'condition' key."})
            validate_condition_node(node["condition"], path=f"{path}.condition")
        else:
            children = node.get("conditions")
            if not isinstance(children, list) or len(children) == 0:
                raise ValidationError({path: f"{op} requires a non-empty 'conditions' list."})
            for i, child in enumerate(children):
                validate_condition_node(child, path=f"{path}[{i}]")
    else:
        # Leaf
        if not isinstance(node["field"], str) or not node["field"]:
            raise ValidationError({path: "'field' must be a non-empty string."})
        leaf_op = node.get("op")
        if leaf_op not in LEAF_OPS:
            raise ValidationError({path: f"'op' must be one of {sorted(LEAF_OPS)} (got '{leaf_op}')."})
        if leaf_op not in NO_VALUE_OPS and "value" not in node:
            raise ValidationError({path: f"op '{leaf_op}' requires a 'value'."})
        if leaf_op == "in" or leaf_op == "not_in":
            if not isinstance(node.get("value"), list):
                raise ValidationError({path: f"op '{leaf_op}' requires 'value' to be a list."})


def validate_actions(actions, path="actions"):
    if not isinstance(actions, list) or len(actions) == 0:
        raise ValidationError({path: "Must be a non-empty list of actions."})
    for i, action in enumerate(actions):
        p = f"{path}[{i}]"
        if not isinstance(action, dict):
            raise ValidationError({p: "Each action must be an object."})
        t = action.get("type")
        if t not in ACTION_TYPES:
            raise ValidationError({p: f"'type' must be one of {sorted(ACTION_TYPES)} (got '{t}')."})
        if t == "downlink":
            if not action.get("action"):
                raise ValidationError({p: "downlink action requires 'action' (command name)."})
        elif t == "notify":
            if not action.get("channel_id"):
                raise ValidationError({p: "notify action requires 'channel_id'."})
            if not action.get("body"):
                raise ValidationError({p: "notify action requires 'body'."})
        elif t == "webhook":
            if not action.get("url"):
                raise ValidationError({p: "webhook action requires 'url'."})
        elif t == "set_shadow":
            if not isinstance(action.get("desired"), dict):
                raise ValidationError({p: "set_shadow action requires 'desired' object."})
