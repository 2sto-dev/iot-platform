import { DndContext, closestCenter, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, useSortable, verticalListSortingStrategy, arrayMove } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  type Condition, type GroupCondition, type LeafCondition,
  LEAF_OPS, isGroup, emptyLeaf, emptyGroup,
} from "./types";

interface Props {
  condition: Condition;
  onChange: (c: Condition) => void;
  fieldSuggestions?: string[];
  depth?: number;
}

export function ConditionTree({ condition, onChange, fieldSuggestions = [], depth = 0 }: Props) {
  if (!isGroup(condition)) {
    return <LeafRow leaf={condition} onChange={onChange as (c: LeafCondition) => void} fieldSuggestions={fieldSuggestions} />;
  }
  return (
    <GroupBlock
      group={condition}
      onChange={onChange as (c: GroupCondition) => void}
      fieldSuggestions={fieldSuggestions}
      depth={depth}
      canDelete={false}
    />
  );
}

function GroupBlock({ group, onChange, onDelete, fieldSuggestions, depth, canDelete }: {
  group: GroupCondition;
  onChange: (g: GroupCondition) => void;
  onDelete?: () => void;
  fieldSuggestions: string[];
  depth: number;
  canDelete: boolean;
}) {
  const colors = [
    "border-blue-200 bg-blue-50/40",
    "border-violet-200 bg-violet-50/40",
    "border-amber-200 bg-amber-50/40",
    "border-emerald-200 bg-emerald-50/40",
  ];
  const color = colors[depth % colors.length];

  function handleDragEnd(e: DragEndEvent) {
    const { active, over } = e;
    if (!over || active.id === over.id) return;
    const oldIdx = group.conditions.findIndex((c) => c.id === active.id);
    const newIdx = group.conditions.findIndex((c) => c.id === over.id);
    if (oldIdx < 0 || newIdx < 0) return;
    onChange({ ...group, conditions: arrayMove(group.conditions, oldIdx, newIdx) });
  }

  function updateChild(idx: number, c: Condition) {
    const next = [...group.conditions];
    next[idx] = c;
    onChange({ ...group, conditions: next });
  }
  function deleteChild(idx: number) {
    onChange({ ...group, conditions: group.conditions.filter((_, i) => i !== idx) });
  }
  function addLeaf() {
    onChange({ ...group, conditions: [...group.conditions, emptyLeaf()] });
  }
  function addGroup() {
    onChange({ ...group, conditions: [...group.conditions, emptyGroup("OR")] });
  }

  return (
    <div className={`border-2 rounded-lg p-3 ${color}`}>
      <div className="flex items-center gap-2 mb-3">
        <select
          value={group.operator}
          onChange={(e) => onChange({ ...group, operator: e.target.value as "AND" | "OR" | "NOT" })}
          className="border border-gray-300 rounded-md px-2 py-1 text-xs font-bold bg-white"
        >
          <option value="AND">ALL of (AND)</option>
          <option value="OR">ANY of (OR)</option>
          <option value="NOT">NONE of (NOT)</option>
        </select>
        <span className="text-xs text-gray-500">
          {group.conditions.length} condition{group.conditions.length !== 1 ? "s" : ""}
        </span>
        <div className="ml-auto flex gap-1">
          <button
            type="button"
            onClick={addLeaf}
            className="text-xs px-2 py-1 rounded bg-white border border-gray-300 text-gray-700 hover:bg-gray-50"
          >
            + Condition
          </button>
          <button
            type="button"
            onClick={addGroup}
            className="text-xs px-2 py-1 rounded bg-white border border-gray-300 text-gray-700 hover:bg-gray-50"
          >
            + Group
          </button>
          {canDelete && onDelete && (
            <button
              type="button"
              onClick={onDelete}
              className="text-xs px-2 py-1 rounded bg-red-50 border border-red-200 text-red-700 hover:bg-red-100"
              title="Delete group"
            >
              ×
            </button>
          )}
        </div>
      </div>

      <DndContext collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={group.conditions.map((c) => c.id)} strategy={verticalListSortingStrategy}>
          <div className="space-y-2">
            {group.conditions.map((c, i) => (
              <SortableCondition key={c.id} id={c.id}>
                {isGroup(c) ? (
                  <GroupBlock
                    group={c}
                    onChange={(g) => updateChild(i, g)}
                    onDelete={() => deleteChild(i)}
                    fieldSuggestions={fieldSuggestions}
                    depth={depth + 1}
                    canDelete
                  />
                ) : (
                  <LeafRow
                    leaf={c}
                    onChange={(l) => updateChild(i, l)}
                    onDelete={() => deleteChild(i)}
                    fieldSuggestions={fieldSuggestions}
                  />
                )}
              </SortableCondition>
            ))}
          </div>
        </SortableContext>
      </DndContext>

      {group.conditions.length === 0 && (
        <p className="text-xs text-gray-400 italic text-center py-2">Empty group — add a condition.</p>
      )}
    </div>
  );
}

function SortableCondition({ id, children }: { id: string; children: React.ReactNode }) {
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
        className="mt-2.5 text-gray-400 hover:text-gray-600 cursor-grab active:cursor-grabbing select-none"
        title="Drag to reorder"
      >
        ⠿
      </button>
      <div className="flex-1">{children}</div>
    </div>
  );
}

function LeafRow({ leaf, onChange, onDelete, fieldSuggestions }: {
  leaf: LeafCondition;
  onChange: (l: LeafCondition) => void;
  onDelete?: () => void;
  fieldSuggestions: string[];
}) {
  const opMeta = LEAF_OPS.find((o) => o.value === leaf.op);
  const datalistId = `fields-${leaf.id}`;

  return (
    <div className="bg-white border border-gray-200 rounded-lg px-3 py-2 flex items-center gap-2 flex-wrap">
      <input
        list={datalistId}
        value={leaf.field}
        onChange={(e) => onChange({ ...leaf, field: e.target.value })}
        placeholder="field (e.g. temperature)"
        className="flex-1 min-w-[140px] border border-gray-300 rounded px-2 py-1 text-sm font-mono"
      />
      <datalist id={datalistId}>
        {fieldSuggestions.map((f) => <option key={f} value={f} />)}
      </datalist>
      <select
        value={leaf.op}
        onChange={(e) => onChange({ ...leaf, op: e.target.value as LeafCondition["op"] })}
        className="border border-gray-300 rounded px-2 py-1 text-sm bg-white"
      >
        {LEAF_OPS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
      {opMeta?.needsValue && (
        <input
          value={String(leaf.value ?? "")}
          onChange={(e) => {
            const v = e.target.value;
            const num = Number(v);
            onChange({ ...leaf, value: v !== "" && !isNaN(num) ? num : v });
          }}
          placeholder="value"
          className="flex-1 min-w-[100px] border border-gray-300 rounded px-2 py-1 text-sm"
        />
      )}
      {onDelete && (
        <button
          type="button"
          onClick={onDelete}
          className="text-red-500 hover:text-red-700 text-sm px-2"
          title="Delete condition"
        >
          ×
        </button>
      )}
    </div>
  );
}
