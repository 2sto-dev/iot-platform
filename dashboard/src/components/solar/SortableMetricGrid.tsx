import { useEffect, useState } from "react";
import { DndContext, closestCenter, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, useSortable, rectSortingStrategy, arrayMove } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { MetricCard } from "./MetricCard";
import type { Metric, MetricValues } from "./types";

interface Props {
  /** Cheie unică pentru a persista ordinea în localStorage. */
  storageKey: string;
  metrics: Metric[];
  values: MetricValues;
  errors?: Record<string, boolean>;
}

function loadOrder(key: string, fallback: string[]): string[] {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return fallback;
    const saved: string[] = JSON.parse(raw);
    // Filtrează field-urile care nu mai există + adaugă field-uri noi la sfârșit.
    const knownSet = new Set(fallback);
    const filtered = saved.filter((f) => knownSet.has(f));
    const missing = fallback.filter((f) => !filtered.includes(f));
    return [...filtered, ...missing];
  } catch {
    return fallback;
  }
}

export function SortableMetricGrid({ storageKey, metrics, values, errors = {} }: Props) {
  const fallback = metrics.map((m) => m.field);
  const [order, setOrder] = useState<string[]>(() => loadOrder(storageKey, fallback));

  // Re-syncronizează dacă lista de metrici se schimbă (alt panel/grup).
  useEffect(() => {
    setOrder(loadOrder(storageKey, fallback));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [storageKey, metrics.length]);

  function handleDragEnd(e: DragEndEvent) {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const oldIdx = order.indexOf(String(active.id));
    const newIdx = order.indexOf(String(over.id));
    if (oldIdx < 0 || newIdx < 0) return;
    const next = arrayMove(order, oldIdx, newIdx);
    setOrder(next);
    try { localStorage.setItem(storageKey, JSON.stringify(next)); } catch { /* quota */ }
  }

  const metricByField = new Map(metrics.map((m) => [m.field, m]));
  const ordered = order.map((f) => metricByField.get(f)).filter((m): m is Metric => !!m);

  return (
    <DndContext collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={order} strategy={rectSortingStrategy}>
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
          {ordered.map((m) => (
            <SortableCard
              key={m.field}
              id={m.field}
              metric={m}
              value={values[m.field]}
              error={errors[m.field]}
            />
          ))}
        </div>
      </SortableContext>
    </DndContext>
  );
}

function SortableCard({ id, metric, value, error }: {
  id: string;
  metric: Metric;
  value: number | null;
  error?: boolean;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    cursor: "grab",
  };
  return (
    <div ref={setNodeRef} style={style} {...attributes} {...listeners} className="active:cursor-grabbing">
      <MetricCard metric={metric} value={value} error={error} />
    </div>
  );
}
