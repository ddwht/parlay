// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: entity-list
import { useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import {
  useEditorStore,
  findReferenceTo,
} from '../../store/editorStore';
import type { DomainEntity } from '../../types/domain';

function uniqueEntityName(existing: string[]): string {
  let n = 1;
  let name = 'NewEntity';
  while (existing.includes(name)) {
    n += 1;
    name = `NewEntity${n}`;
  }
  return name;
}

/**
 * Sidebar list of entities. Supports new / select / delete. Deleting an entity
 * that is still referenced by another entity's ref field is blocked with an
 * explicit message naming the referent.
 */
export function EntityList() {
  const model = useEditorStore((s) => s.model);
  const selection = useEditorStore((s) => s.selection);
  const selectEntity = useEditorStore((s) => s.selectEntity);
  const applyModel = useEditorStore((s) => s.applyModel);
  const [blockMessage, setBlockMessage] = useState<string | null>(null);

  const entities = model.entities;

  const onNew = () => {
    const name = uniqueEntityName(entities.map((e) => e.name));
    const entity: DomainEntity = { name, fields: [] };
    applyModel({ ...model, entities: [...entities, entity] });
    selectEntity(name);
  };

  const onDelete = (name: string) => {
    const ref = findReferenceTo(model, name);
    if (ref) {
      setBlockMessage(`${name} can't be deleted — referenced by ${ref}`);
      return;
    }
    setBlockMessage(null);
    applyModel({
      ...model,
      entities: entities.filter((e) => e.name !== name),
    });
  };

  return (
    <section aria-label="Entities" className="flex flex-col gap-2">
      <header className="flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Entities
        </h2>
        <button
          type="button"
          data-testid="new-entity"
          onClick={onNew}
          aria-label="New entity"
          className="rounded p-1 text-slate-500 hover:bg-slate-100"
        >
          <Plus className="h-4 w-4" aria-hidden />
        </button>
      </header>

      {entities.length === 0 ? (
        <p
          data-testid="empty-entities"
          className="rounded border border-dashed border-slate-300 p-3 text-sm text-slate-400"
        >
          No entities yet. Create one to get started.
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {entities.map((entity) => {
            const active =
              selection?.kind === 'entity' && selection.name === entity.name;
            return (
              <li
                key={entity.name}
                data-testid="entity-rows"
                data-entity={entity.name}
                className={`flex items-center justify-between rounded px-2 py-1 ${
                  active ? 'bg-slate-200' : 'hover:bg-slate-100'
                }`}
              >
                <button
                  type="button"
                  data-testid="open-entity"
                  onClick={() => selectEntity(entity.name)}
                  className="flex-1 text-left text-sm text-slate-800"
                >
                  {entity.name}
                </button>
                <button
                  type="button"
                  data-testid="delete-entity"
                  data-entity={entity.name}
                  onClick={() => onDelete(entity.name)}
                  aria-label={`Delete ${entity.name}`}
                  className="rounded p-1 text-slate-400 hover:bg-slate-200 hover:text-red-600"
                >
                  <Trash2 className="h-4 w-4" aria-hidden />
                </button>
              </li>
            );
          })}
        </ul>
      )}

      {blockMessage && (
        <p
          data-testid="delete-blocked-message"
          role="alert"
          className="text-sm text-red-600"
        >
          {blockMessage}
        </p>
      )}
    </section>
  );
}

export default EntityList;
