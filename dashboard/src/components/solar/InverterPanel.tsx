import { SortableMetricGrid } from "./SortableMetricGrid";
import { useDeviceMetrics } from "./useDeviceMetrics";
import { HEADER_COLORS, type MetricGroup } from "./types";

/**
 * Template invertor — secțiunile PV (DC), Grid (AC), Producție, Smart Meter, Status.
 * Reutilizabil pentru orice device cu metrici Huawei SUN2000-style.
 *
 * Câmpurile sunt definite în `defaultGroups`; pot fi suprascrise prin prop `groups` ca să
 * personalizezi pentru alte modele de invertoare.
 */
export interface InverterPanelProps {
  deviceSerial: string;
  range: string;
  groups?: MetricGroup[];
  /** Afișează un rând de KPI-uri în antet (PV Input, Grid Power, Daily Yield). */
  showHeroKpis?: boolean;
}

export const defaultInverterGroups: MetricGroup[] = [
  {
    key: "pv",
    label: "PV (DC)",
    description: "Curent continuu — panouri fotovoltaice",
    color: "amber",
    metrics: [
      { field: "pv1_voltage", label: "PV1 Voltage", unit: "V", decimals: 1 },
      { field: "pv1_current", label: "PV1 Current", unit: "A", decimals: 2 },
      { field: "pv2_voltage", label: "PV2 Voltage", unit: "V", decimals: 1 },
      { field: "pv2_current", label: "PV2 Current", unit: "A", decimals: 2 },
      { field: "pv_input_power", label: "PV Input Power", unit: "kW", decimals: 3 },
      { field: "insulation_resistance", label: "Insulation R", unit: "MΩ", decimals: 2 },
      { field: "efficiency", label: "Efficiency", unit: "%", decimals: 2 },
    ],
  },
  {
    key: "grid",
    label: "Grid (AC)",
    description: "Curent alternativ — ieșire invertor",
    color: "blue",
    metrics: [
      { field: "grid_power", label: "Grid Power", unit: "W", decimals: 0 },
      { field: "grid_voltage_a", label: "Grid V (A)", unit: "V", decimals: 1 },
      { field: "grid_voltage_b", label: "Grid V (B)", unit: "V", decimals: 1 },
      { field: "grid_voltage_c", label: "Grid V (C)", unit: "V", decimals: 1 },
      { field: "grid_current_a", label: "Grid I (A)", unit: "A", decimals: 2 },
      { field: "grid_current_b", label: "Grid I (B)", unit: "A", decimals: 2 },
      { field: "grid_current_c", label: "Grid I (C)", unit: "A", decimals: 2 },
      { field: "grid_frequency", label: "Frequency", unit: "Hz", decimals: 2 },
      { field: "active_power", label: "Active Power", unit: "kW", decimals: 3 },
      { field: "reactive_power", label: "Reactive Power", unit: "kvar", decimals: 3 },
      { field: "power_factor", label: "Power Factor", unit: "", decimals: 3 },
      { field: "internal_temp", label: "Inverter Temp", unit: "°C", decimals: 1 },
    ],
  },
  {
    key: "production",
    label: "Producție",
    description: "Energie produsă & schimb cu rețeaua",
    color: "emerald",
    metrics: [
      { field: "daily_energy_yield", label: "Daily Yield", unit: "kWh", decimals: 2 },
      { field: "accumulated_energy_yield", label: "Total Yield", unit: "kWh", decimals: 2 },
      { field: "peak_active_power_day", label: "Peak Power Today", unit: "kW", decimals: 3 },
      { field: "grid_energy_exported", label: "Exported", unit: "kWh", decimals: 2 },
      { field: "grid_energy_imported", label: "Imported", unit: "kWh", decimals: 2 },
    ],
  },
  {
    key: "meter",
    label: "Smart Meter",
    description: "Consum casă",
    color: "rose",
    metrics: [
      { field: "house_load_kw_est", label: "House Load", unit: "kW", decimals: 2 },
      { field: "meter_voltage_a", label: "Meter V (A)", unit: "V", decimals: 1 },
      { field: "meter_voltage_b", label: "Meter V (B)", unit: "V", decimals: 1 },
      { field: "meter_voltage_c", label: "Meter V (C)", unit: "V", decimals: 1 },
      { field: "meter_current_a", label: "Meter I (A)", unit: "A", decimals: 2 },
      { field: "meter_current_b", label: "Meter I (B)", unit: "A", decimals: 2 },
      { field: "meter_current_c", label: "Meter I (C)", unit: "A", decimals: 2 },
      { field: "meter_frequency", label: "Meter Freq", unit: "Hz", decimals: 2 },
      { field: "meter_power_factor", label: "Meter PF", unit: "", decimals: 3 },
      { field: "meter_reactive_power", label: "Meter Reactive", unit: "var", decimals: 0 },
    ],
  },
  {
    key: "status",
    label: "Status & Alarme",
    description: "Stare invertor",
    color: "gray",
    metrics: [
      { field: "device_status", label: "Device Status", unit: "", decimals: 0 },
      { field: "meter_status", label: "Meter Status", unit: "", decimals: 0 },
      { field: "fault_code", label: "Fault Code", unit: "", decimals: 0 },
      { field: "alarm_1", label: "Alarm 1", unit: "", decimals: 0 },
      { field: "alarm_2", label: "Alarm 2", unit: "", decimals: 0 },
      { field: "alarm_3", label: "Alarm 3", unit: "", decimals: 0 },
    ],
  },
];

