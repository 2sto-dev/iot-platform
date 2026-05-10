export interface Metric {
  field: string;
  label: string;
  unit: string;
  decimals?: number;
}

export interface MetricGroup {
  key: string;
  label: string;
  description: string;
  metrics: Metric[];
  /** Tailwind color name (amber, blue, emerald, violet, rose, gray) */
  color: "amber" | "blue" | "emerald" | "violet" | "rose" | "gray";
}

export const RANGE_OPTIONS = [
  { value: "-5m", label: "5 min" },
  { value: "-15m", label: "15 min" },
  { value: "-1h", label: "1 hour" },
  { value: "-24h", label: "24 hours" },
];

export interface MetricResponse {
  device: string;
  field: string;
  value: number | null;
  tenant_id: number;
}

/** Map de la field name la valoarea curentă (sau null dacă lipsește/eroare). */
export type MetricValues = Record<string, number | null>;

export const HEADER_COLORS: Record<MetricGroup["color"], {
  dot: string;        // legacy small dot
  text: string;       // title text color
  bar: string;        // vertical accent bar
  eyebrow: string;    // small uppercase eyebrow text color
  bg: string;         // light tonal background pt section header row
  border: string;     // border subtil pe row
}> = {
  amber:   { dot: "bg-amber-500",   text: "text-gray-900", bar: "bg-amber-500",   eyebrow: "text-amber-700",   bg: "bg-amber-50",   border: "border-amber-200" },
  blue:    { dot: "bg-sky-500",     text: "text-gray-900", bar: "bg-sky-500",     eyebrow: "text-sky-700",     bg: "bg-sky-50",     border: "border-sky-200" },
  emerald: { dot: "bg-emerald-500", text: "text-gray-900", bar: "bg-emerald-500", eyebrow: "text-emerald-700", bg: "bg-emerald-50", border: "border-emerald-200" },
  violet:  { dot: "bg-indigo-500",  text: "text-gray-900", bar: "bg-indigo-500",  eyebrow: "text-indigo-700",  bg: "bg-indigo-50",  border: "border-indigo-200" },
  rose:    { dot: "bg-rose-500",    text: "text-gray-900", bar: "bg-rose-500",    eyebrow: "text-rose-700",    bg: "bg-rose-50",    border: "border-rose-200" },
  gray:    { dot: "bg-slate-400",   text: "text-gray-900", bar: "bg-slate-400",   eyebrow: "text-slate-600",   bg: "bg-slate-50",   border: "border-slate-200" },
};
