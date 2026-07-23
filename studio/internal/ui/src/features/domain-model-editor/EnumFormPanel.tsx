// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: enum-form-panel
import { useEffect, useState } from 'react';
import {
  DndContext,
  closestCenter,
  type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
  arrayMove,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { ArrowUp, ArrowDown, Trash2, GripVertical } from 'lucide-react';
import * as Label from '@radix-ui/react-label';
import {
  useEditorStore,
  selectedEnum,
  renameEnumInModel,
  normalizeEnumValue,
} from '../../store/editorStore';
import { TONES } from '../../types/domain';
import type { DomainEnum, DomainEnumValue, Tone } from '../../types/domain';

/** none plus the five closed-set tones, in the order the selector offers them. */
const TONE_CHOICES: (Tone | 'none')[] = ['none', ...TONES];

/** Tailwind badge treatment per tone — the "pick by appearance" preview. */
function toneBadgeClass(tone: Tone): string {
  switch (tone) {
    case 'neutral':
      return 'bg-slate-100 text-slate-700 border-slate-300';
    case 'info':
      return 'bg-sky-100 text-sky-800 border-sky-300';
    case 'warning':
      return 'bg-amber-100 text-amber-800 border-amber-300';
    case 'danger':
      return 'bg-red-100 text-red-800 border-red-300';
    case 'success':
      return 'bg-emerald-100 text-emerald-800 border-emerald-300';
  }
}

function SortableValueRow({
  enumName,
  value,
  index,
  onMoveUp,
  onMoveDown,
  onDelete,
  isFirst,
  isLast,
}: {
  enumName: string;
  value: DomainEnumValue;
  index: number;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onDelete: () => void;
  isFirst: boolean;
  isLast: boolean;
}) {
  const id = `${enumName}:${value.value}:${index}`;
  const { attributes, listeners, setNodeRef, transform, transition } =
    useSortable({ id });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };
  return (
    <li
      ref={setNodeRef}
      style={style}
      data-testid="value-rows"
      data-value={value.value}
      className="flex items-center gap-2 rounded border border-slate-200 bg-white px-2 py-1"
    >
      <button
        type="button"
        aria-label="Drag handle"
        className="cursor-grab text-slate-400"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-4 w-4" aria-hidden />
      </button>
      <span className="flex-1 text-sm text-slate-800">{value.value}</span>
      {value.label && (
        <span className="text-xs text-slate-500">{value.label}</span>
      )}
      {value.tone && (
        <span
          className={`rounded border px-2 py-0.5 text-xs ${toneBadgeClass(
            value.tone,
          )}`}
        >
          {value.tone}
        </span>
      )}
      <button
        type="button"
        data-testid="move-value-up"
        onClick={onMoveUp}
        disabled={isFirst}
        aria-label={`Move ${value.value} up`}
        className="rounded p-1 text-slate-400 disabled:opacity-30"
      >
        <ArrowUp className="h-4 w-4" aria-hidden />
      </button>
      <button
        type="button"
        data-testid="move-value-down"
        onClick={onMoveDown}
        disabled={isLast}
        aria-label={`Move ${value.value} down`}
        className="rounded p-1 text-slate-400 disabled:opacity-30"
      >
        <ArrowDown className="h-4 w-4" aria-hidden />
      </button>
      <button
        type="button"
        data-testid="delete-value"
        onClick={onDelete}
        aria-label={`Delete ${value.value}`}
        className="rounded p-1 text-slate-400 hover:text-red-600"
      >
        <Trash2 className="h-4 w-4" aria-hidden />
      </button>
    </li>
  );
}

/**
 * Edits a single enum: rename (cascading to every referencing field's `type:`
 * and `enum:` companion keys), add/remove/reorder values, and pick each value's
 * tone by its actual badge treatment. Values are unique on `value`; unset
 * label/tone are dropped so they never serialize as empty-string keys. Delete
 * is blocked while a field still references the enum.
 */
