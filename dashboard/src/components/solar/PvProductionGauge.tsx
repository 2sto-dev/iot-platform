/**
 * Gauge semicircular pentru producția fotovoltaică (DC).
 * Status colors:
 *  - amber/yellow gradient cand panourile generează > peakKw * 0.6
 *  - emerald cand 0.2-0.6 (producție bună dar nu peak)
 *  - slate cand < 0.05 (noapte / înnorat / inactivitate)
 *  - sky-amber pt 0.05-0.2 (lumină slabă)
 */
export function PvProductionGauge({
  pvPowerKw,
  dailyYieldKwh,
  peakTodayKw,
  maxKw = 10,
}: {
  pvPowerKw: number | null;
  dailyYieldKwh: number | null;
  peakTodayKw: number | null;
  maxKw?: number;
}) {
  const value = pvPowerKw ?? 0;
  const pct = Math.max(0, Math.min(1, value / maxKw));

  let color = "#cbd5e1"; // slate-300 (default — no production)
  let badge: { text: string; bg: string; fg: string } = {
    text: "Inactive",
    bg: "bg-slate-100",
    fg: "text-slate-600",
  };
  let status = "—";

  if (pvPowerKw !== null) {
    if (value < 0.05) {
      color = "#cbd5e1";
      status = "No production";
      badge = { text: "Idle", bg: "bg-slate-100", fg: "text-slate-600" };
    } else if (value < maxKw * 0.2) {
      color = "#fbbf24"; // amber-400
      status = "Low light";
      badge = { text: "Low", bg: "bg-amber-50", fg: "text-amber-700" };
    } else if (value < maxKw * 0.6) {
      color = "#10b981"; // emerald-500
      status = "Good production";
      badge = { text: "Producing", bg: "bg-emerald-50", fg: "text-emerald-700" };
    } else {
      color = "#f59e0b"; // amber-500 — peak sun
      status = "Peak production";
      badge = { text: "Peak", bg: "bg-amber-100", fg: "text-amber-800" };
    }
  }

  const cx = 130, cy = 120, r = 100, sw = 14;
  const endAngle = Math.PI - Math.PI * pct;
  const x2 = cx + r * Math.cos(endAngle);
  const y2 = cy + r * Math.sin(endAngle);
  const largeArc = pct > 0.5 ? 1 : 0;
  const arcPath = pct > 0
    ? `M ${cx - r} ${cy} A ${r} ${r} 0 ${largeArc} 1 ${x2} ${y2}`
    : "";

  const labelMain = pvPowerKw !== null ? pvPowerKw.toFixed(2) : "—";

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-6 flex flex-col items-center">
      <div className="w-full flex items-center justify-between mb-4">
        <h3 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">PV Production</h3>
        <span className={`text-[11px] font-medium px-2 py-0.5 rounded-full ${badge.bg} ${badge.fg}`}>
          {badge.text}
        </span>
      </div>
      <svg viewBox="0 0 260 150" className="w-full max-w-sm">
        <defs>
          <linearGradient id="pv-gauge-grad" x1="0%" y1="0%" x2="100%" y2="0%">
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
            stroke="url(#pv-gauge-grad)"
            strokeWidth={sw}
            strokeLinecap="round"
            style={{ transition: "stroke 0.4s, d 0.6s" }}
          />
        )}
        <text x={cx} y={cy - 5} textAnchor="middle" fontSize="36" fontWeight="700" fill="#0f172a" fontFamily="ui-sans-serif, system-ui">
          {labelMain}
        </text>
        <text x={cx} y={cy + 18} textAnchor="middle" fontSize="13" fill="#64748b" fontWeight="500">
          kW · max {maxKw} kW
        </text>
      </svg>
      <p className="text-sm font-medium mt-2" style={{ color }}>{status}</p>

      <div className="w-full grid grid-cols-2 gap-3 mt-5 pt-4 border-t border-gray-100">
        <div>
          <p className="text-[10px] text-gray-500 uppercase tracking-wider font-semibold">Today</p>
          <p className="text-sm font-bold text-gray-900 tabular-nums mt-0.5">
            {dailyYieldKwh !== null ? `${dailyYieldKwh.toFixed(2)} kWh` : "—"}
          </p>
        </div>
        <div>
          <p className="text-[10px] text-gray-500 uppercase tracking-wider font-semibold">Peak</p>
          <p className="text-sm font-bold text-gray-900 tabular-nums mt-0.5">
            {peakTodayKw !== null ? `${peakTodayKw.toFixed(2)} kW` : "—"}
          </p>
        </div>
      </div>
    </div>
  );
}
