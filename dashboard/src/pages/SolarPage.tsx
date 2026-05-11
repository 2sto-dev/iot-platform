import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import { InverterPanel } from "../components/solar/InverterPanel";
import { BatteryPanel } from "../components/solar/BatteryPanel";
import { EnergyFlowDiagram } from "../components/solar/EnergyFlowDiagram";
import { useDeviceMetrics } from "../components/solar/useDeviceMetrics";
import { RANGE_OPTIONS } from "../components/solar/types";

interface Device {
  id: number;
  serial_number: string;
  description: string;
  device_type: string;
}

const SUMMARY_FIELDS = [
  "house_load_kw_est",
  "grid_power",
  "pv_input_power",
  "battery_power",
  "battery_soc",
  "internal_temp",
  "battery_temp",
];

type SectionTone = "amber" | "indigo" | "sky" | "rose";

const SECTION_TONES: Record<SectionTone, { bar: string; text: string; sub: string }> = {
  amber:  { bar: "bg-amber-500",  text: "text-amber-900",  sub: "text-amber-700" },
  indigo: { bar: "bg-indigo-500", text: "text-indigo-900", sub: "text-indigo-700" },
  sky:    { bar: "bg-sky-500",    text: "text-sky-900",    sub: "text-sky-700" },
  rose:   { bar: "bg-rose-500",   text: "text-rose-900",   sub: "text-rose-700" },
};

