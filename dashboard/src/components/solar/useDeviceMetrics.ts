import { useQueries } from "@tanstack/react-query";
import { goApi } from "../../lib/api";
import type { MetricResponse, MetricValues } from "./types";

/**
 * Hook care interoghează în paralel valorile curente pentru o listă de field-uri
 * dintr-un device, prin Go API → InfluxDB. Auto-refresh la `refetchMs`.
 *
 * Reutilizabil pentru orice device cu măsurare "devices" în Influx.
 */
export function useDeviceMetrics(
  deviceSerial: string | null,
  fields: string[],
  range: string,
  refetchMs = 5_000
): { values: MetricValues; loading: boolean; errors: Record<string, boolean> } {
  const queries = useQueries({
    queries: fields.map((f) => ({
      queryKey: ["metric", deviceSerial, f, range],
      queryFn: () =>
        goApi
          .get<MetricResponse>(`/metrics/${deviceSerial}/${f}`, { params: { range } })
          .then((r) => r.data.value),
      enabled: !!deviceSerial,
      refetchInterval: refetchMs,
      retry: false,
    })),
  });

  const values: MetricValues = {};
  const errors: Record<string, boolean> = {};
  let loading = false;
  fields.forEach((f, i) => {
    const q = queries[i];
    values[f] = (q.data as number | null | undefined) ?? null;
    errors[f] = !!q.error;
    if (q.isLoading) loading = true;
  });

  return { values, loading, errors };
}
