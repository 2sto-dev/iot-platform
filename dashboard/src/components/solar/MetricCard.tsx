import { useMemo } from "react";
import type { Metric } from "./types";

export function MetricCard({ metric, value, error }: {
  metric: Metric;
  value: number | null;
  error?: boolean;
}) {
  const display = useMemo(() => {
    if (error || value === null || value === undefined) return "—";
    return Number(value).toFixed(metric.decimals ?? 2);
  }, [value, error, metric.decimals]);

  const isEmpty = display === "—";

  return (
    <div className={`group bg-white border rounded-lg px-4 py-3 transition-all hover:shadow-sm hover:border-gray-300 ${
      isEmpty ? "border-gray-100" : "border-gray-200"
    }`}>
      <p className="text-[11px] font-medium text-gray-500 uppercase tracking-wide truncate">
        {metric.label}
      </p>
      <p className="mt-1.5 flex items-baseline gap-1">
        <span className={`text-xl font-semibold tabular-nums ${isEmpty ? "text-gray-300" : "text-gray-900"}`}>
          {display}
        </span>
        {metric.unit && <span className="text-[11px] font-medium text-gray-400">{metric.unit}</span>}
      </p>
    </div>
  );
}