export function EnumFormPanel() {
  const model = useEditorStore((s) => s.model);
  const enumDef = useEditorStore(selectedEnum);
  const applyModel = useEditorStore((s) => s.applyModel);
  const selectEnum = useEditorStore((s) => s.selectEnum);

  const [nameDraft, setNameDraft] = useState('');
  const [valueDraft, setValueDraft] = useState('');
  const [labelDraft, setLabelDraft] = useState('');
  const [toneDraft, setToneDraft] = useState<Tone | 'none'>('none');
  const [dupError, setDupError] = useState<string | null>(null);

  useEffect(() => {
    setNameDraft(enumDef?.name ?? '');
  }, [enumDef?.name]);

  if (!enumDef) {
    return (
      <div data-testid="no-enum-selected" className="p-6 text-sm text-slate-400">
        Select an enum to edit.
      </div>
    );
  }

  const otherNames = model.enums
    .filter((e) => e.name !== enumDef.name)
    .map((e) => e.name);
  const duplicateName =
    nameDraft.trim() !== '' && otherNames.includes(nameDraft.trim());
  const canRename =
    nameDraft.trim() !== '' && nameDraft.trim() !== enumDef.name && !duplicateName;

  // Fields whose enum companion key names this enum block a delete.
  const enumReferents: string[] = [];
  for (const entity of model.entities) {
    for (const field of entity.fields) {
      if (field.enum === enumDef.name) {
        enumReferents.push(`${entity.name}.${field.name}`);
      }
    }
  }
  const referenced = enumReferents.length > 0;

  const existingValues = enumDef.values.map((v) => v.value);
  const duplicateValue =
    valueDraft.trim() !== '' && existingValues.includes(valueDraft.trim());
  const canAddValue = valueDraft.trim() !== '' && !duplicateValue;

  const updateEnum = (next: DomainEnum) => {
    applyModel({
      ...model,
      enums: model.enums.map((e) => (e.name === enumDef.name ? next : e)),
    });
  };

  const commitRename = () => {
    if (!canRename) return;
    const next = renameEnumInModel(model, enumDef.name, nameDraft.trim());
    applyModel(next);
    selectEnum(nameDraft.trim());
  };

  const onValueInput = (raw: string) => {
    setValueDraft(raw);
    const trimmed = raw.trim();
    if (trimmed !== '' && existingValues.includes(trimmed)) {
      setDupError(`"${trimmed}" is already a value in this enum`);
    } else {
      setDupError(null);
    }
  };

  const addValue = () => {
    if (!canAddValue) return;
    const value = normalizeEnumValue(valueDraft.trim(), labelDraft, toneDraft);
    updateEnum({ ...enumDef, values: [...enumDef.values, value] });
    setValueDraft('');
    setLabelDraft('');
    setToneDraft('none');
    setDupError(null);
  };

  const reorder = (from: number, to: number) => {
    if (to < 0 || to >= enumDef.values.length) return;
    updateEnum({ ...enumDef, values: arrayMove(enumDef.values, from, to) });
  };

  const deleteValue = (index: number) => {
    updateEnum({
      ...enumDef,
      values: enumDef.values.filter((_, i) => i !== index),
    });
  };

  const deleteEnum = () => {
    if (referenced) return;
    applyModel({
      ...model,
      enums: model.enums.filter((e) => e.name !== enumDef.name),
    });
  };

  const onDragEnd = (ev: DragEndEvent) => {
    const { active, over } = ev;
    if (!over || active.id === over.id) return;
    const ids = enumDef.values.map((v, i) => `${enumDef.name}:${v.value}:${i}`);
    const from = ids.indexOf(String(active.id));
    const to = ids.indexOf(String(over.id));
    if (from !== -1 && to !== -1) reorder(from, to);
  };

  const sortableIds = enumDef.values.map(
    (v, i) => `${enumDef.name}:${v.value}:${i}`,
  );

  return (
    <div className="flex flex-col gap-6 p-4">
      <div className="flex flex-col gap-1">
        <Label.Root
          htmlFor="enum-name"
          className="text-xs font-semibold uppercase tracking-wide text-slate-500"
        >
          Enum name
        </Label.Root>
        <div className="flex items-center gap-2">
          <input
            id="enum-name"
            data-testid="enum-name"
            value={nameDraft}
            onChange={(e) => setNameDraft(e.target.value)}
            className="rounded border border-slate-300 px-2 py-1 text-sm"
          />
          <button
            type="button"
            data-testid="rename-enum"
            onClick={commitRename}
            disabled={!canRename}
            className="rounded bg-slate-900 px-3 py-1 text-sm font-medium text-white disabled:opacity-40"
          >
            Rename
          </button>
          <button
            type="button"
            data-testid="delete-enum"
            onClick={deleteEnum}
            disabled={referenced}
            aria-label={`Delete ${enumDef.name}`}
            className="rounded border border-red-300 px-3 py-1 text-sm font-medium text-red-700 disabled:opacity-40"
          >
            Delete
          </button>
        </div>
        {duplicateName && (
          <p role="alert" className="text-sm text-red-600">
            An enum named "{nameDraft.trim()}" already exists.
          </p>
        )}
        {referenced && (
          <p
            data-testid="enum-delete-block-message"
            role="alert"
            className="text-sm text-red-600"
          >
            {enumDef.name} can't be deleted — referenced by{' '}
            {enumReferents.join(', ')}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Values
        </h3>
        <DndContext collisionDetection={closestCenter} onDragEnd={onDragEnd}>
          <SortableContext
            items={sortableIds}
            strategy={verticalListSortingStrategy}
          >
            <ul className="flex flex-col gap-1">
              {enumDef.values.map((value, index) => (
                <SortableValueRow
                  key={`${value.value}:${index}`}
                  enumName={enumDef.name}
                  value={value}
                  index={index}
                  isFirst={index === 0}
                  isLast={index === enumDef.values.length - 1}
                  onMoveUp={() => reorder(index, index - 1)}
                  onMoveDown={() => reorder(index, index + 1)}
                  onDelete={() => deleteValue(index)}
                />
              ))}
            </ul>
          </SortableContext>
        </DndContext>
      </div>

      <div className="flex flex-col gap-2 rounded-lg border border-slate-200 p-3">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Add value
        </h3>
        <input
          data-testid="edit-value"
          value={valueDraft}
          onChange={(e) => onValueInput(e.target.value)}
          placeholder="value"
          className="rounded border border-slate-300 px-2 py-1 text-sm"
        />
        <input
          data-testid="edit-value-label"
          value={labelDraft}
          onChange={(e) => setLabelDraft(e.target.value)}
          placeholder="label (optional)"
          className="rounded border border-slate-300 px-2 py-1 text-sm"
        />

        <div
          data-testid="choose-tone"
          role="radiogroup"
          aria-label="Tone"
          className="flex flex-wrap items-center gap-2"
        >
          {TONE_CHOICES.map((choice) =>
            choice === 'none' ? (
              <button
                key="none"
                type="button"
                data-testid="tone-none"
                role="radio"
                aria-checked={toneDraft === 'none'}
                onClick={() => setToneDraft('none')}
                className={`rounded border px-2 py-0.5 text-xs ${
                  toneDraft === 'none'
                    ? 'border-slate-900 text-slate-900'
                    : 'border-slate-200 text-slate-400'
                }`}
              >
                none
              </button>
            ) : (
              <button
                key={choice}
                type="button"
                data-testid="tone-badge-preview"
                data-tone={choice}
                role="radio"
                aria-checked={toneDraft === choice}
                onClick={() => setToneDraft(choice)}
                className={`rounded border px-2 py-0.5 text-xs ${toneBadgeClass(
                  choice,
                )} ${toneDraft === choice ? 'ring-2 ring-slate-900' : ''}`}
              >
                {choice}
              </button>
            ),
          )}
        </div>

        <button
          type="button"
          data-testid="add-value"
          onClick={addValue}
          disabled={!canAddValue}
          className="self-start rounded bg-slate-900 px-3 py-1 text-sm font-medium text-white disabled:opacity-40"
        >
          Add value
        </button>

        {dupError && (
          <p
            data-testid="duplicate-value-message"
            role="alert"
            className="text-sm text-red-600"
          >
            {dupError}
          </p>
        )}
      </div>
    </div>
  );
}

export default EnumFormPanel;
