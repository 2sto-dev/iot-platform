import axios from "axios";

export interface Tenant {
  slug: string;
  name: string;
  plan: string;
  role: string;
}

export async function fetchTenants(username: string, password: string): Promise<Tenant[]> {
  const { data } = await axios.post("/api/auth/tenants/", { username, password });
  return data;
}

export async function login(username: string, password: string, tenantSlug: string) {
  const { data } = await axios.post("/api/token/", { username, password, tenant_slug: tenantSlug });
  localStorage.setItem("access_token", data.access);
  localStorage.setItem("refresh_token", data.refresh);
  localStorage.setItem("tenant_slug", tenantSlug);
  if (data.role) localStorage.setItem("role", data.role);
}

export function logout() {
  localStorage.clear();
  window.location.href = "/login";
}

export function isAuthenticated(): boolean {
  return !!localStorage.getItem("access_token");
}

export type Role = "OWNER" | "ADMIN" | "OPERATOR" | "VIEWER" | "INSTALLER";

export function getRole(): Role | null {
  const r = localStorage.getItem("role");
  return r ? (r as Role) : null;
}

const WRITE_ROLES: Role[] = ["OWNER", "ADMIN"];
const COMMAND_ROLES: Role[] = ["OWNER", "ADMIN", "OPERATOR"];

/** OWNER + ADMIN — pot crea/edita/șterge resurse (devices, channels, rules). */
export function canWrite(): boolean {
  const r = getRole();
  return r !== null && WRITE_ROLES.includes(r);
}

/** OWNER + ADMIN + OPERATOR — pot trimite comenzi downlink. */
export function canSendCommands(): boolean {
  const r = getRole();
  return r !== null && COMMAND_ROLES.includes(r);
}
