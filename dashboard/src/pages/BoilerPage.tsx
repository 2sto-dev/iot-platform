import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, goApi } from "../lib/api";
import { useDeviceMetrics } from "../components/solar/useDeviceMetrics";
import { RANGE_OPTIONS } from "../components/solar/types";
import { canSendCommands } from "../lib/auth";

interface Device {
  id: number;
  serial_number: string;
  description: string;
  device_type: string;
}

// Field-urile Tasmota au prefix `nousat_` ca sa evite type-conflict cu alte
// device-uri din aceeasi masuratoare 'devices' (ex: `power` a fost cuplat cu
// string ON/OFF pe alta cale). RSSI vine din STATE handler, fara prefix.
const FIELDS = [
  "nousat_power", "nousat_voltage", "nousat_current",
  "nousat_total", "nousat_today", "nousat_yesterday",
  "nousat_power_factor",
  "rssi",
];

export default function BoilerPage() {
  const [selectedSerial, setSelectedSerial] = useState<string | null>(null);
  const [range, setRange] = useState("-5m");
  const tenant = localStorage.getItem("tenant_slug") ?? "";

  const { data: devices, isLoading } = useQuery<Device[]>({
    queryKey: ["devices", tenant],
    queryFn: () => api.get("/devices/").then((r) => r.data),
  });

  const nousat = useMemo(
    () => (devices ?? []).filter((d) => d.device_type === "nous_at"),
    [devices]
  );

  const activeSerial = selectedSerial ?? nousat[0]?.serial_number ?? null;
  const activeDevice = nousat.find((d) => d.serial_number === activeSerial);

  const { values: m } = useDeviceMetrics(activeSerial, FIELDS, range);
  const power = m["nousat_power"];          // W
  const voltage = m["nousat_voltage"];      // V
  const current = m["nousat_current"];      // A
  const total = m["nousat_total"];          // kWh lifetime
  const today = m["nousat_today"];          // kWh azi (resetează la 00:00)
  const yesterday = m["nousat_yesterday"];  // kWh ieri
  const powerFactor = m["nousat_power_factor"];
  const rssi = m["rssi"];                   // dBm (de la Tasmota STATE)

  const isOn = power !== null && power > 1; // > 1W = consumă

  // ── Toggle ON/OFF cu confirmare ─────────────────────────────────────────
  const cmdable = canSendCommands();
  const [pending, setPending] = useState<{ desired: "ON" | "OFF"; t0: number } | null>(null);
  const [feedback, setFeedback] = useState<{ kind: "ok" | "fail" | "timeout"; msg: string } | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  function clearPoll() {
    if (pollRef.current) {
      clearInterval(pollRef.current);
      pollRef.current = null;
    }
  }

  async function sendToggle(desired: "ON" | "OFF") {
    if (!activeDevice || !cmdable || pending) return;
    setFeedback(null);
    try {
      await api.post(`/devices/${activeDevice.id}/relay/`, { state: desired });
      const t0 = Date.now();
      const expected = desired === "ON" ? 1 : 0;
      setPending({ desired, t0 });

      // Polling explicit pe `relay_on` (1/0) — Tasmota publica STATE imediat dupa
      // Backlog command, asa ca in 1-3s ar trebui sa avem valoarea actualizata.
      pollRef.current = setInterval(async () => {
        try {
          const r = await goApi.get(`/metrics/${activeSerial}/relay_on`, {
            params: { range: "-30s" },
          });
          const val = r.data?.value;
          if (typeof val === "number" && val === expected) {
            clearPoll();
            setPending(null);
            setFeedback({ kind: "ok", msg: `Confirmat: relay = ${desired}` });
            setTimeout(() => setFeedback(null), 4000);
            return;
          }
        } catch {
          // 'no data' poate aparea daca STATE nu a ajuns inca; continuam polling
        }

        // Timeout 12s (Tasmota uneori intarzie pana la 3-5s)
        if (Date.now() - t0 > 12_000) {
          clearPoll();
          setPending(null);
          setFeedback({
            kind: "timeout",
            msg: "Comanda trimisă dar fără confirmare în 12s — verifică manual",
          });
          setTimeout(() => setFeedback(null), 6000);
        }
      }, 800);
    } catch (e: any) {
      const msg = e?.response?.data?.detail ?? e?.message ?? "Comandă eșuată";
      setFeedback({ kind: "fail", msg });
      setTimeout(() => setFeedback(null), 6000);
    }
  }

  useEffect(() => () => clearPoll(), []);

  if (isLoading) {
    return (
      <div className="animate-pulse">
        <div className="h-8 bg-gray-200 rounded w-48 mb-6" />
        <div className="grid grid-cols-2 gap-4">
          {[1, 2, 3, 4].map((i) => <div key={i} className="h-24 bg-gray-100 rounded-xl" />)}
        </div>
      </div>
    );
  }

  if (nousat.length === 0) {
    return (
      <div>
        <h1 className="text-2xl font-bold text-gray-900 mb-2">Boiler</h1>
        <p className="text-sm text-gray-500 mb-6">Tasmota / Nous A1T smart plug monitoring.</p>
        <div className="bg-white border border-amber-200 rounded-xl p-8 text-center">
          <div className="inline-flex items-center justify-center w-12 h-12 bg-amber-50 rounded-full mb-3">
            <span className="text-xl">🔌</span>
          </div>
          <h3 className="text-base font-semibold text-gray-900 mb-1">No Nous A1T devices</h3>
          <p className="text-sm text-gray-500">
            Register a device with type "Nous AT" on the Devices page.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="max-w-[1400px]">
      {/* Header */}
      <div className="flex flex-wrap items-end justify-between gap-4 mb-6 pb-4 border-b border-gray-200">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 tracking-tight">Boiler</h1>
          <p className="text-sm text-gray-500 mt-1 flex items-center gap-2 flex-wrap">
            <span className="font-mono text-gray-700">{activeSerial}</span>
            {activeDevice?.description && (
              <>
                <span className="text-gray-300">·</span>
                <span>{activeDevice.description}</span>
              </>
            )}
            <span className="text-gray-300">·</span>
            <span>last {RANGE_OPTIONS.find((r) => r.value === range)?.label}</span>
            <span className={`inline-flex items-center gap-1 ml-1 ${isOn ? "text-emerald-600" : "text-gray-400"}`}>
              <span className={`w-1.5 h-1.5 rounded-full ${isOn ? "bg-emerald-500 animate-pulse" : "bg-gray-300"}`} />
              {isOn ? "running" : "idle"}
            </span>
          </p>
        </div>
        <div className="flex gap-2">
          {nousat.length > 1 && (
            <select
              value={activeSerial ?? ""}
              onChange={(e) => setSelectedSerial(e.target.value)}
              className="border border-gray-300 rounded-lg px-3 py-2 text-sm bg-white shadow-sm focus:outline-none focus:ring-2 focus:ring-rose-500"
            >
              {nousat.map((d) => (
                <option key={d.id} value={d.serial_number}>{d.serial_number}</option>
              ))}
            </select>
          )}
          <select
            value={range}
            onChange={(e) => setRange(e.target.value)}
            className="border border-gray-300 rounded-lg px-3 py-2 text-sm bg-white shadow-sm focus:outline-none focus:ring-2 focus:ring-rose-500"
          >
            {RANGE_OPTIONS.map((r) => <option key={r.value} value={r.value}>{r.label}</option>)}
          </select>
        </div>
      </div>

      {/* Toggle feedback banner */}
      {feedback && (
        <div className={`mb-4 rounded-lg px-4 py-2.5 text-sm flex items-center gap-2 ${
          feedback.kind === "ok" ? "bg-emerald-50 text-emerald-800 border border-emerald-200" :
          feedback.kind === "timeout" ? "bg-amber-50 text-amber-800 border border-amber-200" :
          "bg-rose-50 text-rose-800 border border-rose-200"
        }`}>
          <span>{feedback.kind === "ok" ? "✓" : feedback.kind === "timeout" ? "⏱" : "✗"}</span>
          {feedback.msg}
        </div>
      )}

      {/* Hero state — Power */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
        <div className={`lg:col-span-2 rounded-xl p-6 border transition-colors ${
          isOn
            ? "bg-gradient-to-br from-rose-50 to-amber-50 border-rose-200"
            : "bg-white border-gray-200"
        }`}>
          <div className="flex items-center justify-between mb-2">
            <h3 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Current draw</h3>
            <span className={`inline-flex items-center gap-1 text-[11px] font-medium px-2 py-0.5 rounded-full ${
              isOn ? "bg-rose-100 text-rose-700" : "bg-gray-100 text-gray-500"
            }`}>
              {isOn ? "🔥 heating" : "⏸ standby"}
            </span>
          </div>
          <p className="flex items-baseline gap-2 mt-3">
            <span className={`text-6xl font-extrabold tabular-nums ${
              power === null ? "text-gray-300" : isOn ? "text-rose-700" : "text-gray-700"
            }`}>
              {power !== null ? power.toFixed(0) : "—"}
            </span>
            <span className="text-xl font-medium text-gray-500">W</span>
          </p>
          <p className="text-xs text-gray-500 mt-3">
            Putere instantanee consumată de boiler
          </p>

          {/* Control: toggle ON/OFF */}
          {cmdable && (
            <div className="mt-5 pt-4 border-t border-gray-200/60 flex items-center gap-3 flex-wrap">
              <span className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Control</span>
              <button
                onClick={() => sendToggle("ON")}
                disabled={pending !== null}
                className={`px-4 py-2 rounded-lg text-sm font-semibold shadow-sm transition-all ${
                  isOn
                    ? "bg-emerald-600 text-white ring-2 ring-emerald-300"
                    : "bg-white border border-gray-300 text-gray-700 hover:bg-emerald-50 hover:border-emerald-400 hover:text-emerald-700"
                } disabled:opacity-50 disabled:cursor-wait`}
              >
                {pending?.desired === "ON" ? (
                  <span className="inline-flex items-center gap-2">
                    <Spinner /> Pornește…
                  </span>
                ) : "● ON"}
              </button>
              <button
                onClick={() => sendToggle("OFF")}
                disabled={pending !== null}
                className={`px-4 py-2 rounded-lg text-sm font-semibold shadow-sm transition-all ${
                  !isOn && power !== null
                    ? "bg-gray-700 text-white ring-2 ring-gray-300"
                    : "bg-white border border-gray-300 text-gray-700 hover:bg-gray-100"
                } disabled:opacity-50 disabled:cursor-wait`}
              >
                {pending?.desired === "OFF" ? (
                  <span className="inline-flex items-center gap-2">
                    <Spinner /> Oprește…
                  </span>
                ) : "○ OFF"}
              </button>
              {pending && (
                <span className="text-[11px] text-gray-500 ml-auto">
                  aștept confirmare {Math.max(0, Math.ceil((10000 - (Date.now() - pending.t0)) / 1000))}s
                </span>
              )}
            </div>
          )}
        </div>

        {/* Energy stats: Today / Yesterday / Total */}
        <div className="bg-white border border-gray-200 rounded-xl p-6 flex flex-col justify-center space-y-3">
          <div>
            <h3 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Today</h3>
            <p className="flex items-baseline gap-1.5 mt-1">
              <span className="text-2xl font-bold tabular-nums text-emerald-700">
                {today !== null ? today.toFixed(3) : "—"}
              </span>
              <span className="text-sm font-medium text-gray-500">kWh</span>
            </p>
          </div>
          <div className="grid grid-cols-2 gap-3 pt-3 border-t border-gray-100">
            <div>
              <p className="text-[10px] font-semibold text-gray-500 uppercase tracking-wider">Yesterday</p>
              <p className="text-base font-bold tabular-nums text-gray-700 mt-0.5">
                {yesterday !== null ? yesterday.toFixed(2) : "—"}
                <span className="text-[10px] font-medium text-gray-400 ml-1">kWh</span>
              </p>
            </div>
            <div>
              <p className="text-[10px] font-semibold text-gray-500 uppercase tracking-wider">Total</p>
              <p className="text-base font-bold tabular-nums text-gray-700 mt-0.5">
                {total !== null ? total.toFixed(1) : "—"}
                <span className="text-[10px] font-medium text-gray-400 ml-1">kWh</span>
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Voltage / Current / Power Factor / RSSI */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <MetricMini
          label="Voltage"
          value={voltage}
          unit="V"
          decimals={1}
          accent="indigo"
          icon={<VoltageIcon />}
        />
        <MetricMini
          label="Current"
          value={current}
          unit="A"
          decimals={2}
          accent="amber"
          icon={<CurrentIcon />}
        />
        <MetricMini
          label="Power factor"
          value={powerFactor}
          unit=""
          decimals={2}
          accent="emerald"
          icon={<PowerFactorIcon />}
        />
        <MetricMini
          label="WiFi signal"
          value={rssi}
          unit="dBm"
          decimals={0}
          accent={rssi !== null && rssi > -60 ? "emerald" : rssi !== null && rssi > -75 ? "amber" : "rose"}
          icon={<WifiIcon />}
          hint={rssi !== null ? (rssi > -60 ? "Excellent" : rssi > -75 ? "Good" : "Weak") : undefined}
        />
      </div>

      {/* Footer */}
      <div className="text-xs text-gray-400 mt-8 pt-4 border-t border-gray-200 flex flex-wrap items-center gap-2">
        <span className="w-1.5 h-1.5 bg-emerald-500 rounded-full" />
        Auto-refresh la 5 secunde
        <span className="text-gray-300">·</span>
        <span>Sursă: Tasmota SENSOR / STATE prin mqtt-bridge</span>
      </div>
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// MetricMini — card mic cu icon + label + value + unit + accent stripe
// ──────────────────────────────────────────────────────────────────────────
type Accent = "indigo" | "amber" | "emerald" | "rose" | "slate";

const ACCENT: Record<Accent, { bar: string; bg: string; text: string }> = {
  indigo:  { bar: "bg-indigo-500",  bg: "bg-indigo-50",  text: "text-indigo-600" },
  amber:   { bar: "bg-amber-500",   bg: "bg-amber-50",   text: "text-amber-600" },
  emerald: { bar: "bg-emerald-500", bg: "bg-emerald-50", text: "text-emerald-600" },
  rose:    { bar: "bg-rose-500",    bg: "bg-rose-50",    text: "text-rose-600" },
  slate:   { bar: "bg-slate-400",   bg: "bg-slate-50",   text: "text-slate-500" },
};

function MetricMini({
  label, value, unit, decimals = 2, accent, icon, hint,
}: {
  label: string;
  value: number | null;
  unit: string;
  decimals?: number;
  accent: Accent;
  icon: React.ReactNode;
  hint?: string;
}) {
  const a = ACCENT[accent];
  return (
    <div className="bg-white border border-gray-200 rounded-xl overflow-hidden flex shadow-sm">
      <span className={`w-1 ${a.bar}`} />
      <div className="flex-1 px-5 py-4">
        <div className="flex items-center gap-2">
          <span className={`w-7 h-7 rounded-full flex items-center justify-center ${a.bg} ${a.text}`}>
            {icon}
          </span>
          <p className="text-[11px] font-semibold text-gray-500 uppercase tracking-wider">{label}</p>
          {hint && <span className={`ml-auto text-[10px] font-medium ${a.text}`}>{hint}</span>}
        </div>
        <p className="mt-2 flex items-baseline gap-1.5">
          <span className={`text-2xl font-bold tabular-nums ${value === null ? "text-gray-300" : "text-gray-900"}`}>
            {value !== null ? value.toFixed(decimals) : "—"}
          </span>
          <span className="text-sm font-medium text-gray-500">{unit}</span>
        </p>
      </div>
    </div>
  );
}

function VoltageIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M13 2 L4 14 L11 14 L11 22 L20 10 L13 10 Z" fill="currentColor" />
    </svg>
  );
}

function CurrentIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 12 Q 7 6, 12 12 T 21 12" />
    </svg>
  );
}

function Spinner() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round">
      <circle cx="12" cy="12" r="9" strokeOpacity="0.25" />
      <path d="M21 12 a 9 9 0 0 0 -9 -9">
        <animateTransform
          attributeName="transform"
          type="rotate"
          from="0 12 12"
          to="360 12 12"
          dur="0.8s"
          repeatCount="indefinite"
        />
      </path>
    </svg>
  );
}

function PowerFactorIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12 L21 12 M12 3 L12 21" strokeOpacity="0.4" />
    </svg>
  );
}

function WifiIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M5 12 a 10 10 0 0 1 14 0" />
      <path d="M8.5 15.5 a 5 5 0 0 1 7 0" />
      <circle cx="12" cy="19" r="1.2" fill="currentColor" />
    </svg>
  );
}
