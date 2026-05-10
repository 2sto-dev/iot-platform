/**
 * Gauge semicircular pentru consumul casei.
 * Culoare după sursa dominantă: emerald (PV/export), indigo (baterie), rose (rețea), amber (mix).
 */
export function HouseLoadGauge({ loadKw, gridPowerW, pvPowerKw, batteryPowerKw, maxKw = 10 }: {
  loadKw: number | null;
  gridPowerW: number | null;
  pvPowerKw: number | null;
  batteryPowerKw: number | null;
  maxKw?: number;
}) {
  const loadW = loadKw !== null ? loadKw * 1000 : null;
  const pvW = pvPowerKw !== null ? pvPowerKw * 1000 : null;
  const battW = batteryPowerKw !== null ? batteryPowerKw * 1000 : null;
  const max = maxKw * 1000;
  const value = loadW ?? 0;
  const pct = Math.max(0, Math.min(1, value / max));

  let color = "#cbd5e1"; // slate-300
  let label = "—";
  let source = "No data";
  let badge: { text: string; bg: string; fg: string } = { text: "—", bg: "bg-gray-100", fg: "text-gray-500" };

  if (loadW !== null && gridPowerW !== null && pvW !== null && battW !== null) {
    const fromPv = Math.max(0, pvW);
    const fromBattery = battW < 0 ? Math.abs(battW) : 0;
    const fromGrid = gridPowerW > 0 ? gridPowerW : 0;
    const totalSrc = fromPv + fromBattery + fromGrid || 1;

    const pctPv = (fromPv / totalSrc) * 100;
    const pctBatt = (fromBattery / totalSrc) * 100;
    const pctGrid = (fromGrid / totalSrc) * 100;

    if (gridPowerW < -50) {
      color = "#10b981";
      source = `Exporting ${Math.abs(gridPowerW).toFixed(0)} W`;
      badge = { text: "Exporting", bg: "bg-emerald-50", fg: "text-emerald-700" };
    } else if (pctGrid > 50) {
      color = "#e11d48";
      source = `${pctGrid.toFixed(0)}% from grid`;
      badge = { text: "Grid", bg: "bg-rose-50", fg: "text-rose-700" };
    } else if (pctPv > 50) {
      color = "#10b981";
      source = `${pctPv.toFixed(0)}% from solar`;
      badge = { text: "Solar", bg: "bg-emerald-50", fg: "text-emerald-700" };
    } else if (pctBatt > 50) {
      color = "#6366f1";
      source = `${pctBatt.toFixed(0)}% from battery`;
      badge = { text: "Battery", bg: "bg-indigo-50", fg: "text-indigo-700" };
    } else if (pctGrid > 5) {
      color = "#f59e0b";
      source = `${pctPv.toFixed(0)}% PV · ${pctBatt.toFixed(0)}% batt · ${pctGrid.toFixed(0)}% grid`;
      badge = { text: "Mixed", bg: "bg-amber-50", fg: "text-amber-700" };
    } else {
      color = "#10b981";
      source = `${pctPv.toFixed(0)}% solar · ${pctBatt.toFixed(0)}% battery`;
      badge = { text: "Self-sufficient", bg: "bg-emerald-50", fg: "text-emerald-700" };
    }
    label = `${(loadW / 1000).toFixed(2)}`;
  }

  const cx = 130, cy = 120, r = 100, sw = 14;
  const endAngle = Math.PI - Math.PI * pct;
  const x2 = cx + r * Math.cos(endAngle);
  const y2 = cy + r * Math.sin(endAngle);
  const largeArc = pct > 0.5 ? 1 : 0;
  const arcPath = pct > 0
    ? `M ${cx - r} ${cy} A ${r} ${r} 0 ${largeArc} 1 ${x2} ${y2}`
    : "";

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-6 flex flex-col items-center">
      <div className="w-full flex items-center justify-between mb-4">
        <h3 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">House Consumption</h3>
        <span className={`text-[11px] font-medium px-2 py-0.5 rounded-full ${badge.bg} ${badge.fg}`}>
          {badge.text}
        </span>
      </div>
      <svg viewBox="0 0 260 150" className="w-full max-w-sm">
        <defs>
          <linearGradient id="gauge-grad" x1="0%" y1="0%" x2="100%" y2="0%">
            <stop offset="0%" stopColor={color} stopOpacity="0.7" />
            <stop offset="100%" stopColor={color} stopOpacity="1" />
          </linearGradient>
        </defs>
        <path
          d={`M ${cx - r} ${cy} A ${r} ${r} 0 0 1 ${cx + r} ${cy}`}
          fill="none"
          stroke="#f1f5f9"
          strokeWidth={sw}
          strokeLinecap="round"
        />
        {arcPath && (
          <path
            d={arcPath}
            fill="none"
            stroke="url(#gauge-grad)"
            strokeWidth={sw}
            strokeLinecap="round"
            style={{ transition: "stroke 0.4s, d 0.6s" }}
          />
        )}
        <text x={cx} y={cy - 5} textAnchor="middle" fontSize="36" fontWeight="700" fill="#0f172a" fontFamily="ui-sans-serif, system-ui">
          {label}
        </text>
        <text x={cx} y={cy + 18} textAnchor="middle" fontSize="13" fill="#64748b" fontWeight="500">
          kW · max {maxKw} kW
        </text>
      </svg>
      <p className="text-sm font-medium mt-2" style={{ color }}>{source}</p>
    </div>
  );
}
