import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { canWrite } from "../lib/auth";

interface Device {
  id: number;
  serial_number: string;
  description: string;
  device_type: string;
  tenant: number;
  tenant_plan: string;
  topics: Record<string, string>;
}

const DEVICE_TYPES = [
  { value: "shelly_em", label: "Shelly EM" },
  { value: "nous_at", label: "Nous A1T" },
  { value: "zigbee_sensor", label: "Zigbee Sensor" },
  { value: "auto_detected", label: "Auto Detected" },
  { value: "sun2000", label: "Huawei SUN2000" },
];

interface FormState {
  serial_number: string;
  description: string;
  device_type: string;
}

const EMPTY_FORM: FormState = { serial_number: "", description: "", device_type: "auto_detected" };

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

function DeviceModal({ title, form, onChange, onSave, onClose, saving, error, isEdit }: ModalProps) {
  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6">
        <h2 className="text-lg font-semibold text-gray-900 mb-4">{title}</h2>
        {error && <p className="mb-3 text-sm text-red-600 bg-red-50 rounded px-3 py-2">{error}</p>}
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Serial number</label>
            <input
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:bg-gray-100"
              value={form.serial_number}
              onChange={(e) => onChange({ ...form, serial_number: e.target.value })}
              disabled={isEdit}
              placeholder="e.g. SHELF001"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Description</label>
            <input
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              value={form.description}
              onChange={(e) => onChange({ ...form, description: e.target.value })}
              placeholder="Optional"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Device type</label>
            <select
              className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              value={form.device_type}
              onChange={(e) => onChange({ ...form, device_type: e.target.value })}
            >
              {DEVICE_TYPES.map((t) => (
                <option key={t.value} value={t.value}>{t.label}</option>
              ))}
            </select>
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
            disabled={saving || !form.serial_number.trim()}
            className="px-4 py-2 text-sm text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50"
          >
            {saving ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default function DevicesPage() {
  const qc = useQueryClient();
  const tenant = localStorage.getItem("tenant_slug") ?? "";
  const writable = canWrite();
  const [modal, setModal] = useState<"add" | "edit" | null>(null);
  const [editTarget, setEditTarget] = useState<Device | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [formError, setFormError] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<number | null>(null);

  const { data: devices, isLoading, error } = useQuery<Device[]>({
    queryKey: ["devices", tenant],
    queryFn: () => api.get("/devices/").then((r) => r.data),
    refetchInterval: 30_000,
  });

  const createMut = useMutation({
    mutationFn: (body: FormState) => api.post("/devices/", body),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["devices", tenant] }); closeModal(); },
    onError: (e: any) => setFormError(extractError(e)),
  });

  const updateMut = useMutation({
    mutationFn: ({ id, body }: { id: number; body: Partial<FormState> }) =>
      api.patch(`/devices/${id}/`, body),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["devices", tenant] }); closeModal(); },
    onError: (e: any) => setFormError(extractError(e)),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => api.delete(`/devices/${id}/`),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["devices", tenant] }); setDeleteId(null); },
  });

  function openAdd() {
    setForm(EMPTY_FORM);
    setFormError(null);
    setModal("add");
  }

  function openEdit(d: Device) {
    setEditTarget(d);
    setForm({ serial_number: d.serial_number, description: d.description ?? "", device_type: d.device_type });
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
    if (modal === "add") {
      createMut.mutate(form);
    } else if (modal === "edit" && editTarget) {
      updateMut.mutate({ id: editTarget.id, body: { description: form.description, device_type: form.device_type } });
    }
  }

  function extractError(e: any): string {
    const data = e?.response?.data;
    const status = e?.response?.status;
    if (!data) return `Request failed (${status ?? "no response"}).`;
    if (typeof data === "string") {
      if (data.trimStart().startsWith("<")) return `Server error ${status} — check Django logs.`;
      return data;
    }
    const msgs = Object.entries(data).map(([k, v]) => `${k}: ${Array.isArray(v) ? v.join(", ") : v}`);
    return msgs.join(" | ");
  }

  if (isLoading) return <p className="text-gray-500">Loading devices...</p>;
  if (error) return <p className="text-red-500">Failed to load devices.</p>;

  return (
    <div>
      {modal && (
        <DeviceModal
          title={modal === "add" ? "Add device" : "Edit device"}
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
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-white rounded-xl shadow-xl p-6 max-w-sm w-full">
            <h2 className="text-lg font-semibold text-gray-900 mb-2">Delete device?</h2>
            <p className="text-sm text-gray-600 mb-6">This action cannot be undone.</p>
            <div className="flex justify-end gap-3">
              <button onClick={() => setDeleteId(null)} className="px-4 py-2 text-sm text-gray-700 border border-gray-300 rounded-lg hover:bg-gray-50">
                Cancel
              </button>
              <button
                onClick={() => deleteMut.mutate(deleteId!)}
                disabled={deleteMut.isPending}
                className="px-4 py-2 text-sm text-white bg-red-600 rounded-lg hover:bg-red-700 disabled:opacity-50"
              >
                {deleteMut.isPending ? "Deleting…" : "Delete"}
              </button>
            </div>
          </div>
        </div>
      )}

      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Devices</h1>
        {writable && (
          <button
            onClick={openAdd}
            className="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700"
          >
            + Add device
          </button>
        )}
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              {["Serial", "Description", "Type", "Plan", ""].map((h, i) => (
                <th key={i} className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {devices?.map((d) => (
              <tr key={d.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 text-sm font-mono text-gray-900">{d.serial_number}</td>
                <td className="px-4 py-3 text-sm text-gray-700">{d.description || "—"}</td>
                <td className="px-4 py-3 text-sm text-gray-500">
                  {DEVICE_TYPES.find((t) => t.value === d.device_type)?.label ?? d.device_type}
                </td>
                <td className="px-4 py-3">
                  <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-100 text-blue-700">
                    {d.tenant_plan}
                  </span>
                </td>
                <td className="px-4 py-3 text-right">
                  {writable ? (
                    <div className="inline-flex gap-2">
                      <button
                        onClick={() => openEdit(d)}
                        className="text-xs text-blue-600 hover:underline"
                      >
                        Edit
                      </button>
                      <button
                        onClick={() => setDeleteId(d.id)}
                        className="text-xs text-red-500 hover:underline"
                      >
                        Delete
                      </button>
                    </div>
                  ) : (
                    <span className="text-[11px] text-gray-400">read-only</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {devices?.length === 0 && (
          <p className="text-center text-gray-400 py-8">No devices registered.</p>
        )}
      </div>
    </div>
  );
}
