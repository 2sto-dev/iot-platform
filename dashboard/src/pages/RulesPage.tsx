import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { canWrite } from "../lib/auth";
import { RuleBuilder } from "../components/rules/RuleBuilder";
import type { RuleDraft, Condition, Action } from "../components/rules/types";
import { newId } from "../components/rules/types";

interface Rule {
  id: number;
  name: string;
  description: string;
  trigger_stream_pattern: string;
  enabled: boolean;
  cooldown_seconds: number;
  conditions: unknown;
  actions: unknown;
  updated_at: string;
}

/** Reattach client-side IDs needed for drag/keys. */
function hydrateConditions(c: unknown): Condition {
  if (c && typeof c === "object" && "operator" in c) {
    const g = c as { operator: "AND" | "OR" | "NOT"; conditions: unknown[] };
    return {
      id: newId(),
      operator: g.operator,
      conditions: g.conditions.map(hydrateConditions),
    };
  }
  const leaf = c as { field: string; op: string; value?: unknown };
  return {
    id: newId(),
    field: leaf.field ?? "",
    op: (leaf.op ?? "gt") as never,
    value: leaf.value as never,
  };
}

function hydrateActions(arr: unknown): Action[] {
  if (!Array.isArray(arr)) return [];
  return arr.map((a) => ({ ...(a as object), id: newId() } as Action));
}

function summarizeCondition(c: unknown, depth = 0): string {
  if (!c || typeof c !== "object") return "—";
  if ("operator" in c) {
    const g = c as { operator: string; conditions: unknown[] };
    const op = g.operator;
    const inner = g.conditions.map((x) => summarizeCondition(x, depth + 1)).join(op === "OR" ? " OR " : op === "NOT" ? "" : " AND ");
    return depth === 0 ? inner : `(${op === "NOT" ? "NOT " : ""}${inner})`;
  }
  const l = c as { field: string; op: string; value?: unknown };
  return `${l.field} ${l.op}${l.value !== undefined ? " " + JSON.stringify(l.value) : ""}`;
}

export default function RulesPage() {
  const qc = useQueryClient();
  const tenant = localStorage.getItem("tenant_slug") ?? "";
  const writable = canWrite();
  const [editing, setEditing] = useState<{ id?: number; draft?: RuleDraft } | null>(null);

  const { data: rules, isLoading } = useQuery<Rule[]>({
    queryKey: ["rules", tenant],
    queryFn: () => api.get("/v1/rules/").then((r) => r.data),
  });

  const toggle = useMutation({
    mutationFn: (id: number) => api.patch(`/v1/rules/${id}/toggle/`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["rules", tenant] }),
  });

  const deleteRule = useMutation({
    mutationFn: (id: number) => api.delete(`/v1/rules/${id}/`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["rules", tenant] }),
  });

  function startNew() {
    setEditing({});
  }

  function startEdit(r: Rule) {
    const draft: RuleDraft = {
      name: r.name,
      description: r.description,
      trigger_stream_pattern: r.trigger_stream_pattern,
      cooldown_seconds: r.cooldown_seconds,
      enabled: r.enabled,
      conditions: hydrateConditions(r.conditions),
      actions: hydrateActions(r.actions),
    };
    setEditing({ id: r.id, draft });
  }

  if (isLoading) return <p className="text-gray-500">Loading rules…</p>;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Rules</h1>
          <p className="text-sm text-gray-500 mt-1">Automate device behavior — drag conditions and actions to organize.</p>
        </div>
        {!editing && writable && (
          <button
            onClick={startNew}
            className="bg-blue-600 text-white px-4 py-2 rounded-lg text-sm font-medium hover:bg-blue-700"
          >
            + New rule
          </button>
        )}
      </div>

      {!writable && !editing && (
        <div className="mb-4 bg-amber-50 border border-amber-200 rounded-lg px-4 py-2.5 text-sm text-amber-800 flex items-center gap-2">
          <span className="text-amber-600">ⓘ</span>
          You have read-only access. Contact an OWNER or ADMIN to manage rules.
        </div>
      )}

      {editing && (
        <RuleBuilder
          ruleId={editing.id}
          initial={editing.draft}
          onSaved={() => setEditing(null)}
          onCancel={() => setEditing(null)}
        />
      )}

      {!editing && (
        <div className="space-y-3">
          {rules?.map((rule) => (
            <div key={rule.id} className="bg-white border border-gray-200 rounded-xl p-4 shadow-sm">
              <div className="flex items-start justify-between gap-4">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-3 flex-wrap">
                    <span className={`w-2 h-2 rounded-full flex-shrink-0 ${rule.enabled ? "bg-green-500" : "bg-gray-300"}`} />
                    <h3 className="font-semibold text-gray-900">{rule.name}</h3>
                    <span className="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">
                      stream: <code>{rule.trigger_stream_pattern}</code>
                    </span>
                    <span className="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">
                      cooldown: {rule.cooldown_seconds}s
                    </span>
                    <span className="text-xs text-gray-400 bg-gray-100 px-2 py-0.5 rounded-full">
                      {Array.isArray(rule.actions) ? rule.actions.length : 0} action{(Array.isArray(rule.actions) ? rule.actions.length : 0) !== 1 ? "s" : ""}
                    </span>
                  </div>
                  {rule.description && <p className="text-sm text-gray-600 mt-2 ml-5">{rule.description}</p>}
                  <p className="text-xs text-gray-500 mt-2 ml-5 font-mono truncate" title={summarizeCondition(rule.conditions)}>
                    IF {summarizeCondition(rule.conditions)}
                  </p>
                  <p className="text-[11px] text-gray-400 mt-1 ml-5">
                    Updated {new Date(rule.updated_at).toLocaleString()}
                  </p>
                </div>
                {writable ? (
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => startEdit(rule)}
                      className="text-xs px-3 py-1.5 rounded-lg font-medium bg-blue-100 text-blue-700 hover:bg-blue-200"
                    >
                      Edit
                    </button>
                    <button
                      onClick={() => toggle.mutate(rule.id)}
                      className={`text-xs px-3 py-1.5 rounded-lg font-medium ${
                        rule.enabled ? "bg-yellow-100 text-yellow-700 hover:bg-yellow-200" : "bg-green-100 text-green-700 hover:bg-green-200"
                      }`}
                    >
                      {rule.enabled ? "Disable" : "Enable"}
                    </button>
                    <button
                      onClick={() => { if (confirm(`Delete rule "${rule.name}"?`)) deleteRule.mutate(rule.id); }}
                      className="text-xs px-3 py-1.5 rounded-lg font-medium bg-red-100 text-red-700 hover:bg-red-200"
                    >
                      Delete
                    </button>
                  </div>
                ) : (
                  <span className="text-[11px] text-gray-400">read-only</span>
                )}
              </div>
            </div>
          ))}
          {rules?.length === 0 && (
            <div className="text-center py-12 text-gray-400 border-2 border-dashed border-gray-200 rounded-xl">
              {writable ? (
                <>No rules yet. Click <strong>+ New rule</strong> to create your first automation.</>
              ) : (
                <>No rules configured for this tenant.</>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
