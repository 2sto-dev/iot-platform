/**
 * Energy flow diagram — FusionSolar-style.
 *
 * Layout: 4 noduri circulare aranjate ca o cruce (PV sus, Battery stânga,
 * Grid dreapta, Consumption jos). Liniile converg într-un punct central
 * invizibil — nu mai există hub "INV" desenat.
 *
 * Fiecare nod e un cerc alb cu inel colorat (gri când inactiv, accent
 * când activ) + icon + value (kW) + label.
 *
 * Liniile sunt animate cu SMIL stroke-dashoffset, direcția depinde de
 * sensul fluxului real:
 *  - PV → centru: când pv > 50W (amber)
 *  - Battery ↔ centru: charging = în baterie (indigo); discharging = spre centru (emerald)
 *  - Grid ↔ centru: import = spre centru (rose); export = spre grid (emerald)
 *  - Centru → House: mereu, când load > 50W (sky blue)
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
  // ── Status flags (Huawei sign convention: grid > 0 = export) ────────────
  const pvActive = pvPowerKw !== null && pvPowerKw > 0.05;
  const exporting = gridPowerW !== null && gridPowerW > 50;
  const importing = gridPowerW !== null && gridPowerW < -50;
  const charging = batteryPowerKw !== null && batteryPowerKw > 0.05;
  const discharging = batteryPowerKw !== null && batteryPowerKw < -0.05;
  const consuming = houseLoadKw !== null && houseLoadKw > 0.05;

  // Speed proporțional cu puterea (kW absolut)
  const speed = (kw: number) => {
    const v = Math.max(0.1, Math.abs(kw));
    return Math.min(3.5, Math.max(0.5, 2 / v)).toFixed(2);
  };

  const pvSpeed = pvActive ? speed(pvPowerKw!) : "2";
  const battSpeed = (charging || discharging) ? speed(batteryPowerKw!) : "2";
  const gridSpeed = (importing || exporting) ? speed(Math.abs(gridPowerW!) / 1000) : "2";
  const houseSpeed = consuming ? speed(houseLoadKw!) : "2";

  // ── Layout: viewBox 640 × 660, centru invizibil în (320, 330) ──────────
  // Cercuri mai mari (r=72) ca textul să fie spațios + lizibil.
  const cx = 320, cy = 330;
  const nodeR = 72;
  const ringW = 5;

  const pv = { x: 320, y: 100 };
  const battery = { x: 100, y: 330 };
  const grid = { x: 540, y: 330 };
  const house = { x: 320, y: 560 };

  // Neon palette — fluorescent versions of the original colors
  const COLOR_PV       = "#fdbf3a";  // golden neon
  const COLOR_CHARGE   = "#b794ff";  // violet neon
  const COLOR_DISCHARGE = "#4dffaa"; // lime neon
  const COLOR_IMPORT   = "#ff3c8e";  // hot pink
  const COLOR_EXPORT   = "#4dffaa";  // lime
  const COLOR_HOUSE    = "#5bf2ff";  // cyan
  const COLOR_IDLE     = "#3a4060";  // very dim

  return (
    <div className="rounded-xl p-6 neon-card-cyan" style={{ background: "linear-gradient(180deg, rgba(13, 17, 38, 0.7), rgba(7, 9, 22, 0.85))" }}>
      <div className="flex items-center justify-between mb-2">
        <h3 className="text-xs font-semibold uppercase tracking-wider" style={{ color: "#5bf2ff" }}>Energy Flow</h3>
        <span className="text-[10px] flex items-center gap-1.5" style={{ color: "#4dffaa" }}>
          <span className="neon-live-dot" style={{ width: 6, height: 6 }} />
          <span style={{ textShadow: "0 0 8px rgba(0, 255, 136, 0.5)" }}>LIVE</span>
        </span>
      </div>

      <svg viewBox="0 0 640 680" className="w-full" style={{ maxHeight: 620 }}>
        <defs>
          {/* Glow filter pentru noduri active (neon outer + inner glow) */}
          <filter id="neon-glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="coloredBlur" />
            <feMerge>
              <feMergeNode in="coloredBlur" />
              <feMergeNode in="coloredBlur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
          {/* Glow strong pentru linii energie */}
          <filter id="line-glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="2.5" result="coloredBlur" />
            <feMerge>
              <feMergeNode in="coloredBlur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>

          {/* Halo radial pentru noduri active */}
          <radialGradient id="halo-amber" cx="50%" cy="50%" r="50%">
            <stop offset="40%" stopColor={COLOR_PV} stopOpacity="0" />
            <stop offset="100%" stopColor={COLOR_PV} stopOpacity="0.35" />
          </radialGradient>
          <radialGradient id="halo-emerald" cx="50%" cy="50%" r="50%">
            <stop offset="40%" stopColor={COLOR_DISCHARGE} stopOpacity="0" />
            <stop offset="100%" stopColor={COLOR_DISCHARGE} stopOpacity="0.35" />
          </radialGradient>
          <radialGradient id="halo-indigo" cx="50%" cy="50%" r="50%">
            <stop offset="40%" stopColor={COLOR_CHARGE} stopOpacity="0" />
            <stop offset="100%" stopColor={COLOR_CHARGE} stopOpacity="0.35" />
          </radialGradient>
          <radialGradient id="halo-sky" cx="50%" cy="50%" r="50%">
            <stop offset="40%" stopColor={COLOR_HOUSE} stopOpacity="0" />
            <stop offset="100%" stopColor={COLOR_HOUSE} stopOpacity="0.35" />
          </radialGradient>
          <radialGradient id="halo-rose" cx="50%" cy="50%" r="50%">
            <stop offset="40%" stopColor={COLOR_IMPORT} stopOpacity="0" />
            <stop offset="100%" stopColor={COLOR_IMPORT} stopOpacity="0.35" />
          </radialGradient>

          {/* Săgeți marker per culoare neon */}
          {([COLOR_PV, COLOR_DISCHARGE, COLOR_CHARGE, COLOR_IMPORT, COLOR_HOUSE, COLOR_IDLE] as const).map((c) => (
            <marker
              key={c}
              id={`flow-arrow-${c.slice(1)}`}
              viewBox="0 0 10 10"
              refX="8"
              refY="5"
              markerWidth="6"
              markerHeight="6"
              orient="auto-start-reverse"
            >
              <path d="M 0 0 L 10 5 L 0 10 z" fill={c} />
            </marker>
          ))}
        </defs>

        {/* PV → centru */}
        <CurvedFlow
          from={{ x: pv.x, y: pv.y + nodeR }}
          to={{ x: cx, y: cy - 6 }}
          control={{ x: pv.x, y: cy - 80 }}
          active={pvActive}
          color={COLOR_PV}
          dur={pvSpeed}
          reverse={false}
        />
        {/* Battery ↔ centru */}
        <CurvedFlow
          from={{ x: battery.x + nodeR, y: battery.y }}
          to={{ x: cx - 8, y: cy }}
          control={{ x: cx - 80, y: battery.y }}
          active={charging || discharging}
          color={discharging ? COLOR_DISCHARGE : COLOR_CHARGE}
          dur={battSpeed}
          reverse={charging}
        />
        {/* centru ↔ Grid */}
        <CurvedFlow
          from={{ x: cx + 8, y: cy }}
          to={{ x: grid.x - nodeR, y: grid.y }}
          control={{ x: cx + 80, y: grid.y }}
          active={importing || exporting}
          color={exporting ? COLOR_EXPORT : COLOR_IMPORT}
          dur={gridSpeed}
          reverse={importing}
        />
        {/* centru → House */}
        <CurvedFlow
          from={{ x: cx, y: cy + 6 }}
          to={{ x: house.x, y: house.y - nodeR }}
          control={{ x: house.x, y: cy + 80 }}
          active={consuming}
          color={COLOR_HOUSE}
          dur={houseSpeed}
          reverse={false}
        />

        {/* Punct central — pulsator când există flux */}
        <circle cx={cx} cy={cy} r="4" fill={COLOR_HOUSE} filter="url(#neon-glow)">
          <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite" />
          <animate attributeName="opacity" values="0.6;1;0.6" dur="2s" repeatCount="indefinite" />
        </circle>

        <CircleNode
          cx={pv.x} cy={pv.y} r={nodeR} ringW={ringW}
          ringColor={pvActive ? COLOR_PV : COLOR_IDLE}
          haloId={pvActive ? "halo-amber" : null}
          icon={<PvIcon />}
          value={pvPowerKw !== null ? `${pvPowerKw.toFixed(3)}` : "—"}
          unit="kW"
          label="PV"
        />

        <CircleNode
          cx={battery.x} cy={battery.y} r={nodeR} ringW={ringW}
          ringColor={charging ? COLOR_CHARGE : discharging ? COLOR_DISCHARGE : COLOR_IDLE}
          haloId={charging ? "halo-indigo" : discharging ? "halo-emerald" : null}
          icon={<BatteryIcon soc={batterySoc} />}
          value={batteryPowerKw !== null ? `${Math.abs(batteryPowerKw).toFixed(3)}` : "—"}
          unit="kW"
          label="Battery"
          badge={batterySoc !== null ? `${batterySoc.toFixed(0)}%` : null}
        />

        <CircleNode
          cx={grid.x} cy={grid.y} r={nodeR} ringW={ringW}
          ringColor={exporting ? COLOR_EXPORT : importing ? COLOR_IMPORT : COLOR_IDLE}
          haloId={exporting ? "halo-emerald" : importing ? "halo-rose" : null}
          icon={<GridIcon />}
          value={gridPowerW !== null ? `${(Math.abs(gridPowerW) / 1000).toFixed(3)}` : "—"}
          unit="kW"
          label="Grid"
        />

        <CircleNode
          cx={house.x} cy={house.y} r={nodeR} ringW={ringW}
          ringColor={consuming ? COLOR_HOUSE : COLOR_IDLE}
          haloId={consuming ? "halo-sky" : null}
          icon={<HouseIcon />}
          value={houseLoadKw !== null ? `${houseLoadKw.toFixed(3)}` : "—"}
          unit="kW"
          label="Consumption"
        />
      </svg>
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// CurvedFlow — linie curbă (quadratic bezier) cu animație stroke-dashoffset
// + săgeată la final. `reverse=true` schimbă sensul.
// ──────────────────────────────────────────────────────────────────────────
function CurvedFlow({
  from, to, control, active, color, dur, reverse,
}: {
  from: { x: number; y: number };
  to: { x: number; y: number };
  control: { x: number; y: number };
  active: boolean;
  color: string;
  dur: string;
  reverse: boolean;
}) {
  // Pentru reverse, schimbăm capetele (animația rămâne forward, dar săgeata
  // și sensul punctelor curg invers).
  const a = reverse ? to : from;
  const b = reverse ? from : to;
  const ctrl = control; // controlul rămâne același; bezier-ul e simetric vizual

  const path = `M ${a.x} ${a.y} Q ${ctrl.x} ${ctrl.y} ${b.x} ${b.y}`;
  // Path "underlay" gri (linie statică inactivă)
  const underlay = `M ${from.x} ${from.y} Q ${control.x} ${control.y} ${to.x} ${to.y}`;

  return (
    <g>
      {/* Underlay subtil pe dark theme */}
      <path
        d={underlay}
        fill="none"
        stroke="rgba(80, 90, 120, 0.25)"
        strokeWidth="2"
        strokeLinecap="round"
      />
      {active && (
        <>
          {/* Halo larg blur sub linia activă */}
          <path
            d={path}
            fill="none"
            stroke={color}
            strokeWidth="6"
            strokeLinecap="round"
            strokeOpacity="0.35"
            filter="url(#line-glow)"
          />
          {/* Linia neon principală cu dash animate */}
          <path
            d={path}
            fill="none"
            stroke={color}
            strokeWidth="3"
            strokeLinecap="round"
            strokeDasharray="6 10"
            markerEnd={`url(#flow-arrow-${color.slice(1)})`}
            filter="url(#line-glow)"
          >
            <animate
              attributeName="stroke-dashoffset"
              from="0" to="-32"
              dur={`${dur}s`}
              repeatCount="indefinite"
            />
          </path>
        </>
      )}
    </g>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// CircleNode — cerc alb cu inel colorat + icon + value + unit + label
// ──────────────────────────────────────────────────────────────────────────
function CircleNode({
  cx, cy, r, ringW, ringColor, haloId, icon, value, unit, label, badge,
}: {
  cx: number; cy: number; r: number; ringW: number;
  ringColor: string;
  haloId: string | null;
  icon: React.ReactNode;
  value: string;
  unit: string;
  label: string;
  badge?: string | null;
}) {
  // Typography spacing — cercul are r=72, conținut interior 144px diametru.
  // Distribuim: icon (sus, ~y=-30), value (mare, y=+5), unit (y=+24).
  const ICON_Y = -32;          // top of icon (translateY)
  const VALUE_Y = 8;           // baseline value (relative to cy)
  const UNIT_Y = 28;           // baseline unit
  const LABEL_Y_OFFSET = 26;   // label distance under circle bottom
  const VALUE_FONT = 22;       // mare, prominent
  const UNIT_FONT = 12;
  const LABEL_FONT = 15;

  const isEmpty = value === "—";
  const isActive = haloId !== null;

  return (
    <g>
      {/* Halo glow extern când activ */}
      {haloId && (
        <circle cx={cx} cy={cy} r={r + 22} fill={`url(#${haloId})`} pointerEvents="none">
          <animate attributeName="r" values={`${r + 16};${r + 26};${r + 16}`} dur="2.4s" repeatCount="indefinite" />
        </circle>
      )}
      {/* Cerc dark cu inel neon */}
      <circle
        cx={cx} cy={cy} r={r}
        fill="rgba(13, 17, 38, 0.9)"
        stroke={ringColor}
        strokeWidth={ringW}
        filter={isActive ? "url(#neon-glow)" : undefined}
      />
      {/* Inner ring subtle */}
      <circle
        cx={cx} cy={cy} r={r - 4}
        fill="none"
        stroke={ringColor}
        strokeWidth="1"
        strokeOpacity="0.3"
      />
      {/* Icon — sus, în culoarea inelului */}
      <g transform={`translate(${cx - 14}, ${cy + ICON_Y})`} style={{ color: ringColor, filter: isActive ? `drop-shadow(0 0 6px ${ringColor})` : undefined }}>
        {icon}
      </g>
      {/* Value */}
      <text
        x={cx} y={cy + VALUE_Y}
        textAnchor="middle"
        fontSize={VALUE_FONT}
        fontWeight="800"
        fill={isEmpty ? "#5a6587" : isActive ? ringColor : "#e0e6ff"}
        fontFamily="ui-monospace, monospace"
        style={{ letterSpacing: "-0.5px", filter: isActive && !isEmpty ? `drop-shadow(0 0 6px ${ringColor})` : undefined }}
      >
        {value}
      </text>
      {/* Unit */}
      <text
        x={cx} y={cy + UNIT_Y}
        textAnchor="middle"
        fontSize={UNIT_FONT}
        fill="#8b95b8"
        fontWeight="600"
        fontFamily="ui-sans-serif"
        style={{ letterSpacing: "0.5px" }}
      >
        {unit}
      </text>
      {/* Badge SOC */}
      {badge && (
        <g>
          <rect
            x={cx - 24} y={cy - r - 12}
            width="48" height="22" rx="11"
            fill={ringColor}
            stroke="rgba(13, 17, 38, 0.9)" strokeWidth="2"
            filter={isActive ? "url(#neon-glow)" : undefined}
          />
          <text
            x={cx} y={cy - r + 3}
            textAnchor="middle"
            fontSize="12"
            fontWeight="800"
            fill="#0b0d1e"
            fontFamily="ui-monospace, monospace"
            style={{ letterSpacing: "0.3px" }}
          >
            {badge}
          </text>
        </g>
      )}
      {/* Label sub cerc */}
      <text
        x={cx} y={cy + r + LABEL_Y_OFFSET}
        textAnchor="middle"
        fontSize={LABEL_FONT}
        fontWeight="700"
        fill={isActive ? ringColor : "#6f7a99"}
        fontFamily="ui-sans-serif"
        style={{ letterSpacing: "0.8px", filter: isActive ? `drop-shadow(0 0 4px ${ringColor})` : undefined }}
      >
        {label.toUpperCase()}
      </text>
    </g>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// Icons (inline SVG, 24×24, currentColor)
// ──────────────────────────────────────────────────────────────────────────
function PvIcon() {
  return (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none"
         stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="5" width="18" height="13" rx="1" fill="currentColor" fillOpacity="0.12" />
      <line x1="3" y1="9" x2="21" y2="9" />
      <line x1="3" y1="13" x2="21" y2="13" />
      <line x1="9" y1="5" x2="9" y2="18" />
      <line x1="15" y1="5" x2="15" y2="18" />
    </svg>
  );
}

function BatteryIcon({ soc }: { soc: number | null }) {
  const pct = soc !== null ? Math.max(0, Math.min(100, soc)) : 0;
  const fillW = (pct / 100) * 14;
  return (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none"
         stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="8" width="16" height="9" rx="1" />
      <rect x="19" y="11" width="2" height="3" rx="0.5" fill="currentColor" />
      {soc !== null && (
        <rect x="4.5" y="9.5" width={fillW} height="6" fill="currentColor" />
      )}
    </svg>
  );
}

function GridIcon() {
  return (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none"
         stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 3 L7 11 L10 11 L7 21 L17 21 L14 11 L17 11 Z"
            fill="currentColor" fillOpacity="0.12" />
    </svg>
  );
}

function HouseIcon() {
  return (
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none"
         stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 11 L12 3 L21 11 L21 21 L3 21 Z" fill="currentColor" fillOpacity="0.12" />
      <path d="M11 13 L9 17 L12 17 L11 21 L14 16 L11 16 Z" fill="currentColor" stroke="none" />
    </svg>
  );
}
