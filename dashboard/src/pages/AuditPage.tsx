import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";

interface AuditLog {
  id: number;
  actor_username: string | null;
  action: string;
  resource_type: string;
  resource_id: string;
  ip: string;
  ts: string;
}

const actionColor: Record<string, string> = {
  create: "bg-green-100 text-green-700",
  update: "bg-blue-100 text-blue-700",
  delete: "bg-red-100 text-red-700",
};

export default function AuditPage() {
  const tenant = localStorage.getItem("tenant_slug") ?? "";
  const { data: logs, isLoading } = useQuery<AuditLog[]>({
    queryKey: ["audit", tenant],
    queryFn: () => api.get("/v1/audit/").then((r) => r.data),
    refetchInterval: 30_000,
  });

  if (isLoading) return <p className="text-gray-500">Loading audit log...</p>;

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 mb-6">Audit Log</h1>
      <div className="bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden">
        <table className="min-w-full divide-y divide-gray-200">
          <thead className="bg-gray-50">
            <tr>
              {["Actor", "Action", "Resource", "ID", "IP", "Time"].map((h) => (
                <th key={h} className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100">
            {logs?.map((log) => (
              <tr key={log.id} className="hover:bg-gray-50">
                <td className="px-4 py-3 text-sm text-gray-700">{log.actor_username ?? "system"}</td>
                <td className="px-4 py-3">
                  <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${actionColor[log.action] ?? "bg-gray-100 text-gray-600"}`}>
                    {log.action}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-gray-700">{log.resource_type}</td>
                <td className="px-4 py-3 text-sm font-mono text-gray-500">{log.resource_id}</td>
                <td className="px-4 py-3 text-sm text-gray-400">{log.ip}</td>
                <td className="px-4 py-3 text-sm text-gray-400">{new Date(log.ts).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {logs?.length === 0 && (
          <p className="text-center text-gray-400 py-8">No audit entries yet.</p>
        )}
      </div>
    </div>
  );
}
