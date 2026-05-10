import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { fetchTenants, login, type Tenant } from "../lib/auth";

type Step = "credentials" | "tenant";

export default function LoginPage() {
  const navigate = useNavigate();
  const [step, setStep] = useState<Step>("credentials");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleCredentials(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const list = await fetchTenants(username, password);
      if (list.length === 1) {
        await login(username, password, list[0].slug);
        navigate("/", { replace: true });
      } else {
        setTenants(list);
        setStep("tenant");
      }
    } catch {
      setError("Invalid username or password.");
    } finally {
      setLoading(false);
    }
  }

  async function handleTenantSelect(slug: string) {
    setLoading(true);
    try {
      await login(username, password, slug);
      navigate("/", { replace: true });
    } catch {
      setError("Login failed. Please try again.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-100">
      <div className="bg-white rounded-xl shadow-md w-full max-w-sm p-8">
        <h1 className="text-2xl font-bold text-gray-900 mb-6">IoT Platform</h1>

        {step === "credentials" && (
          <form onSubmit={handleCredentials} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Username</label>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
                className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Password</label>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
            </div>
            {error && <p className="text-red-500 text-sm">{error}</p>}
            <button
              type="submit"
              disabled={loading}
              className="w-full bg-blue-600 text-white rounded-lg py-2 text-sm font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
            >
              {loading ? "Loading..." : "Continue"}
            </button>
          </form>
        )}

        {step === "tenant" && (
          <div className="space-y-3">
            <p className="text-sm text-gray-600 mb-4">Select a tenant to continue:</p>
            {tenants.map((t) => (
              <button
                key={t.slug}
                onClick={() => handleTenantSelect(t.slug)}
                disabled={loading}
                className="w-full text-left border border-gray-200 rounded-lg px-4 py-3 hover:border-blue-500 hover:bg-blue-50 transition-colors disabled:opacity-50"
              >
                <p className="font-medium text-gray-900">{t.name}</p>
                <p className="text-xs text-gray-500">
                  {t.plan} · {t.role}
                </p>
              </button>
            ))}
            {error && <p className="text-red-500 text-sm">{error}</p>}
          </div>
        )}
      </div>
    </div>
  );
}
