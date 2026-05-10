import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../lib/api";
import { ConditionTree } from "./ConditionTree";
import { ActionsList } from "./ActionsList";
import {
  type RuleDraft, type Action,
  emptyGroup, stripIds, stripActionIds,
} from "./types";

interface Device {
  id: number;
  serial_number: string;
  description: string;
  device_type: string;
}

interface Props {
  initial?: RuleDraft;
  ruleId?: number;
  onSaved: () => void;
  onCancel: () => void;
}

export function RuleBuilder({ initial, ruleId, onSaved, onCancel }: Props) {
  const qc = useQueryClient();
  const tenant = localStorage.getItem("tenant_slug") ?? "";
  const [draft, setDraft] = useState<RuleDraft>(initial ?? {
    name: "",
    description: "",
    trigger_stream_pattern: "telemetry",
    cooldown_seconds: 60,
    enabled: true,
    conditions: emptyGroup("AND"),
    actions: [],
  });
  const [error, setError] = useState<string | null>(null);

  const { data: devices } = useQuery<Device[]>({
    queryKey: ["devices", tenant],
    queryFn: () => api.get("/devices/").then((r) => r.data),
  });

  const save = useMutation({
    mutationFn: (body: unknown) =>
      ruleId
        ? api.patch(`/v1/rules/${ruleId}/`, body)
        : api.post("/v1/rules/", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["rules"] });
      onSaved();
    },
    onError: (err: unknown) => {
      const e = err as { response?: { data?: unknown } };
      const data = e.response?.data;
      setError(typeof data === "string" ? data : JSON.stringify(data, null, 2));
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (!draft.name.trim()) { setError("Name is required."); return; }
    if (draft.actions.length === 0) { setError("Add at least one action."); return; }
    save.mutate({
      name: draft.name,
      description: draft.description,
      trigger_stream_pattern: draft.trigger_stream_pattern,
      cooldown_seconds: draft.cooldown_seconds,
      enabled: draft.enabled,
      conditions: stripIds(draft.conditions),
      actions: draft.actions.map(stripActionIds),
    });
  }

  return (
    <form onSubmit={handleSubmit} className="bg-white border border-gray-200 rounded-xl p-6 mb-6 shadow-sm space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-gray-900">
          {ruleId ? "Edit rule" : "New rule"}
        </h2>
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            checked={draft.enabled}
            onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
            className="rounded"
          />
          Enabled
        </label>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="md:col-span-2">
          <label className="block text-xs font-semibold text-gray-700 uppercase tracking-wider mb-1">Name</label>
          <input
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            placeholder="e.g. Turn ventilator on when hot"
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm"
            required
          />
        </div>
        <div>
          <label className="block text-xs font-semibold text-gray-700 uppercase tracking-wider mb-1">Cooldown (s)</label>
          <input
            type="number"
            value={draft.cooldown_seconds}
            onChange={(e) => setDraft({ ...draft, cooldown_seconds: Number(e.target.value) })}
            min={0}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm"
          />
        </div>
        <div className="md:col-span-3">
          <label className="block text-xs font-semibold text-gray-700 uppercase tracking-wider mb-1">Description (optional)</label>
          <input
            value={draft.description}
            onChange={(e) => setDraft({ ...draft, description: e.target.value })}
            className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm"
          />
        </div>
      </div>

      {/* WHEN */}
      <Section title="WHEN" subtitle="Trigger source — message arrives on this stream">
        <input
          value={draft.trigger_stream_pattern}
          onChange={(e) => setDraft({ ...draft, trigger_stream_pattern: e.target.value })}
          placeholder="telemetry · or telemetry,emeter · or *"
          className="w-full max-w-md border border-gray-300 rounded-lg px-3 py-2 text-sm"
        />
        <p className="text-xs text-gray-500 mt-1">
          Use comma to match multiple streams (e.g. <code className="bg-gray-100 px-1 rounded">telemetry,emeter</code>) or <code className="bg-gray-100 px-1 rounded">*</code> for any.
        </p>
      </Section>

      {/* IF */}
      <Section title="IF" subtitle="Conditions — drag to reorder, nest groups for AND/OR/NOT logic">
        <ConditionTree
          condition={draft.conditions}
          onChange={(c) => setDraft({ ...draft, conditions: c })}
          fieldSuggestions={[
            "temperature", "humidity", "power_w", "voltage", "current",
            "pv_input_power", "battery_soc", "house_load_kw_est",
            "relay.state", "measurements.0.value",
          ]}
        />
      </Section>

      {/* THEN */}
      <Section title="THEN" subtitle="Actions — executed in order; drag to reorder">
        <ActionsList
          actions={draft.actions}
          onChange={(a: Action[]) => setDraft({ ...draft, actions: a })}
          devices={devices ?? []}
        />
      </Section>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-3 text-sm text-red-700 whitespace-pre-wrap font-mono">
          {error}
        </div>
      )}

      <div className="flex gap-2 pt-2 border-t border-gray-100">
        <button
          type="submit"
          disabled={save.isPending}
          className="bg-blue-600 text-white px-5 py-2 rounded-lg text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
        >
          {save.isPending ? "Saving…" : ruleId ? "Save changes" : "Create rule"}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="px-5 py-2 rounded-lg text-sm font-medium text-gray-600 hover:bg-gray-100"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}

function Section({ title, subtitle, children }: { title: string; subtitle: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="flex items-baseline gap-3 mb-2">
        <h3 className="text-sm font-bold text-gray-800 uppercase tracking-wider">{title}</h3>
        <p className="text-xs text-gray-500">{subtitle}</p>
      </div>
      {children}
    </div>
  );
}
