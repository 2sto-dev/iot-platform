import { SortableMetricGrid } from "./SortableMetricGrid";
import { useDeviceMetrics } from "./useDeviceMetrics";
import { HEADER_COLORS, type MetricGroup } from "./types";

/**
 * Template baterie — SOC, putere, charge/discharge, temperaturi, status.
 * Reutilizabil pentru orice device cu metrici de baterie compatibile (LUNA2000-style).
 */
export interface BatteryPanelProps {
  deviceSerial: string;
  range: string;
  groups?: MetricGroup[];
  /** Afișează rândul de KPI-uri (SOC, Output, Day Charge, Day Discharge). */
  showHeroKpis?: boolean;
}

export const defaultBatteryGroups: MetricGroup[] = [
  {
    key: "battery",
    label: "Battery",
    description: "Acumulator",
    color: "violet",
    metrics: [
      { field: "battery_soc", label: "Battery SOC", unit: "%", decimals: 0 },
      { field: "battery_power", label: "Battery Power", unit: "kW", decimals: 3 },
      { field: "battery_bus_voltage", label: "Bus Voltage", unit: "V", decimals: 1 },
      { field: "battery_bus_current", label: "Bus Current", unit: "A", decimals: 2 },
      { field: "battery_temp", label: "Battery Temp", unit: "°C", decimals: 1 },
      { field: "battery_max_temp", label: "Max Temp", unit: "°C", decimals: 1 },
      { field: "battery_day_charge_capacity", label: "Day Charge", unit: "kWh", decimals: 2 },
      { field: "battery_day_discharge_capacity", label: "Day Discharge", unit: "kWh", decimals: 2 },
      { field: "battery_total_charge", label: "Total Charge", unit: "kWh", decimals: 2 },
      { field: "battery_total_discharge", label: "Total Discharge", unit: "kWh", decimals: 2 },
    ],
  },
  {
    key: "battery_status",
    label: "Battery Status",
    description: "Stare baterie",
    color: "gray",
    metrics: [
      { field: "battery_running_status", label: "Running Status", unit: "", decimals: 0 },
      { field: "battery_working_mode", label: "Working Mode", unit: "", decimals: 0 },
    ],
  },
];

