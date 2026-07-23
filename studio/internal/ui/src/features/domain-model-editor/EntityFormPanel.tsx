// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: entity-form-panel
import { useEffect, useState } from 'react';
import * as Select from '@radix-ui/react-select';
import * as Switch from '@radix-ui/react-switch';
import * as Label from '@radix-ui/react-label';
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
import {
  ChevronDown,
  Check,
  ArrowUp,
  ArrowDown,
  Trash2,
  GripVertical,
} from 'lucide-react';
import {
  useEditorStore,
  selectedEntity,
  renameEntityInModel,
} from '../../store/editorStore';
import { fieldTypeOptions } from '../../types/domain';
import type { DomainEntity, DomainField } from '../../types/domain';

function SortableFieldRow({
  entity,
  field,
  index,
  onMoveUp,
  onMoveDown,
  onDelete,
  isFirst,
  isLast,
}: {
  entity: DomainEntity;
  field: DomainField;
  index: number;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onDelete: () => void;
  isFirst: boolean;
  isLast: boolean;
}) {
  const id = `${entity.name}:${field.name}:${index}`;
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
      data-testid="field-row"
      data-field={field.name}
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
      <span className="flex-1 text-sm text-slate-800">{field.name}</span>
      <span className="text-xs text-slate-500">
        {field.type}
        {field.target ? ` → ${field.target}` : ''}
        {field.enum ? ` (${field.enum})` : ''}
      </span>
      <button
        type="button"
        data-testid="move-field-up"
        onClick={onMoveUp}
        disabled={isFirst}
        aria-label={`Move ${field.name} up`}
        className="rounded p-1 text-slate-400 disabled:opacity-30"
      >
        <ArrowUp className="h-4 w-4" aria-hidden />
      </button>
      <button
        type="button"
        data-testid="move-field-down"
        onClick={onMoveDown}
        disabled={isLast}
        aria-label={`Move ${field.name} down`}
        className="rounded p-1 text-slate-400 disabled:opacity-30"
      >
        <ArrowDown className="h-4 w-4" aria-hidden />
      </button>
      <button
        type="button"
        data-testid="delete-field"
        onClick={onDelete}
        aria-label={`Delete ${field.name}`}
        className="rounded p-1 text-slate-400 hover:text-red-600"
      >
        <Trash2 className="h-4 w-4" aria-hidden />
      </button>
    </li>
  );
}

/**
 * Edits a single entity: rename (with cascade to referencing ref targets),
 * add/remove/reorder fields, and choose a field type from the closed
 * vocabulary (8 scalars + declared enum names). Choosing an enum name auto-sets
 * field.enum; choosing "ref" reveals a target picker and gates add-field.
 */
