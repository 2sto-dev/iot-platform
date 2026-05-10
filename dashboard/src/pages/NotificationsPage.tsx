import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { canWrite, getRole } from "../lib/auth";

type ChannelType = "webhook" | "email" | "fcm";

interface Channel {
  id: number;
  name: string;
  type: ChannelType;
  config: Record<string, any>;
  enabled: boolean;
  created_at: string;
}

interface Event {
  id: number;
  channel_name: string;
  title: string;
  status: string;
  created_at: string;
}

interface FormState {
  name: string;
  type: ChannelType;
  enabled: boolean;
  // webhook
  webhook_url: string;
  webhook_method: string;
  // email
  email_to: string;
  email_from_name: string;
  // fcm
  fcm_target_kind: "token" | "topic";
  fcm_target_value: string;
}

const EMPTY_FORM: FormState = {
  name: "",
  type: "webhook",
  enabled: true,
  webhook_url: "",
  webhook_method: "POST",
  email_to: "",
  email_from_name: "",
  fcm_target_kind: "token",
  fcm_target_value: "",
};

const TYPE_LABELS: Record<ChannelType, string> = {
  webhook: "Webhook HTTP",
  email: "Email",
  fcm: "Firebase Push",
};

const TYPE_COLOR: Record<ChannelType, string> = {
  webhook: "bg-indigo-50 text-indigo-700 border-indigo-200",
  email: "bg-emerald-50 text-emerald-700 border-emerald-200",
  fcm: "bg-amber-50 text-amber-700 border-amber-200",
};

function formToConfig(f: FormState): Record<string, any> {
  if (f.type === "webhook") {
    return { url: f.webhook_url.trim(), method: f.webhook_method || "POST" };
  }
  if (f.type === "email") {
    const recipients = f.email_to
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    const cfg: Record<string, any> = { to: recipients };
    if (f.email_from_name.trim()) cfg.from_name = f.email_from_name.trim();
    return cfg;
  }
  // fcm
  return { [f.fcm_target_kind]: f.fcm_target_value.trim() };
}

function channelToForm(c: Channel): FormState {
  const cfg = c.config ?? {};
  return {
    name: c.name,
    type: c.type,
    enabled: c.enabled,
    webhook_url: cfg.url ?? "",
    webhook_method: cfg.method ?? "POST",
    email_to: Array.isArray(cfg.to) ? cfg.to.join(", ") : "",
    email_from_name: cfg.from_name ?? "",
    fcm_target_kind: cfg.topic ? "topic" : "token",
    fcm_target_value: cfg.topic ?? cfg.token ?? "",
  };
}

function extractError(e: any): string {
  const data = e?.response?.data;
  if (!data) return e?.message ?? "Unknown error";
  if (typeof data === "string") return data;
  return JSON.stringify(data, null, 2);
}

interface ModalProps {
  title: string;
  form: FormState;
  onChange: (f: FormState) => void;
  onSave: () => void;
  onClose: () => void;
  saving: boolean;
  error: string | null;
  isEdit: boolean;
}