export function BatteryPanel({ deviceSerial, range, groups = defaultBatteryGroups, showHeroKpis = true }: BatteryPanelProps) {
  const allFields = groups.flatMap((g) => g.metrics.map((m) => m.field));
  const { values, errors } = useDeviceMetrics(deviceSerial, allFields, range);

  const soc = values["battery_soc"];
  const battPowerKw = values["battery_power"];
  const dayCharge = values["battery_day_charge_capacity"];
  const dayDischarge = values["battery_day_discharge_capacity"];

  return (
    <div>
      {showHeroKpis && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          <SocKpi soc={soc} />
          <BatteryFlowKpi powerKw={battPowerKw} />
          <DayCapacityKpi label="Day Charge" value={dayCharge} arrow="up" />
          <DayCapacityKpi label="Day Discharge" value={dayDischarge} arrow="down" />
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
              storageKey={`solar-order-battery-${g.key}`}
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

function SocKpi({ soc }: { soc: number | null }) {
  const value = soc ?? 0;
  const tone = value > 70 ? "emerald" : value > 30 ? "amber" : "rose";
  const colors = {
    emerald: { bg: "bg-emerald-100", text: "text-emerald-900", label: "text-emerald-800", fill: "bg-emerald-500" },
    amber: { bg: "bg-amber-100", text: "text-amber-900", label: "text-amber-800", fill: "bg-amber-500" },
    rose: { bg: "bg-rose-100", text: "text-rose-900", label: "text-rose-800", fill: "bg-rose-500" },
  } as const;
  const c = colors[tone];
  return (
    <div className={`${c.bg} border border-gray-200 rounded-xl overflow-hidden flex`}>
      <div className="flex-1 px-5 py-4">
        <p className={`text-[11px] font-semibold ${c.label} uppercase tracking-wider`}>Battery SOC</p>
        <p className="mt-2 flex items-baseline gap-1.5">
          <span className={`text-3xl font-bold tabular-nums ${soc === null ? "text-gray-300" : c.text}`}>
            {soc !== null ? soc.toFixed(0) : "—"}
          </span>
          <span className={`text-sm font-medium ${c.label}`}>%</span>
        </p>
        {soc !== null && (
          <div className="mt-2 h-1 bg-gray-200 rounded-full overflow-hidden">
            <div className={`h-full ${c.fill} transition-all`} style={{ width: `${Math.max(0, Math.min(100, value))}%` }} />
          </div>
        )}
      </div>
    </div>
  );
}

function BatteryFlowKpi({ powerKw }: { powerKw: number | null }) {
  const charging = powerKw !== null && powerKw > 0.05;
  const discharging = powerKw !== null && powerKw < -0.05;

  let bg = "bg-slate-100";
  let label = "Battery Idle";
  let sub = "No flow";
  let arrow = "·";
  let labelColor = "text-slate-800";
  let textColor = "text-slate-900";
  let arrowColor = "text-slate-500";
  let subColor = "text-slate-700";

  if (discharging) {
    bg = "bg-emerald-100";
    label = "Battery Output";
    sub = "Delivering power";
    arrow = "↓";
    labelColor = "text-emerald-800";
    textColor = "text-emerald-900";
    arrowColor = "text-emerald-700";
    subColor = "text-emerald-700";
  } else if (charging) {
    bg = "bg-indigo-100";
    label = "Battery Charging";
    sub = "Storing energy";
    arrow = "↑";
    labelColor = "text-indigo-800";
    textColor = "text-indigo-900";
    arrowColor = "text-indigo-700";
    subColor = "text-indigo-700";
  }

  return (
    <div className={`${bg} border border-gray-200 rounded-xl overflow-hidden flex`}>
      <div className="flex-1 px-5 py-4">
        <div className="flex items-center gap-2">
          <p className={`text-[11px] font-semibold ${labelColor} uppercase tracking-wider`}>{label}</p>
          <span className={`text-sm ${arrowColor}`}>{arrow}</span>
        </div>
        <p className="mt-2 flex items-baseline gap-1.5">
          <span className={`text-3xl font-bold tabular-nums ${powerKw === null ? "text-gray-300" : textColor}`}>
            {powerKw !== null ? Math.abs(powerKw * 1000).toFixed(0) : "—"}
          </span>
          <span className={`text-sm font-medium ${labelColor}`}>W</span>
        </p>
        <p className={`text-[11px] mt-1 ${subColor}`}>{sub}</p>
      </div>
    </div>
  );
}

function DayCapacityKpi({ label, value, arrow }: { label: string; value: number | null; arrow: "up" | "down" }) {
  const isEmpty = value === null;
  const bg = arrow === "up" ? "bg-indigo-100" : "bg-slate-100";
  const labelColor = arrow === "up" ? "text-indigo-800" : "text-slate-800";
  const textColor = arrow === "up" ? "text-indigo-900" : "text-slate-900";
  const arrowColor = arrow === "up" ? "text-indigo-700" : "text-slate-600";
  return (
    <div className={`${bg} border border-gray-200 rounded-xl overflow-hidden flex`}>
      <div className="flex-1 px-5 py-4">
        <div className="flex items-center gap-2">
          <p className={`text-[11px] font-semibold ${labelColor} uppercase tracking-wider`}>{label}</p>
          <span className={`text-sm ${arrowColor}`}>{arrow === "up" ? "↑" : "↓"}</span>
        </div>
        <p className="mt-2 flex items-baseline gap-1.5">
          <span className={`text-3xl font-bold tabular-nums ${isEmpty ? "text-gray-300" : textColor}`}>
            {value !== null ? value.toFixed(2) : "—"}
          </span>
          <span className={`text-sm font-medium ${labelColor}`}>kWh</span>
        </p>
      </div>
    </div>
  );
}
