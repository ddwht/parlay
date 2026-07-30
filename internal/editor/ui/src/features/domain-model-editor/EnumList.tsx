// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: enum-list
import { useState } from 'react';
import { Plus } from 'lucide-react';
import { useEditorStore, isBuiltinType } from '../../store/editorStore';
import type { DomainEnum } from '../../types/domain';

/**
 * Sidebar list of enums with an inline "create enum" field. New enum names may
 * not collide with a built-in scalar field type.
 */
export function EnumList() {
  const model = useEditorStore((s) => s.model);
  const selection = useEditorStore((s) => s.selection);
  const selectEnum = useEditorStore((s) => s.selectEnum);
  const applyModel = useEditorStore((s) => s.applyModel);
  const [draft, setDraft] = useState('');
  const [error, setError] = useState<string | null>(null);

  const enums = model.enums;

  const onCreate = () => {
    const name = draft.trim();
    if (name === '') {
      setError('Enter a name for the enum.');
      return;
    }
    if (isBuiltinType(name)) {
      setError(`"${name}" is a built-in field type name`);
      return;
    }
    if (enums.some((e) => e.name === name)) {
      setError(`"${name}" is already an enum`);
      return;
    }
    const next: DomainEnum = { name, values: [] };
    applyModel({ ...model, enums: [...enums, next] });
    selectEnum(name);
    setDraft('');
    setError(null);
  };

  return (
    <section aria-label="Enums" className="flex flex-col gap-2">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
        Enums
      </h2>

      {enums.length === 0 ? (
        <p
          data-testid="empty-enums"
          className="rounded border border-dashed border-slate-300 p-3 text-sm text-slate-400"
        >
          No enums yet.
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {enums.map((e) => {
            const active =
              selection?.kind === 'enum' && selection.name === e.name;
            return (
              <li
                key={e.name}
                data-testid="enum-rows"
                data-enum={e.name}
                className={`rounded px-2 py-1 ${
                  active ? 'bg-slate-200' : 'hover:bg-slate-100'
                }`}
              >
                <button
                  type="button"
                  data-testid="open-enum"
                  onClick={() => selectEnum(e.name)}
                  className="w-full text-left text-sm text-slate-800"
                >
                  {e.name}
                </button>
              </li>
            );
          })}
        </ul>
      )}

      <div className="flex items-center gap-1">
        <input
          data-testid="new-enum-name"
          value={draft}
          onChange={(ev) => {
            setDraft(ev.target.value);
            if (error) setError(null);
          }}
          placeholder="New enum name"
          className="min-w-0 flex-1 rounded border border-slate-300 px-2 py-1 text-sm"
        />
        <button
          type="button"
          data-testid="new-enum"
          onClick={onCreate}
          aria-label="Create enum"
          className="rounded p-1 text-slate-500 hover:bg-slate-100"
        >
          <Plus className="h-4 w-4" aria-hidden />
        </button>
      </div>

      {error && (
        <p
          data-testid="new-enum-error"
          role="alert"
          className="text-sm text-red-600"
        >
          {error}
        </p>
      )}
    </section>
  );
}

export default EnumList;