export default function SolarPage() {
  const [selectedSerial, setSelectedSerial] = useState<string | null>(null);
  const [range, setRange] = useState("-5m");
  const tenant = localStorage.getItem("tenant_slug") ?? "";

  const { data: devices, isLoading: devicesLoading } = useQuery<Device[]>({
    queryKey: ["devices", tenant],
    queryFn: () => api.get("/devices/").then((r) => r.data),
  });

  const sun2000 = useMemo(
    () => (devices ?? []).filter((d) => d.device_type === "sun2000"),
    [devices]
  );

  const activeSerial = selectedSerial ?? sun2000[0]?.serial_number ?? null;

  const { values: summary } = useDeviceMetrics(activeSerial, SUMMARY_FIELDS, range);
  const houseLoad = summary["house_load_kw_est"];
  const gridPower = summary["grid_power"];
  const pvPower = summary["pv_input_power"];
  const batteryPower = summary["battery_power"];
  const batterySoc = summary["battery_soc"];
  const inverterTemp = summary["internal_temp"];
  const batteryTemp = summary["battery_temp"];

  if (devicesLoading) {
    return (
      <div className="solar-neon animate-pulse">
        <div className="h-8 w-48 mb-6 rounded bg-white/5"></div>
        <div className="grid grid-cols-2 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="h-24 rounded-xl bg-white/5"></div>
          ))}
        </div>
      </div>
    );
  }

  if (sun2000.length === 0) {
    return (
      <div className="solar-neon">
        <h1 className="text-3xl font-bold text-white mb-2 neon-glow-text" style={{ color: "#fdbf3a" }}>Solar</h1>
        <p className="text-sm text-gray-400 mb-6">Huawei SUN2000 inverter monitoring.</p>
        <div className="rounded-xl p-8 text-center neon-card-amber bg-white">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-full mb-3 bg-amber-50">
            <span className="text-xl">☀️</span>
          </div>
          <h3 className="text-base font-semibold text-gray-900 mb-1">No SUN2000 devices</h3>
          <p className="text-sm text-gray-500">Register a device with type "Huawei SUN2000" on the Devices page to start monitoring.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="solar-neon max-w-[1400px]">
      {/* ──── Page header ─────────────────────────────────────────────────── */}
      <div className="flex flex-wrap items-end justify-between gap-4 mb-6 pb-4">
        <div>
          <h1
            className="text-3xl font-bold tracking-tight neon-glow-text"
            style={{ color: "#fdbf3a", textShadow: "0 0 12px rgba(253, 191, 58, 0.6), 0 0 28px rgba(253, 191, 58, 0.35)" }}
          >
            SOLAR
          </h1>
          <p className="text-sm mt-2 flex items-center gap-2 text-gray-500">
            <span className="font-mono" style={{ color: "#5bf2ff" }}>{activeSerial}</span>
            <span className="text-gray-300">·</span>
            <span>Huawei SUN2000</span>
            <span className="text-gray-300">·</span>
            <span>last {RANGE_OPTIONS.find((r) => r.value === range)?.label}</span>
            <span className="inline-flex items-center gap-1.5 ml-2" style={{ color: "#4dffaa" }}>
              <span className="neon-live-dot" />
              <span style={{ textShadow: "0 0 8px rgba(0, 255, 136, 0.6)" }}>LIVE</span>
            </span>
          </p>
        </div>
        <div className="flex gap-2">
          {sun2000.length > 1 && (
            <select
              value={activeSerial ?? ""}
              onChange={(e) => setSelectedSerial(e.target.value)}
              className="rounded-lg px-3 py-2 text-sm shadow-sm"
            >
              {sun2000.map((d) => (
                <option key={d.id} value={d.serial_number}>{d.serial_number}</option>
              ))}
            </select>
          )}
          <select
            value={range}
            onChange={(e) => setRange(e.target.value)}
            className="rounded-lg px-3 py-2 text-sm shadow-sm"
          >
            {RANGE_OPTIONS.map((r) => (
              <option key={r.value} value={r.value}>{r.label}</option>
            ))}
          </select>
        </div>
      </div>

      <div className="neon-divider mb-6" />

      {/* ──── Live overview ───────────────────────────────────────────────── */}
      <SectionHeader
        tone="amber"
        eyebrow="Live overview"
        title="Real-time energy flow"
        subtitle="Animated FusionSolar-style diagram + production / consumption gauges"
      />
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 mb-6">
        <div className="lg:col-span-2 solar-neon-card-in" style={{ animationDelay: "0.05s" }}>
          <EnergyFlowDiagram
            pvPowerKw={pvPower}
            gridPowerW={gridPower}
            batteryPowerKw={batteryPower}
            batterySoc={batterySoc}
            houseLoadKw={houseLoad}
          />
        </div>
        <div className="space-y-4 solar-neon-card-in" style={{ animationDelay: "0.15s" }}>
          <EnergyFlow
            pvKw={pvPower}
            gridW={gridPower}
            batteryKw={batteryPower}
            loadKw={houseLoad}
          />
          <div className="grid grid-cols-2 gap-3">
            <TemperatureCard
              label="Inverter"
              value={inverterTemp}
              warmAt={50}
              hotAt={70}
              icon={<InverterTempIcon />}
            />
            <TemperatureCard
              label="Battery"
              value={batteryTemp}
              warmAt={35}
              hotAt={45}
              icon={<BatteryTempIcon />}
            />
          </div>
        </div>
      </div>

      {/* ──── Production (PV + Inverter) ──────────────────────────────────── */}
      <SectionHeader
        tone="amber"
        eyebrow="Production"
        title="Photovoltaic & Inverter"
        subtitle="DC strings, AC output, daily/total yield, alarms"
      />
      <div className="mb-10 solar-neon-card-in" style={{ animationDelay: "0.25s" }}>
        {activeSerial && <InverterPanel deviceSerial={activeSerial} range={range} showHeroKpis={false} />}
      </div>

      {/* ──── Storage (Battery) ───────────────────────────────────────────── */}
      <SectionHeader
        tone="indigo"
        eyebrow="Storage"
        title="Battery"
        subtitle="State of charge, charge/discharge cycles, temperatures"
      />
      <div className="mb-10 solar-neon-card-in" style={{ animationDelay: "0.35s" }}>
        {activeSerial && <BatteryPanel deviceSerial={activeSerial} range={range} />}
      </div>

      {/* ──── Footer status ───────────────────────────────────────────────── */}
      <div className="neon-divider mb-4" />
      <div className="text-xs mt-2 flex flex-wrap items-center gap-2" style={{ color: "#5a6587" }}>
        <span className="neon-live-dot" style={{ width: 6, height: 6 }} />
        <span>Auto-refreshing every 5 seconds</span>
        <span>·</span>
        <span>Source: Go API → InfluxDB</span>
        <span>·</span>
        <span style={{ color: "#3a4060" }}>Drag cards within sections to reorder</span>
      </div>
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// Section header — bar neon glowing + eyebrow + title
// ──────────────────────────────────────────────────────────────────────────
function SectionHeader({
  tone,
  eyebrow,
  title,
  subtitle,
}: {
  tone: SectionTone;
  eyebrow: string;
  title: string;
  subtitle: string;
}) {
  const c = SECTION_TONES[tone];
  return (
    <div className="mb-5 flex items-center gap-4 px-5 py-4 rounded-xl neon-section-header">
      <span className={`inline-block w-1 h-12 rounded-full ${c.bar}`} />
      <div className="flex-1">
        <p className={`text-[10px] font-bold tracking-[0.2em] uppercase ${c.sub}`}>{eyebrow}</p>
        <h2 className={`text-lg font-bold tracking-tight leading-tight ${c.text}`}>{title}</h2>
        <p className="text-xs text-gray-600 mt-0.5">{subtitle}</p>
      </div>
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// Energy flow card — bilanț pe verticală cu icoane
// ──────────────────────────────────────────────────────────────────────────
type RowTone = "amber" | "indigo" | "emerald" | "rose" | "slate";

const ROW_TONES: Record<RowTone, { bg: string; text: string; ring: string }> = {
  amber:   { bg: "bg-amber-50",   text: "text-amber-600",   ring: "ring-amber-100" },
  indigo:  { bg: "bg-indigo-50",  text: "text-indigo-600",  ring: "ring-indigo-100" },
  emerald: { bg: "bg-emerald-50", text: "text-emerald-600", ring: "ring-emerald-100" },
  rose:    { bg: "bg-rose-50",    text: "text-rose-600",    ring: "ring-rose-100" },
  slate:   { bg: "bg-slate-50",   text: "text-slate-400",   ring: "ring-slate-100" },
};

function IconBubble({ tone, children }: { tone: RowTone; children: React.ReactNode }) {
  const t = ROW_TONES[tone];
  return (
    <div className={`w-9 h-9 rounded-full flex items-center justify-center flex-shrink-0 ring-1 ${t.bg} ${t.ring} ${t.text}`}>
      {children}
    </div>
  );
}

function SolarSvg() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="4" fill="currentColor" fillOpacity="0.2" />
      <line x1="12" y1="2" x2="12" y2="4" /><line x1="12" y1="20" x2="12" y2="22" />
      <line x1="2" y1="12" x2="4" y2="12" /><line x1="20" y1="12" x2="22" y2="12" />
      <line x1="4.93" y1="4.93" x2="6.34" y2="6.34" /><line x1="17.66" y1="17.66" x2="19.07" y2="19.07" />
      <line x1="4.93" y1="19.07" x2="6.34" y2="17.66" /><line x1="17.66" y1="6.34" x2="19.07" y2="4.93" />
    </svg>
  );
}

function GridSvg({ direction }: { direction: "import" | "export" | "idle" }) {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 2 L7 12 L10 12 L7 22 L17 22 L14 12 L17 12 Z" fill="currentColor" fillOpacity="0.15" />
      {direction === "import" && <path d="M14 7 L18 7 M16 5 L18 7 L16 9" />}
      {direction === "export" && <path d="M18 7 L14 7 M16 5 L14 7 L16 9" />}
    </svg>
  );
}

function BatterySvg({ state }: { state: "charging" | "discharging" | "idle" }) {
  return (
    <svg width="22" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="2" y="7" width="18" height="10" rx="2" fill="currentColor" fillOpacity="0.15" />
      <line x1="22" y1="11" x2="22" y2="13" />
      {state === "charging" && <path d="M11 9 L9 12 L13 12 L11 15" stroke="currentColor" strokeWidth="2.2" />}
      {state === "discharging" && <path d="M8 11 L11 11 L11 9 L14 12 L11 15 L11 13 L8 13 Z" fill="currentColor" stroke="none" />}
    </svg>
  );
}

function HouseSvg() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 11 L12 3 L21 11 L21 21 L3 21 Z" fill="currentColor" fillOpacity="0.15" />
      <rect x="10" y="14" width="4" height="7" fill="currentColor" />
    </svg>
  );
}

