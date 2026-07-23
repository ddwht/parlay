// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: relationship-list
import { Plus, Trash2 } from 'lucide-react';
import { useEditorStore } from '../../store/editorStore';

/**
 * Sidebar list of the model's declared relationships — a peer of EntityList and
 * EnumList (surface order 30). Selecting a relationship opens its form panel in
 * the main region; new-relationship creates a blank draft and opens it. Delete
 * is immediate: nothing in the model references a relationship by name, so
 * there are no dependents to check. An empty-state renders when the model has
 * no relationships yet.
 */
export function RelationshipList() {
  const model = useEditorStore((s) => s.model);
  const selection = useEditorStore((s) => s.selection);
  const createRelationship = useEditorStore((s) => s.createRelationship);
  const openRelationship = useEditorStore((s) => s.openRelationship);
  const deleteRelationship = useEditorStore((s) => s.deleteRelationship);

  const relationships = model.relationships ?? [];

  const onNew = () => {
    const index = createRelationship();
    openRelationship(index);
  };

  return (
    <section aria-label="Relationships" className="flex flex-col gap-2">
      <header className="flex items-center justify-between">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Relationships
        </h2>
        <button
          type="button"
          data-testid="new-relationship"
          onClick={onNew}
          aria-label="New relationship"
          className="rounded p-1 text-slate-500 hover:bg-slate-100"
        >
          <Plus className="h-4 w-4" aria-hidden />
        </button>
      </header>

      {relationships.length === 0 ? (
        <p
          data-testid="empty-relationships"
          className="rounded border border-dashed border-slate-300 p-3 text-sm text-slate-400"
        >
          No relationships yet. Create one to connect two entities.
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {relationships.map((rel, index) => {
            const active =
              selection?.kind === 'relationship' && selection.index === index;
            const label = rel.name || '(unnamed)';
            return (
              <li
                key={`${rel.name}:${index}`}
                data-testid="relationship-rows"
                data-relationship={rel.name}
                className={`flex items-center justify-between rounded px-2 py-1 ${
                  active ? 'bg-slate-200' : 'hover:bg-slate-100'
                }`}
              >
                <button
                  type="button"
                  data-testid="open-relationship"
                  data-relationship={rel.name}
                  onClick={() => openRelationship(index)}
                  className="flex-1 text-left text-sm text-slate-800"
                >
                  <span>{label}</span>
                  {rel.from && rel.to && (
                    <span className="ml-1 text-xs text-slate-400">
                      {rel.from} → {rel.to}
                    </span>
                  )}
                </button>
                <button
                  type="button"
                  data-testid="delete-relationship"
                  data-relationship={rel.name}
                  onClick={() => deleteRelationship(index)}
                  aria-label={`Delete ${label}`}
                  className="rounded p-1 text-slate-400 hover:bg-slate-200 hover:text-red-600"
                >
                  <Trash2 className="h-4 w-4" aria-hidden />
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

export default RelationshipList;
