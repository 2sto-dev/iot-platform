export type LeafOp =
  | "eq" | "ne" | "gt" | "gte" | "lt" | "lte"
  | "in" | "not_in"
  | "contains" | "not_contains" | "regex"
  | "is_null" | "is_not_null"
  | "changed";

export const LEAF_OPS: { value: LeafOp; label: string; needsValue: boolean }[] = [
  { value: "eq", label: "= equals", needsValue: true },
  { value: "ne", label: "≠ not equals", needsValue: true },
  { value: "gt", label: "> greater than", needsValue: true },
  { value: "gte", label: "≥ greater or equal", needsValue: true },
  { value: "lt", label: "< less than", needsValue: true },
  { value: "lte", label: "≤ less or equal", needsValue: true },
  { value: "in", label: "in (any of)", needsValue: true },
  { value: "not_in", label: "not in", needsValue: true },
  { value: "contains", label: "contains text", needsValue: true },
  { value: "not_contains", label: "doesn't contain", needsValue: true },
  { value: "regex", label: "matches regex", needsValue: true },
  { value: "is_null", label: "is null", needsValue: false },
  { value: "is_not_null", label: "is not null", needsValue: false },
  { value: "changed", label: "value changed", needsValue: false },
];

export interface LeafCondition {
  id: string; // client-side only, pentru drag/keys
  field: string;
  op: LeafOp;
  value?: string | number | boolean;
}

export interface GroupCondition {
  id: string;
  operator: "AND" | "OR" | "NOT";
  conditions: Condition[];
}

export type Condition = LeafCondition | GroupCondition;

export function isGroup(c: Condition): c is GroupCondition {
  return (c as GroupCondition).operator !== undefined;
}

export type ActionType = "downlink" | "notify" | "webhook" | "set_shadow";

export interface DownlinkAction {
  id: string;
  type: "downlink";
  target_serial: string;
  action: string;
  payload: Record<string, unknown>;
}

export interface NotifyAction {
  id: string;
  type: "notify";
  channel_id: number | null;
  title: string;
  body: string;
}

export interface WebhookAction {
  id: string;
  type: "webhook";
  url: string;
  method: "GET" | "POST" | "PUT";
  body_template: string;
}

export interface SetShadowAction {
  id: string;
  type: "set_shadow";
  target_serial: string;
  desired: Record<string, unknown>;
}

export type Action = DownlinkAction | NotifyAction | WebhookAction | SetShadowAction;

export interface RuleDraft {
  name: string;
  description: string;
  trigger_stream_pattern: string;
  cooldown_seconds: number;
  enabled: boolean;
  conditions: Condition;
  actions: Action[];
}

export const ACTION_TYPE_LABELS: Record<ActionType, { label: string; icon: string; color: string }> = {
  downlink: { label: "Send command to device", icon: "📡", color: "blue" },
  notify: { label: "Send notification", icon: "🔔", color: "amber" },
  webhook: { label: "Call webhook", icon: "🌐", color: "violet" },
  set_shadow: { label: "Update device shadow", icon: "🪄", color: "emerald" },
};

let _idCounter = 0;
export const newId = () => `c${++_idCounter}_${Date.now()}`;

export function emptyLeaf(): LeafCondition {
  return { id: newId(), field: "", op: "gt", value: "" };
}

export function emptyGroup(operator: "AND" | "OR" | "NOT" = "AND"): GroupCondition {
  return { id: newId(), operator, conditions: [emptyLeaf()] };
}

export function emptyAction(type: ActionType): Action {
  switch (type) {
    case "downlink":
      return { id: newId(), type, target_serial: "", action: "", payload: {} };
    case "notify":
      return { id: newId(), type, channel_id: null, title: "", body: "" };
    case "webhook":
      return { id: newId(), type, url: "", method: "POST", body_template: "" };
    case "set_shadow":
      return { id: newId(), type, target_serial: "", desired: {} };
  }
}

/** Curăță obiectul de id-urile client-side înainte de POST la API. */
export function stripIds(c: Condition): unknown {
  if (isGroup(c)) {
    return {
      operator: c.operator,
      conditions: c.conditions.map(stripIds),
    };
  }
  const { id: _, ...rest } = c;
  return rest;
}

export function stripActionIds(a: Action): unknown {
  const { id: _, ...rest } = a;
  return rest;
}