function EnergyFlow({ pvKw, gridW, batteryKw, loadKw }: {
  pvKw: number | null;
  gridW: number | null;
  batteryKw: number | null;
  loadKw: number | null;
}) {
  const pvOn = pvKw !== null && pvKw > 0.05;
  const exporting = gridW !== null && gridW > 50;
  const importing = gridW !== null && gridW < -50;
  const charging = batteryKw !== null && batteryKw > 0.05;
  const discharging = batteryKw !== null && batteryKw < -0.05;

  const rows = [
    {
      icon: <SolarSvg />,
      tone: (pvOn ? "amber" : "slate") as RowTone,
      label: "Solar production",
      value: pvKw !== null ? `${(pvKw * 1000).toFixed(0)} W` : "—",
      sub: pvKw !== null ? (pvOn ? "Generating" : "Inactive") : "—",
      active: pvOn,
    },
    {
      icon: <GridSvg direction={importing ? "import" : exporting ? "export" : "idle"} />,
      tone: (importing ? "rose" : exporting ? "emerald" : "slate") as RowTone,
      label: importing ? "Grid import" : exporting ? "Grid export" : "Grid",
      value: gridW !== null ? `${Math.abs(gridW).toFixed(0)} W` : "—",
      sub: gridW !== null
        ? (importing ? "Drawing from grid" : exporting ? "Selling to grid" : "Balanced")
        : "—",
      active: importing || exporting,
    },
    {
      icon: <BatterySvg state={charging ? "charging" : discharging ? "discharging" : "idle"} />,
      tone: (charging ? "indigo" : discharging ? "emerald" : "slate") as RowTone,
      label: charging ? "Battery charging" : discharging ? "Battery discharging" : "Battery",
      value: batteryKw !== null ? `${Math.abs(batteryKw * 1000).toFixed(0)} W` : "—",
      sub: batteryKw !== null
        ? (charging ? "Storing energy" : discharging ? "Delivering power" : "Idle")
        : "—",
      active: charging || discharging,
    },
  ];

  return (
    <div className="bg-white rounded-xl p-6 neon-card-cyan">
      <h3 className="text-xs font-semibold uppercase tracking-wider mb-4" style={{ color: "#5bf2ff" }}>
        Energy Balance
      </h3>
      <div className="space-y-1">
        {rows.map((r, i) => (
          <div key={i} className="flex items-center justify-between py-2.5 border-b border-gray-100 last:border-0">
            <div className="flex items-center gap-3 min-w-0">
              <IconBubble tone={r.tone}>{r.icon}</IconBubble>
              <div className="min-w-0">
                <p className="text-sm font-medium text-gray-900 truncate">{r.label}</p>
                <p className="text-[11px] text-gray-500 truncate">{r.sub}</p>
              </div>
            </div>
            <span
              className={`font-semibold tabular-nums text-base ml-2 flex-shrink-0 ${r.active ? "neon-glow-text" : ""}`}
              style={r.active ? { color: r.tone === "amber" ? "#ffd166" : r.tone === "indigo" ? "#b794ff" : r.tone === "emerald" ? "#4dffaa" : r.tone === "rose" ? "#ff6ba8" : "#e0e6ff" } : { color: "#e0e6ff" }}
            >
              {r.value}
            </span>
          </div>
        ))}
        <div className="flex items-center justify-between pt-4 mt-1 border-t-2 border-gray-200">
          <div className="flex items-center gap-3 min-w-0">
            <IconBubble tone="rose"><HouseSvg /></IconBubble>
            <div className="min-w-0">
              <p className="text-sm font-bold text-gray-900">House load</p>
              <p className="text-[11px] text-gray-500">Total consumption</p>
            </div>
          </div>
          <span
            className="font-bold tabular-nums text-lg ml-2 flex-shrink-0 neon-glow-text"
            style={{ color: "#ff6ba8" }}
          >
            {loadKw !== null ? `${(loadKw * 1000).toFixed(0)} W` : "—"}
          </span>
        </div>
      </div>
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// TemperatureCard
// ──────────────────────────────────────────────────────────────────────────
function TemperatureCard({
  label, value, warmAt, hotAt, icon,
}: {
  label: string;
  value: number | null;
  warmAt: number;
  hotAt: number;
  icon: React.ReactNode;
}) {
  const tone: RowTone = value === null
    ? "slate"
    : value >= hotAt ? "rose"
    : value >= warmAt ? "amber"
    : "emerald";
  const status = value === null
    ? "—"
    : value >= hotAt ? "Hot"
    : value >= warmAt ? "Warm"
    : "Normal";
  const pct = value === null ? 0 : Math.max(0, Math.min(1, value / 80));
  const isHot = tone === "rose";

  const neonClass =
    tone === "emerald" ? "neon-card-lime" :
    tone === "amber"   ? "neon-card-amber" :
    tone === "rose"    ? "neon-card-magenta" :
                         "neon-card-cyan";

  const cardClass = isHot
    ? `pulse-warn rounded-xl p-4`
    : `bg-white rounded-xl p-4 ${neonClass}`;

  const valueColor =
    value === null ? "#5a6587" :
    isHot ? "#ff6ba8" :
    tone === "amber" ? "#ffd166" :
    tone === "emerald" ? "#4dffaa" : "#e0e6ff";

  return (
    <div className={cardClass}>
      <div className="flex items-center gap-3">
        <IconBubble tone={tone}>{icon}</IconBubble>
        <div className="flex-1 min-w-0">
          <p className="text-[11px] font-semibold uppercase tracking-wider text-gray-500">{label}</p>
          <p className="flex items-baseline gap-1 mt-0.5">
            <span
              className="text-xl font-bold tabular-nums neon-glow-text"
              style={{ color: valueColor }}
            >
              {value !== null ? value.toFixed(1) : "—"}
            </span>
            <span className="text-xs font-medium text-gray-500">°C</span>
          </p>
        </div>
        <span
          className="text-[10px] font-bold"
          style={{ color: valueColor, textShadow: isHot ? "0 0 8px rgba(255, 60, 142, 0.6)" : undefined }}
        >
          {status}
        </span>
      </div>
      {value !== null && (
        <div className="mt-3 h-1.5 bg-gray-100 rounded-full overflow-hidden">
          <div
            className="h-full transition-all"
            style={{
              width: `${pct * 100}%`,
              background:
                tone === "emerald" ? "linear-gradient(90deg, #4dffaa, #00ff88)" :
                tone === "amber"   ? "linear-gradient(90deg, #ffd166, #ff9500)" :
                tone === "rose"    ? "linear-gradient(90deg, #ff6ba8, #ff2d6e)" :
                                     "rgba(80, 90, 120, 0.5)",
              boxShadow:
                tone === "emerald" ? "0 0 8px rgba(0, 255, 136, 0.6)" :
                tone === "amber"   ? "0 0 8px rgba(253, 191, 58, 0.6)" :
                tone === "rose"    ? "0 0 8px rgba(255, 60, 142, 0.6)" :
                                     "none",
            }}
          />
        </div>
      )}
    </div>
  );
}

function InverterTempIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="6" width="18" height="12" rx="2" fill="currentColor" fillOpacity="0.15" />
      <path d="M7 12 L9 10 L11 13 L13 9 L15 14 L17 11" />
    </svg>
  );
}

function BatteryTempIcon() {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 3 v9 a3 3 0 1 0 0 6 a3 3 0 0 0 0 -6" fill="currentColor" fillOpacity="0.2" />
      <circle cx="12" cy="17" r="2" fill="currentColor" />
    </svg>
  );
}