export function EntityFormPanel() {
  const model = useEditorStore((s) => s.model);
  const entity = useEditorStore(selectedEntity);
  const applyModel = useEditorStore((s) => s.applyModel);
  const selectEntity = useEditorStore((s) => s.selectEntity);

  const [nameDraft, setNameDraft] = useState('');
  const [fieldName, setFieldName] = useState('');
  const [fieldType, setFieldType] = useState('string');
  const [refTarget, setRefTarget] = useState('');
  const [required, setRequired] = useState(false);

  useEffect(() => {
    setNameDraft(entity?.name ?? '');
  }, [entity?.name]);

  if (!entity) {
    return (
      <div
        data-testid="no-entity-selected"
        className="p-6 text-sm text-slate-400"
      >
        Select an entity to edit.
      </div>
    );
  }

  const otherNames = model.entities
    .filter((e) => e.name !== entity.name)
    .map((e) => e.name);
  const duplicateName =
    nameDraft.trim() !== '' && otherNames.includes(nameDraft.trim());
  const canRename =
    nameDraft.trim() !== '' && nameDraft.trim() !== entity.name && !duplicateName;

  const typeOptions = fieldTypeOptions(model);
  const refTargets = model.entities.map((e) => e.name);
  const isEnumType = model.enums.some((e) => e.name === fieldType);
  const needsTarget = fieldType === 'ref';
  const canAddField =
    fieldName.trim() !== '' && (!needsTarget || refTarget !== '');

  const commitRename = () => {
    if (!canRename) return;
    const next = renameEntityInModel(model, entity.name, nameDraft.trim());
    applyModel(next);
    selectEntity(nameDraft.trim());
  };

  const addField = () => {
    if (!canAddField) return;
    const field: DomainField = {
      name: fieldName.trim(),
      type: fieldType,
      required,
    };
    if (isEnumType) field.enum = fieldType;
    if (needsTarget) field.target = refTarget;
    const nextEntity: DomainEntity = {
      ...entity,
      fields: [...entity.fields, field],
    };
    applyModel({
      ...model,
      entities: model.entities.map((e) =>
        e.name === entity.name ? nextEntity : e,
      ),
    });
    setFieldName('');
    setFieldType('string');
    setRefTarget('');
    setRequired(false);
  };

  const reorder = (from: number, to: number) => {
    if (to < 0 || to >= entity.fields.length) return;
    const nextEntity: DomainEntity = {
      ...entity,
      fields: arrayMove(entity.fields, from, to),
    };
    applyModel({
      ...model,
      entities: model.entities.map((e) =>
        e.name === entity.name ? nextEntity : e,
      ),
    });
  };

  const deleteField = (index: number) => {
    const nextEntity: DomainEntity = {
      ...entity,
      fields: entity.fields.filter((_, i) => i !== index),
    };
    applyModel({
      ...model,
      entities: model.entities.map((e) =>
        e.name === entity.name ? nextEntity : e,
      ),
    });
  };

  const onDragEnd = (ev: DragEndEvent) => {
    const { active, over } = ev;
    if (!over || active.id === over.id) return;
    const ids = entity.fields.map((f, i) => `${entity.name}:${f.name}:${i}`);
    const from = ids.indexOf(String(active.id));
    const to = ids.indexOf(String(over.id));
    if (from !== -1 && to !== -1) reorder(from, to);
  };

  const sortableIds = entity.fields.map(
    (f, i) => `${entity.name}:${f.name}:${i}`,
  );

  return (
    <div className="flex flex-col gap-6 p-4">
      <div className="flex flex-col gap-1">
        <Label.Root
          htmlFor="entity-name"
          className="text-xs font-semibold uppercase tracking-wide text-slate-500"
        >
          Entity name
        </Label.Root>
        <div className="flex items-center gap-2">
          <input
            id="entity-name"
            data-testid="edit-entity-name"
            value={nameDraft}
            onChange={(e) => setNameDraft(e.target.value)}
            className="rounded border border-slate-300 px-2 py-1 text-sm"
          />
          <button
            type="button"
            data-testid="rename-entity"
            onClick={commitRename}
            disabled={!canRename}
            className="rounded bg-slate-900 px-3 py-1 text-sm font-medium text-white disabled:opacity-40"
          >
            Rename
          </button>
        </div>
        {duplicateName && (
          <p
            data-testid="duplicate-name-message"
            role="alert"
            className="text-sm text-red-600"
          >
            An entity named "{nameDraft.trim()}" already exists.
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Fields
        </h3>
        <DndContext collisionDetection={closestCenter} onDragEnd={onDragEnd}>
          <SortableContext
            items={sortableIds}
            strategy={verticalListSortingStrategy}
          >
            <ul className="flex flex-col gap-1">
              {entity.fields.map((field, index) => (
                <SortableFieldRow
                  key={`${field.name}:${index}`}
                  entity={entity}
                  field={field}
                  index={index}
                  isFirst={index === 0}
                  isLast={index === entity.fields.length - 1}
                  onMoveUp={() => reorder(index, index - 1)}
                  onMoveDown={() => reorder(index, index + 1)}
                  onDelete={() => deleteField(index)}
                />
              ))}
            </ul>
          </SortableContext>
        </DndContext>
      </div>

      <div className="flex flex-col gap-2 rounded-lg border border-slate-200 p-3">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Add field
        </h3>
        <input
          data-testid="new-field-name"
          value={fieldName}
          onChange={(e) => setFieldName(e.target.value)}
          placeholder="Field name"
          className="rounded border border-slate-300 px-2 py-1 text-sm"
        />

        <Select.Root
          value={fieldType}
          onValueChange={(v) => {
            setFieldType(v);
            if (v !== 'ref') setRefTarget('');
          }}
        >
          <Select.Trigger
            data-testid="choose-field-type"
            aria-label="Field type"
            className="inline-flex items-center justify-between gap-2 rounded border border-slate-300 px-2 py-1 text-sm"
          >
            <Select.Value placeholder="Choose type" />
            <Select.Icon>
              <ChevronDown className="h-4 w-4" aria-hidden />
            </Select.Icon>
          </Select.Trigger>
          <Select.Portal>
            <Select.Content className="rounded-md border border-slate-200 bg-white shadow-lg">
              <Select.Viewport className="p-1">
                {typeOptions.map((opt) => (
                  <Select.Item
                    key={opt}
                    value={opt}
                    data-testid="field-type-option"
                    className="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm data-[highlighted]:bg-slate-100"
                  >
                    <Select.ItemIndicator>
                      <Check className="h-3 w-3" aria-hidden />
                    </Select.ItemIndicator>
                    <Select.ItemText>{opt}</Select.ItemText>
                  </Select.Item>
                ))}
              </Select.Viewport>
            </Select.Content>
          </Select.Portal>
        </Select.Root>

        {needsTarget && (
          <Select.Root value={refTarget} onValueChange={setRefTarget}>
            <Select.Trigger
              data-testid="choose-ref-target"
              aria-label="Reference target"
              className="inline-flex items-center justify-between gap-2 rounded border border-slate-300 px-2 py-1 text-sm"
            >
              <Select.Value placeholder="Choose target entity" />
              <Select.Icon>
                <ChevronDown className="h-4 w-4" aria-hidden />
              </Select.Icon>
            </Select.Trigger>
            <Select.Portal>
              <Select.Content className="rounded-md border border-slate-200 bg-white shadow-lg">
                <Select.Viewport className="p-1">
                  {refTargets.map((t) => (
                    <Select.Item
                      key={t}
                      value={t}
                      data-testid="ref-target-option"
                      className="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-sm data-[highlighted]:bg-slate-100"
                    >
                      <Select.ItemText>{t}</Select.ItemText>
                    </Select.Item>
                  ))}
                </Select.Viewport>
              </Select.Content>
            </Select.Portal>
          </Select.Root>
        )}

        <div className="flex items-center gap-2">
          <Label.Root htmlFor="field-required" className="text-sm text-slate-600">
            Required
          </Label.Root>
          <Switch.Root
            id="field-required"
            data-testid="field-required"
            checked={required}
            onCheckedChange={setRequired}
            className="relative h-5 w-9 rounded-full bg-slate-300 data-[state=checked]:bg-emerald-500"
          >
            <Switch.Thumb className="block h-4 w-4 translate-x-0.5 rounded-full bg-white transition-transform data-[state=checked]:translate-x-4" />
          </Switch.Root>
        </div>

        <button
          type="button"
          data-testid="add-field"
          onClick={addField}
          disabled={!canAddField}
          className="self-start rounded bg-slate-900 px-3 py-1 text-sm font-medium text-white disabled:opacity-40"
        >
          Add field
        </button>
      </div>
    </div>
  );
}

export default EntityFormPanel;
