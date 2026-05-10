/**
 * Energy flow diagram (FusionSolar-style).
 * Diamond layout: PV (top), Battery (left), Grid (right), House (bottom),
 * with the inverter as central hub. Lines animate cu stroke-dashoffset
 * (SMIL <animate>) — direcția și viteza reflectă fluxul real:
 *  - PV → Inverter când pv > 50 W
 *  - Inverter → Battery când charging, Battery → Inverter când discharging
 *  - Inverter → Grid când exporting (grid_power < 0), Grid → Inverter când importing
 *  - Inverter → House mereu (când load > 0)
 *
 * Speed = clamp(2 / kW, 0.4s ... 3.5s) — mai multă putere = puncte mai rapide.
 */
export function EnergyFlowDiagram({
  pvPowerKw,
  gridPowerW,
  batteryPowerKw,
  batterySoc,
  houseLoadKw,
}: {
  pvPowerKw: number | null;
  gridPowerW: number | null;
  batteryPowerKw: number | null;
  batterySoc: number | null;
  houseLoadKw: number | null;
}) {
  // Status flags. Convenția Huawei SUN2000 pe `grid_power`:
  //   > 0  → EXPORT (sistemul produce mai mult decât consumă, surplus la rețea)
  //   < 0  → IMPORT (rețeaua acoperă deficitul intern)
  const pvActive = pvPowerKw !== null && pvPowerKw > 0.05;
  const exporting = gridPowerW !== null && gridPowerW > 50;
  const importing = gridPowerW !== null && gridPowerW < -50;
  const charging = batteryPowerKw !== null && batteryPowerKw > 0.05;
  const discharging = batteryPowerKw !== null && batteryPowerKw < -0.05;
  const consuming = houseLoadKw !== null && houseLoadKw > 0.05;

  // Speed-from-power → dur in seconds (smaller = faster)
  const speed = (kw: number) => {
    const v = Math.max(0.1, Math.abs(kw));
    return Math.min(3.5, Math.max(0.4, 2 / v)).toFixed(2);
  };

  const pvSpeed = pvActive ? speed(pvPowerKw!) : "2";
  const battSpeed = (charging || discharging) ? speed(batteryPowerKw!) : "2";
  const gridSpeed = (importing || exporting) ? speed(Math.abs(gridPowerW!) / 1000) : "2";
  const houseSpeed = consuming ? speed(houseLoadKw!) : "2";

  // Layout: 600x500, central hub (300, 250)
  const cx = 300, cy = 250;
  // Node positions
  const pv = { x: 300, y: 60 };
  const battery = { x: 90, y: 250 };
  const grid = { x: 510, y: 250 };
  const house = { x: 300, y: 440 };

  return (
    <div className="bg-white border border-gray-200 rounded-xl p-6">
      <div className="flex items-center justify-between mb-2">
        <h3 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">Energy Flow</h3>
        <span className="text-[10px] text-gray-400">live</span>
      </div>

      <svg viewBox="0 0 600 500" className="w-full" style={{ maxHeight: 460 }}>
        {/* ── Arrow markers per culoare (refuse pe linii active) ────────── */}
        <defs>
          {(["#f59e0b", "#10b981", "#6366f1", "#e11d48"] as const).map((c) => (
            <marker
              key={c}
              id={`arrow-${c.slice(1)}`}
              viewBox="0 0 10 10"
              refX="8"
              refY="5"
              markerWidth="7"
              markerHeight="7"
              orient="auto-start-reverse"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" fill={c} />
            </marker>
          ))}
        </defs>

        {/* ── Lines (drawn first so nodes overlay) ───────────────────────── */}
        {/* PV → Inverter (vertical top) */}
        <FlowLine
          x1={pv.x} y1={pv.y + 36} x2={cx} y2={cy - 32}
          active={pvActive}
          color="#f59e0b"
          dur={pvSpeed}
          reverse={false}
        />
        {/* Battery ↔ Inverter (horizontal left)
            Linia e orientată x1=baterie, x2=inverter.
            - discharging (battery → inverter)  → reverse=false (forward, x1→x2)
            - charging    (inverter → battery)  → reverse=true  (x2→x1) */}
        <FlowLine
          x1={battery.x + 36} y1={battery.y} x2={cx - 32} y2={cy}
          active={charging || discharging}
          color={discharging ? "#10b981" : "#6366f1"}
          dur={battSpeed}
          reverse={charging}
        />
        {/* Inverter ↔ Grid (horizontal right) */}
        <FlowLine
          x1={cx + 32} y1={cy} x2={grid.x - 36} y2={grid.y}
          active={importing || exporting}
          color={exporting ? "#10b981" : "#e11d48"}
          dur={gridSpeed}
          reverse={importing}
        />
        {/* Inverter → House (vertical bottom) */}
        <FlowLine
          x1={cx} y1={cy + 32} x2={house.x} y2={house.y - 36}
          active={consuming}
          color="#e11d48"
          dur={houseSpeed}
          reverse={false}
        />

        {/* ── Central hub: Inverter ─────────────────────────────────────── */}
        <g>
          <circle cx={cx} cy={cy} r="32" fill="#0f172a" stroke="#1e293b" strokeWidth="2" />
          <circle cx={cx} cy={cy} r="38" fill="none" stroke="#1e293b" strokeOpacity="0.15" strokeWidth="2" />
          <text x={cx} y={cy + 5} textAnchor="middle" fontSize="13" fontWeight="700" fill="white" fontFamily="ui-sans-serif">INV</text>
        </g>

        {/* ── PV node ───────────────────────────────────────────────────── */}
        <Node
          x={pv.x} y={pv.y}
          icon={<SunIcon />}
          label="Solar"
          value={pvPowerKw !== null ? `${(pvPowerKw * 1000).toFixed(0)} W` : "—"}
          accent={pvActive ? "amber" : "slate"}
        />

        {/* ── Battery node ──────────────────────────────────────────────── */}
        <Node
          x={battery.x} y={battery.y}
          icon={<BatteryIcon soc={batterySoc} />}
          label="Battery"
          value={batterySoc !== null ? `${batterySoc.toFixed(0)}%` : "—"}
          subValue={
            charging ? `+${(batteryPowerKw! * 1000).toFixed(0)} W`
            : discharging ? `−${Math.abs(batteryPowerKw! * 1000).toFixed(0)} W`
            : "Idle"
          }
          accent={charging ? "indigo" : discharging ? "emerald" : "slate"}
        />

        {/* ── Grid node ─────────────────────────────────────────────────── */}
        <Node
          x={grid.x} y={grid.y}
          icon={<GridIcon />}
          label="Grid"
          value={gridPowerW !== null ? `${Math.abs(gridPowerW).toFixed(0)} W` : "—"}
          subValue={exporting ? "Exporting" : importing ? "Importing" : "Balanced"}
          accent={exporting ? "emerald" : importing ? "rose" : "slate"}
        />

        {/* ── House node ────────────────────────────────────────────────── */}
        <Node
          x={house.x} y={house.y}
          icon={<HouseIcon />}
          label="House"
          value={houseLoadKw !== null ? `${(houseLoadKw * 1000).toFixed(0)} W` : "—"}
          accent={consuming ? "rose" : "slate"}
        />
      </svg>

      {/* Legend below */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 mt-4 pt-4 border-t border-gray-100">
        <LegendItem color="#f59e0b" label="Solar" active={pvActive} />
        <LegendItem color="#6366f1" label="Charging" active={charging} />
        <LegendItem color="#10b981" label="Self-use / Export" active={discharging || exporting} />
        <LegendItem color="#e11d48" label="Grid Import / Load" active={importing || consuming} />
      </div>
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// FlowLine — segment SVG cu animație stroke-dashoffset (SMIL).
// reverse=true → puncte curg invers (de la x2,y2 spre x1,y1).
// ──────────────────────────────────────────────────────────────────────────
function FlowLine({
  x1, y1, x2, y2, active, color, dur, reverse,
}: {
  x1: number; y1: number; x2: number; y2: number;
  active: boolean; color: string; dur: string; reverse: boolean;
}) {
  // Săgeata se pune la "destination". Dacă reverse, schimbăm sensul: x1↔x2.
  const ax1 = reverse ? x2 : x1;
  const ay1 = reverse ? y2 : y1;
  const ax2 = reverse ? x1 : x2;
  const ay2 = reverse ? y1 : y2;
  // Tragem linia un pic mai scurt ca să lase loc săgeții să nu se suprapună cu nodul
  const dx = ax2 - ax1, dy = ay2 - ay1;
  const len = Math.sqrt(dx * dx + dy * dy);
  const shrink = 4;
  const tipX = ax2 - (dx / len) * shrink;
  const tipY = ay2 - (dy / len) * shrink;

  return (
    <g>
      {/* base line (always visible, dim) */}
      <line
        x1={x1} y1={y1} x2={x2} y2={y2}
        stroke="#e2e8f0" strokeWidth="3" strokeLinecap="round"
      />
      {/* animated dashes + săgeată la destinație (când activ) */}
      {active && (
        <>
          <line
            x1={ax1} y1={ay1} x2={tipX} y2={tipY}
            stroke={color} strokeWidth="3" strokeLinecap="round"
            strokeDasharray="6 10"
            markerEnd={`url(#arrow-${color.slice(1)})`}
          >
            <animate
              attributeName="stroke-dashoffset"
              from="0" to="-32"
              dur={`${dur}s`}
              repeatCount="indefinite"
            />
          </line>
        </>
      )}
    </g>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// Node — capsulă rotunjită cu icon, label, value, subValue
// ──────────────────────────────────────────────────────────────────────────
type NodeAccent = "amber" | "indigo" | "emerald" | "rose" | "slate";

const ACCENTS: Record<NodeAccent, { bg: string; stroke: string; text: string; sub: string }> = {
  amber:   { bg: "#fef3c7", stroke: "#f59e0b", text: "#92400e", sub: "#b45309" },
  indigo:  { bg: "#e0e7ff", stroke: "#6366f1", text: "#3730a3", sub: "#4338ca" },
  emerald: { bg: "#d1fae5", stroke: "#10b981", text: "#065f46", sub: "#047857" },
  rose:    { bg: "#ffe4e6", stroke: "#e11d48", text: "#9f1239", sub: "#be123c" },
  slate:   { bg: "#f1f5f9", stroke: "#94a3b8", text: "#334155", sub: "#64748b" },
};

function Node({
  x, y, icon, label, value, subValue, accent,
}: {
  x: number; y: number;
  icon: React.ReactNode;
  label: string;
  value: string;
  subValue?: string;
  accent: NodeAccent;
}) {
  const a = ACCENTS[accent];
  const w = 140, h = 72;
  const left = x - w / 2;
  const top = y - h / 2;
  return (
    <g>
      <rect
        x={left} y={top} width={w} height={h} rx="14"
        fill={a.bg} stroke={a.stroke} strokeWidth="2"
      />
      <g transform={`translate(${left + 16}, ${top + 18})`}>
        {icon}
      </g>
      <text x={left + 56} y={top + 24} fontSize="11" fontWeight="700" fill={a.sub}
            fontFamily="ui-sans-serif" letterSpacing="1.2">
        {label.toUpperCase()}
      </text>
      <text x={left + 56} y={top + 44} fontSize="18" fontWeight="700" fill={a.text}
            fontFamily="ui-sans-serif">
        {value}
      </text>
      {subValue && (
        <text x={left + 56} y={top + 60} fontSize="10" fill={a.sub} fontFamily="ui-sans-serif">
          {subValue}
        </text>
      )}
    </g>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// Icons (inline SVG)
// ──────────────────────────────────────────────────────────────────────────
function SunIcon() {
  return (
    <svg width="32" height="32" viewBox="0 0 24 24" fill="none">
      <circle cx="12" cy="12" r="4" fill="#f59e0b" />
      <g stroke="#f59e0b" strokeWidth="2" strokeLinecap="round">
        <line x1="12" y1="2" x2="12" y2="5" />
        <line x1="12" y1="19" x2="12" y2="22" />
        <line x1="2" y1="12" x2="5" y2="12" />
        <line x1="19" y1="12" x2="22" y2="12" />
        <line x1="4.93" y1="4.93" x2="7.05" y2="7.05" />
        <line x1="16.95" y1="16.95" x2="19.07" y2="19.07" />
        <line x1="4.93" y1="19.07" x2="7.05" y2="16.95" />
        <line x1="16.95" y1="7.05" x2="19.07" y2="4.93" />
      </g>
    </svg>
  );
}

function BatteryIcon({ soc }: { soc: number | null }) {
  const pct = soc !== null ? Math.max(0, Math.min(100, soc)) : 0;
  const fillW = (pct / 100) * 22;
  const color = soc === null ? "#94a3b8" : pct > 70 ? "#10b981" : pct > 30 ? "#f59e0b" : "#e11d48";
  return (
    <svg width="32" height="32" viewBox="0 0 32 32" fill="none">
      <rect x="3" y="9" width="24" height="14" rx="2" stroke="#475569" strokeWidth="2" />
      <rect x="27" y="13" width="3" height="6" rx="1" fill="#475569" />
      {soc !== null && <rect x="5" y="11" width={fillW} height="10" rx="1" fill={color} />}
    </svg>
  );
}

function GridIcon() {
  return (
    <svg width="32" height="32" viewBox="0 0 32 32" fill="none">
      <path d="M16 4 L8 14 L12 14 L8 24 L24 24 L20 14 L24 14 Z"
            stroke="#475569" strokeWidth="2" strokeLinejoin="round" fill="#f1f5f9" />
      <line x1="16" y1="14" x2="16" y2="20" stroke="#475569" strokeWidth="1.5" />
    </svg>
  );
}

function HouseIcon() {
  return (
    <svg width="32" height="32" viewBox="0 0 32 32" fill="none">
      <path d="M4 14 L16 4 L28 14 L28 26 L4 26 Z"
            stroke="#9f1239" strokeWidth="2" strokeLinejoin="round" fill="#ffe4e6" />
      <rect x="13" y="18" width="6" height="8" fill="#9f1239" />
    </svg>
  );
}

function LegendItem({ color, label, active }: { color: string; label: string; active: boolean }) {
  return (
    <div className="flex items-center gap-2 text-[11px]">
      <span
        className={`inline-block w-6 h-1 rounded-full ${active ? "" : "opacity-30"}`}
        style={{ backgroundColor: color }}
      />
      <span className={active ? "text-gray-700 font-medium" : "text-gray-400"}>{label}</span>
    </div>
  );
}
