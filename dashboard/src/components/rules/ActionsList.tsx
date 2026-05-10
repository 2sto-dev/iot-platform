import { DndContext, closestCenter, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy, arrayMove } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { type Action, type ActionType, ACTION_TYPE_LABELS, emptyAction } from "./types";

interface Device {
  serial_number: string;
  description?: string;
}

interface Props {
  actions: Action[];
  onChange: (actions: Action[]) => void;
  devices?: Device[];
}

export function ActionsList({ actions, onChange, devices = [] }: Props) {
  function handleDragEnd(e: DragEndEvent) {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const oldIdx = actions.findIndex((a) => a.id === active.id);
    const newIdx = actions.findIndex((a) => a.id === over.id);
    onChange(arrayMove(actions, oldIdx, newIdx));
  }

  function update(idx: number, a: Action) {
    const next = [...actions];
    next[idx] = a;
    onChange(next);
  }

  function remove(idx: number) {
    onChange(actions.filter((_, i) => i !== idx));
  }

  function add(type: ActionType) {
    onChange([...actions, emptyAction(type)]);
  }

  return (
    <div>
      <DndContext collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={actions.map((a) => a.id)} strategy={verticalListSortingStrategy}>
          <div className="space-y-2 mb-3">
            {actions.map((a, i) => (
              <SortableAction key={a.id} id={a.id}>
                <ActionEditor
                  index={i + 1}
                  action={a}
                  onChange={(na) => update(i, na)}
                  onDelete={() => remove(i)}
                  devices={devices}
                />
              </SortableAction>
            ))}
          </div>
        </SortableContext>
      </DndContext>

      <div className="flex flex-wrap gap-2">
        {(Object.keys(ACTION_TYPE_LABELS) as ActionType[]).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => add(t)}
            className="text-xs px-3 py-1.5 rounded-lg bg-white border border-gray-300 text-gray-700 hover:bg-gray-50 flex items-center gap-1.5"
          >
            <span>{ACTION_TYPE_LABELS[t].icon}</span>
            <span>+ {ACTION_TYPE_LABELS[t].label}</span>
          </button>
        ))}
      </div>

      {actions.length === 0 && (
        <p className="text-xs text-gray-400 italic mt-2">No actions yet — add at least one above.</p>
      )}
    </div>
  );
}

function SortableAction({ id, children }: { id: string; children: React.ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };
  return (
    <div ref={setNodeRef} style={style} className="flex items-start gap-2">
      <button
        type="button"
        {...attributes}
        {...listeners}
        className="mt-3 text-gray-400 hover:text-gray-600 cursor-grab active:cursor-grabbing select-none"
        title="Drag to reorder"
      >
        ⠿
      </button>
      <div className="flex-1">{children}</div>
    </div>
  );
}

function ActionEditor({ index, action, onChange, onDelete, devices }: {
  index: number;
  action: Action;
  onChange: (a: Action) => void;
  onDelete: () => void;
  devices: Device[];
}) {
  const meta = ACTION_TYPE_LABELS[action.type];
  return (
    <div className="bg-white border border-gray-200 rounded-lg p-3">
      <div className="flex items-center gap-2 mb-3">
        <span className="text-lg">{meta.icon}</span>
        <span className="text-sm font-semibold text-gray-700">
          {index}. {meta.label}
        </span>
        <button
          type="button"
          onClick={onDelete}
          className="ml-auto text-red-500 hover:text-red-700 text-sm"
          title="Delete action"
        >
          ×
        </button>
      </div>

      {action.type === "downlink" && (
        <div className="grid grid-cols-2 gap-2">
          <DeviceSelect
            value={action.target_serial}
            onChange={(s) => onChange({ ...action, target_serial: s })}
            devices={devices}
            placeholder="Target device"
          />
          <input
            value={action.action}
            onChange={(e) => onChange({ ...action, action: e.target.value })}
            placeholder="action (e.g. relay_on)"
            className="border border-gray-300 rounded px-2 py-1.5 text-sm"
          />
          <input
            value={JSON.stringify(action.payload)}
            onChange={(e) => {
              try { onChange({ ...action, payload: JSON.parse(e.target.value) }); }
              catch { /* invalid JSON, păstrează vechi */ }
            }}
            placeholder='payload {"key":"val"}'
            className="col-span-2 border border-gray-300 rounded px-2 py-1.5 text-sm font-mono"
          />
        </div>
      )}

      {action.type === "notify" && (
        <div className="grid grid-cols-2 gap-2">
          <input
            type="number"
            value={action.channel_id ?? ""}
            onChange={(e) => onChange({ ...action, channel_id: e.target.value ? Number(e.target.value) : null })}
            placeholder="channel id"
            className="border border-gray-300 rounded px-2 py-1.5 text-sm"
          />
          <input
            value={action.title}
            onChange={(e) => onChange({ ...action, title: e.target.value })}
            placeholder="title"
            className="border border-gray-300 rounded px-2 py-1.5 text-sm"
          />
          <textarea
            value={action.body}
            onChange={(e) => onChange({ ...action, body: e.target.value })}
            placeholder="body — supports {{field}} placeholders"
            rows={2}
            className="col-span-2 border border-gray-300 rounded px-2 py-1.5 text-sm"
          />
        </div>
      )}

      {action.type === "webhook" && (
        <div className="grid grid-cols-4 gap-2">
          <select
            value={action.method}
            onChange={(e) => onChange({ ...action, method: e.target.value as "GET" | "POST" | "PUT" })}
            className="border border-gray-300 rounded px-2 py-1.5 text-sm bg-white"
          >
            <option>GET</option>
            <option>POST</option>
            <option>PUT</option>
          </select>
          <input
            value={action.url}
            onChange={(e) => onChange({ ...action, url: e.target.value })}
            placeholder="https://api.example.com/hook"
            className="col-span-3 border border-gray-300 rounded px-2 py-1.5 text-sm"
          />
          <textarea
            value={action.body_template}
            onChange={(e) => onChange({ ...action, body_template: e.target.value })}
            placeholder='body template — {"device":"{{serial}}"}'
            rows={2}
            className="col-span-4 border border-gray-300 rounded px-2 py-1.5 text-sm font-mono"
          />
        </div>
      )}

      {action.type === "set_shadow" && (
        <div className="space-y-2">
          <DeviceSelect
            value={action.target_serial}
            onChange={(s) => onChange({ ...action, target_serial: s })}
            devices={devices}
            placeholder="Target device"
          />
          <input
            value={JSON.stringify(action.desired)}
            onChange={(e) => {
              try { onChange({ ...action, desired: JSON.parse(e.target.value) }); }
              catch { /* keep prev */ }
            }}
            placeholder='desired state {"relay":"on"}'
            className="w-full border border-gray-300 rounded px-2 py-1.5 text-sm font-mono"
          />
        </div>
      )}
    </div>
  );
}

function DeviceSelect({ value, onChange, devices, placeholder }: {
  value: string;
  onChange: (s: string) => void;
  devices: Device[];
  placeholder: string;
}) {
  if (devices.length === 0) {
    return (
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder + " (serial)"}
        className="border border-gray-300 rounded px-2 py-1.5 text-sm font-mono"
      />
    );
  }
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="border border-gray-300 rounded px-2 py-1.5 text-sm bg-white"
    >
      <option value="">{placeholder}</option>
      {devices.map((d) => (
        <option key={d.serial_number} value={d.serial_number}>
          {d.serial_number}{d.description ? ` — ${d.description}` : ""}
        </option>
      ))}
    </select>
  );
}