function ChannelModal({ title, form, onChange, onSave, onClose, saving, error, isEdit }: ModalProps) {
  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg p-6 max-h-[90vh] overflow-auto">
        <h2 className="text-lg font-semibold text-gray-900 mb-4">{title}</h2>
        {error && (
          <pre className="mb-3 text-xs text-red-600 bg-red-50 rounded px-3 py-2 whitespace-pre-wrap break-words">
            {error}
          </pre>
        )}
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
            <input
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              value={form.name}
              onChange={(e) => onChange({ ...form, name: e.target.value })}
              placeholder="e.g. Slack #ops"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Type</label>
            <select
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100"
              value={form.type}
              onChange={(e) => onChange({ ...form, type: e.target.value as ChannelType })}
              disabled={isEdit}
            >
              <option value="webhook">Webhook HTTP</option>
              <option value="email">Email</option>
              <option value="fcm">Firebase Push</option>
            </select>
            {isEdit && (
              <p className="text-[11px] text-gray-400 mt-1">Type cannot be changed after creation.</p>
            )}
          </div>

          {form.type === "webhook" && (
            <>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Webhook URL</label>
                <input
                  className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                  value={form.webhook_url}
                  onChange={(e) => onChange({ ...form, webhook_url: e.target.value })}
                  placeholder="https://hooks.slack.com/services/..."
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">HTTP Method</label>
                <select
                  className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                  value={form.webhook_method}
                  onChange={(e) => onChange({ ...form, webhook_method: e.target.value })}
                >
                  <option value="POST">POST</option>
                  <option value="PUT">PUT</option>
                  <option value="PATCH">PATCH</option>
                </select>
              </div>
            </>
          )}

          {form.type === "email" && (
            <>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Recipients <span className="text-gray-400 text-xs">(comma-separated)</span>
                </label>
                <input
                  className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                  value={form.email_to}
                  onChange={(e) => onChange({ ...form, email_to: e.target.value })}
                  placeholder="ops@firma.ro, manager@firma.ro"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  From name <span className="text-gray-400 text-xs">(optional)</span>
                </label>
                <input
                  className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
                  value={form.email_from_name}
                  onChange={(e) => onChange({ ...form, email_from_name: e.target.value })}
                  placeholder="IoT Platform Alerts"
                />
              </div>
            </>
          )}

          {form.type === "fcm" && (
            <>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Target type</label>
                <select
                  className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                  value={form.fcm_target_kind}
                  onChange={(e) => onChange({ ...form, fcm_target_kind: e.target.value as "token" | "topic" })}
                >
                  <option value="token">Device token (single device)</option>
                  <option value="topic">Topic (broadcast)</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  {form.fcm_target_kind === "token" ? "FCM token" : "Topic name"}
                </label>
                <input
                  className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500"
                  value={form.fcm_target_value}
                  onChange={(e) => onChange({ ...form, fcm_target_value: e.target.value })}
                  placeholder={form.fcm_target_kind === "token" ? "fEx...abc" : "alerts-prod"}
                />
              </div>
            </>
          )}

          <div className="flex items-center gap-2 pt-2">
            <input
              id="enabled"
              type="checkbox"
              className="h-4 w-4 text-blue-600 border-gray-300 rounded focus:ring-blue-500"
              checked={form.enabled}
              onChange={(e) => onChange({ ...form, enabled: e.target.checked })}
            />
            <label htmlFor="enabled" className="text-sm text-gray-700">
              Enabled
            </label>
          </div>
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-gray-700 border border-gray-300 rounded-lg hover:bg-gray-50"
          >
            Cancel
          </button>
          <button
            onClick={onSave}
            disabled={saving || !form.name.trim()}
            className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default function NotificationsPage() {
  const qc = useQueryClient();
  const tenant = localStorage.getItem("tenant_slug") ?? "";
  const writable = canWrite();
  const role = getRole();

  const [modal, setModal] = useState<"add" | "edit" | null>(null);
  const [editTarget, setEditTarget] = useState<Channel | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [formError, setFormError] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<number | null>(null);
  const [testFeedback, setTestFeedback] = useState<{ id: number; ok: boolean } | null>(null);

  const { data: channels, isLoading: loadingChannels } = useQuery<Channel[]>({
    queryKey: ["channels", tenant],
    queryFn: () => api.get("/v1/notifications/channels/").then((r) => r.data),
  });

  const { data: events, isLoading: loadingEvents } = useQuery<Event[]>({
    queryKey: ["events", tenant],
    queryFn: () => api.get("/v1/notifications/events/").then((r) => r.data),
    refetchInterval: 15_000,
  });

  const createMut = useMutation({
    mutationFn: (body: { name: string; type: string; config: Record<string, any>; enabled: boolean }) =>
      api.post("/v1/notifications/channels/", body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["channels", tenant] });
      closeModal();
    },
    onError: (e: any) => setFormError(extractError(e)),
  });

  const updateMut = useMutation({
    mutationFn: ({ id, body }: { id: number; body: Record<string, any> }) =>
      api.patch(`/v1/notifications/channels/${id}/`, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["channels", tenant] });
      closeModal();
    },
    onError: (e: any) => setFormError(extractError(e)),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => api.delete(`/v1/notifications/channels/${id}/`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["channels", tenant] });
      setDeleteId(null);
    },
  });

  const testMut = useMutation({
    mutationFn: (id: number) => api.post(`/v1/notifications/channels/${id}/test/`),
    onSuccess: (_, id) => {
      setTestFeedback({ id, ok: true });
      qc.invalidateQueries({ queryKey: ["events", tenant] });
      setTimeout(() => setTestFeedback(null), 3000);
    },
    onError: (_, id) => {
      setTestFeedback({ id, ok: false });
      setTimeout(() => setTestFeedback(null), 3000);
    },
  });

  function openAdd() {
    setForm(EMPTY_FORM);
    setFormError(null);
    setModal("add");
  }

  function openEdit(c: Channel) {
    setEditTarget(c);
    setForm(channelToForm(c));
    setFormError(null);
    setModal("edit");
  }

  function closeModal() {
    setModal(null);
    setEditTarget(null);
    setFormError(null);
  }

  function handleSave() {
    setFormError(null);
    const config = formToConfig(form);
    if (modal === "add") {
      createMut.mutate({ name: form.name.trim(), type: form.type, config, enabled: form.enabled });
    } else if (modal === "edit" && editTarget) {
      updateMut.mutate({
        id: editTarget.id,
        body: { name: form.name.trim(), config, enabled: form.enabled },
      });
    }
  }

  const statusColor: Record<string, string> = {
    sent: "bg-emerald-100 text-emerald-700",
    failed: "bg-rose-100 text-rose-700",
    pending: "bg-amber-100 text-amber-700",
  };

  return (
    <div className="space-y-8 max-w-[1400px]">
      <div className="flex items-center justify-between mb-2">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 tracking-tight">Notification Channels</h1>
          <p className="text-sm text-gray-500 mt-1 flex items-center gap-2">
            Configure how alerts are delivered (Slack, email, push).
            {role && (
              <span className="inline-flex items-center gap-1 text-[11px] font-medium px-2 py-0.5 rounded-full bg-gray-100 text-gray-600">
                role: {role}
              </span>
            )}
          </p>
        </div>
        {writable && (
          <button
            onClick={openAdd}
            className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 shadow-sm"
          >
            + Add channel
          </button>
        )}
      </div>

      {!writable && (
        <div className="bg-amber-50 border border-amber-200 rounded-lg px-4 py-2.5 text-sm text-amber-800 flex items-center gap-2">
          <span className="text-amber-600">ⓘ</span>
          You have read-only access. Contact an OWNER or ADMIN to manage channels.
        </div>
      )}

      {loadingChannels ? (
        <p className="text-gray-500">Loading...</p>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {channels?.map((c) => (
            <div key={c.id} className="bg-white border border-gray-200 rounded-xl p-5 shadow-sm hover:shadow-md transition-shadow">
              <div className="flex items-start justify-between mb-3">
                <div className="flex-1 min-w-0">
                  <p className="font-semibold text-gray-900 truncate">{c.name}</p>
                  <span
                    className={`inline-block mt-1 text-[10px] font-medium px-2 py-0.5 rounded-full border ${TYPE_COLOR[c.type]}`}
                  >
                    {TYPE_LABELS[c.type]}
                  </span>
                </div>
                <span
                  className={`w-2 h-2 rounded-full mt-1 ${c.enabled ? "bg-emerald-500" : "bg-gray-300"}`}
                  title={c.enabled ? "Enabled" : "Disabled"}
                />
              </div>

              <div className="text-xs text-gray-500 font-mono break-all mb-3 min-h-[2rem]">
                {c.type === "webhook" && (c.config?.url ?? "—")}
                {c.type === "email" && (Array.isArray(c.config?.to) ? c.config.to.join(", ") : "—")}
                {c.type === "fcm" && (c.config?.topic ? `topic: ${c.config.topic}` : `token: ${(c.config?.token ?? "").slice(0, 24)}…`)}
              </div>

              {writable ? (
                <div className="flex items-center gap-2 pt-3 border-t border-gray-100">
                  <button
                    onClick={() => testMut.mutate(c.id)}
                    disabled={testMut.isPending && testMut.variables === c.id}
                    className="text-xs px-2.5 py-1.5 rounded-md text-gray-700 border border-gray-200 hover:bg-gray-50 disabled:opacity-50"
                  >
                    {testMut.isPending && testMut.variables === c.id ? "Testing…" : "Send test"}
                  </button>
                  {testFeedback?.id === c.id && (
                    <span className={`text-xs ${testFeedback.ok ? "text-emerald-600" : "text-rose-600"}`}>
                      {testFeedback.ok ? "✓ queued" : "✗ failed"}
                    </span>
                  )}
                  <div className="ml-auto flex gap-1">
                    <button
                      onClick={() => openEdit(c)}
                      className="text-xs px-2.5 py-1.5 rounded-md text-blue-700 hover:bg-blue-50"
                    >
                      Edit
                    </button>
                    <button
                      onClick={() => setDeleteId(c.id)}
                      className="text-xs px-2.5 py-1.5 rounded-md text-rose-700 hover:bg-rose-50"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ) : (
                <div className="pt-3 border-t border-gray-100 text-[11px] text-gray-400">
                  Read-only
                </div>
              )}
            </div>
          ))}
          {channels?.length === 0 && (
            <div className="col-span-full bg-white border border-dashed border-gray-300 rounded-xl p-10 text-center">
              <p className="text-sm text-gray-500 mb-3">No channels configured yet.</p>
              {writable && (
                <button
                  onClick={openAdd}
                  className="text-sm text-blue-600 hover:text-blue-700 font-medium"
                >
                  + Configure your first channel
                </button>
              )}
            </div>
          )}
        </div>
      )}

      <div>
        <h2 className="text-lg font-semibold text-gray-900 mb-4">Recent Events</h2>
        {loadingEvents ? (
          <p className="text-gray-500">Loading...</p>
        ) : (
          <div className="bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  {["Channel", "Title", "Status", "Time"].map((h) => (
                    <th key={h} className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {events?.map((ev) => (
                  <tr key={ev.id} className="hover:bg-gray-50">
                    <td className="px-4 py-3 text-sm text-gray-700">{ev.channel_name}</td>
                    <td className="px-4 py-3 text-sm text-gray-900">{ev.title}</td>
                    <td className="px-4 py-3">
                      <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${statusColor[ev.status] ?? "bg-gray-100 text-gray-600"}`}>
                        {ev.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-400">
                      {new Date(ev.created_at).toLocaleString()}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {events?.length === 0 && (
              <p className="text-center text-gray-400 py-8">No notification events yet.</p>
            )}
          </div>
        )}
      </div>

      {modal && (
        <ChannelModal
          title={modal === "add" ? "Add notification channel" : "Edit channel"}
          form={form}
          onChange={setForm}
          onSave={handleSave}
          onClose={closeModal}
          saving={createMut.isPending || updateMut.isPending}
          error={formError}
          isEdit={modal === "edit"}
        />
      )}

      {deleteId !== null && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
          <div className="bg-white rounded-xl shadow-xl w-full max-w-sm p-6">
            <h3 className="text-lg font-semibold text-gray-900 mb-2">Delete channel?</h3>
            <p className="text-sm text-gray-600 mb-4">
              This action cannot be undone. Rules using this channel will fail to send.
            </p>
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setDeleteId(null)}
                className="px-4 py-2 text-sm text-gray-700 border border-gray-300 rounded-lg hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={() => deleteMut.mutate(deleteId)}
                disabled={deleteMut.isPending}
                className="px-4 py-2 text-sm text-white bg-rose-600 rounded-lg hover:bg-rose-700 disabled:opacity-50"
              >
                {deleteMut.isPending ? "Deleting…" : "Delete"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
