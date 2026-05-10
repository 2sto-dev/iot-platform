import { useQuery } from "@tanstack/react-query";
import { NavLink, Outlet } from "react-router-dom";
import { api } from "../lib/api";
import { logout } from "../lib/auth";

interface Device {
  id: number;
  serial_number: string;
  device_type: string;
}

const allLinks = [
  { to: "/devices", label: "Devices", requireDevice: null },
  { to: "/solar", label: "Solar", requireDevice: "sun2000" },
  { to: "/boiler", label: "Boiler", requireDevice: "nous_at" },
  { to: "/rules", label: "Rules", requireDevice: null },
  { to: "/notifications", label: "Notifications", requireDevice: null },
  { to: "/audit", label: "Audit Log", requireDevice: null },
];

export default function Layout() {
  const tenant = localStorage.getItem("tenant_slug") ?? "";
  const { data: devices = [] } = useQuery<Device[]>({
    queryKey: ["devices", tenant],
    queryFn: () => api.get("/devices/").then((r) => r.data),
  });

  const links = allLinks.filter((link) => {
    if (!link.requireDevice) return true;
    return devices.some((d) => d.device_type === link.requireDevice);
  });

  return (
    <div className="min-h-screen flex bg-gray-50">
      <aside className="w-56 bg-gray-900 text-white flex flex-col">
        <div className="px-4 py-5 border-b border-gray-700">
          <p className="text-xs text-gray-400 uppercase tracking-wider">Tenant</p>
          <p className="font-semibold truncate">{tenant}</p>
        </div>
        <nav className="flex-1 py-4 space-y-1 px-2">
          {links.map((l) => (
            <NavLink
              key={l.to}
              to={l.to}
              className={({ isActive }) =>
                `block px-3 py-2 rounded text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-blue-600 text-white"
                    : "text-gray-300 hover:bg-gray-700 hover:text-white"
                }`
              }
            >
              {l.label}
            </NavLink>
          ))}
        </nav>
        <div className="p-4 border-t border-gray-700">
          <button
            onClick={logout}
            className="w-full text-left text-sm text-gray-400 hover:text-white transition-colors"
          >
            Log out
          </button>
        </div>
      </aside>
      <main className="flex-1 p-8 overflow-auto">
        <Outlet />
      </main>
    </div>
  );
}