export function InverterPanel({ deviceSerial, range, groups = defaultInverterGroups, showHeroKpis = true }: InverterPanelProps) {
  const allFields = groups.flatMap((g) => g.metrics.map((m) => m.field));
  const { values, errors } = useDeviceMetrics(deviceSerial, allFields, range);

  const pvPower = values["pv_input_power"];
  const gridPower = values["grid_power"];
  const dailyYield = values["daily_energy_yield"];

  return (
    <div>
      {showHeroKpis && (
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
          <HeroKpi label="PV Input" value={pvPower !== null ? pvPower.toFixed(2) : "—"} unit="kW" accent="amber" />
          <HeroKpi
            label={gridPower !== null && gridPower > 0 ? "Grid Import" : gridPower !== null && gridPower < 0 ? "Grid Export" : "Grid"}
            value={gridPower !== null ? Math.abs(gridPower).toFixed(0) : "—"}
            unit="W"
            accent={gridPower !== null && gridPower > 50 ? "rose" : gridPower !== null && gridPower < -50 ? "emerald" : "slate"}
            arrow={gridPower !== null && gridPower > 50 ? "down" : gridPower !== null && gridPower < -50 ? "up" : null}
          />
          <HeroKpi label="Daily Yield" value={dailyYield !== null ? dailyYield.toFixed(2) : "—"} unit="kWh" accent="emerald" />
        </div>
      )}

      {groups.map((g) => {
        const colors = HEADER_COLORS[g.color];
        return (
          <section key={g.key} className="mb-8">
            <div className={`flex items-center gap-3 mb-4 px-4 py-3 rounded-lg border ${colors.bg} ${colors.border}`}>
              <span className={`inline-block w-1 h-10 rounded-full flex-shrink-0 ${colors.bar}`} />
              <div className="flex-1 min-w-0">
                <p className={`text-[10px] font-bold tracking-[0.15em] uppercase ${colors.eyebrow}`}>{g.key}</p>
                <h2 className="text-base font-bold text-gray-900 tracking-tight leading-tight">{g.label}</h2>
                <p className="text-xs text-gray-600 mt-0.5">{g.description}</p>
              </div>
              <span className="text-[10px] text-gray-500 hidden sm:inline self-start mt-1">⠿ drag to reorder</span>
            </div>
            <SortableMetricGrid
              storageKey={`solar-order-inverter-${g.key}`}
              metrics={g.metrics}
              values={values}
              errors={errors}
            />
          </section>
        );
      })}
    </div>
  );
}

type Accent = "amber" | "sky" | "emerald" | "indigo" | "rose" | "slate";

function HeroKpi({ label, value, unit, accent, arrow }: {
  label: string;
  value: string;
  unit: string;
  accent: Accent;
  arrow?: "up" | "down" | null;
}) {
  const accents: Record<Accent, { bar: string; text: string }> = {
    amber: { bar: "bg-amber-500", text: "text-amber-600" },
    sky: { bar: "bg-sky-500", text: "text-sky-600" },
    emerald: { bar: "bg-emerald-500", text: "text-emerald-600" },
    indigo: { bar: "bg-indigo-500", text: "text-indigo-600" },
    rose: { bar: "bg-rose-500", text: "text-rose-600" },
    slate: { bar: "bg-slate-400", text: "text-slate-500" },
  };
  const a = accents[accent];
  const isEmpty = value === "—";
  return (
    <div className="bg-white border border-gray-200 rounded-xl overflow-hidden flex">
      <span className={`w-1 ${a.bar}`} />
      <div className="flex-1 px-5 py-4">
        <div className="flex items-center gap-2">
          <p className="text-[11px] font-semibold text-gray-500 uppercase tracking-wider">{label}</p>
          {arrow && (
            <span className={`text-xs ${a.text}`}>{arrow === "up" ? "↑" : "↓"}</span>
          )}
        </div>
        <p className="mt-2 flex items-baseline gap-1.5">
          <span className={`text-3xl font-bold tabular-nums ${isEmpty ? "text-gray-300" : "text-gray-900"}`}>
            {value}
          </span>
          <span className="text-sm font-medium text-gray-500">{unit}</span>
        </p>
      </div>
    </div>
  );
}
