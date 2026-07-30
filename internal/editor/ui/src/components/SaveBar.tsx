// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: save-bar
// parlay-extends: domain-model-editor/domain-model-editor-validation/cross-cutting/validation-surfacing-integration
import { Loader2, Save } from 'lucide-react';
import { useEditorStore, isBlocked as selectIsBlocked } from '../store/editorStore';

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString();
}

/**
 * Footer save affordance. Save is disabled while the draft is clean; any edit
 * flips the store to dirty (showing the dirty indicator) and enables Save.
 * After a successful save the last-saved timestamp is shown.
 */
export function SaveBar() {
  const isDirty = useEditorStore((s) => s.isDirty);
  const isSaving = useEditorStore((s) => s.isSaving);
  const lastSavedAt = useEditorStore((s) => s.lastSavedAt);
  const save = useEditorStore((s) => s.save);
  // While any error finding exists, the Save affordance is deferred to
  // SaveGatingState's blocked state (which names the count and links to the
  // panel) rather than rendering a bare disabled button. Warnings never gate.
  const blocked = useEditorStore(selectIsBlocked);

  return (
    <div className="flex items-center justify-between gap-4 border-t border-slate-200 bg-white px-4 py-2">
      <div className="flex items-center gap-3 text-sm text-slate-600">
        {isDirty && (
          <span
            data-testid="dirty-indicator"
            className="flex items-center gap-1 text-amber-600"
          >
            <span className="h-2 w-2 rounded-full bg-amber-500" aria-hidden />
            Unsaved changes
          </span>
        )}
        {!isDirty && lastSavedAt !== null && (
          <span data-testid="last-saved-time" className="text-slate-500">
            Last saved at {formatTime(lastSavedAt)}
          </span>
        )}
      </div>

      {!blocked && (
        <button
          type="button"
          data-testid="save-button"
          disabled={!isDirty || isSaving}
          onClick={() => void save()}
          className="inline-flex items-center gap-2 rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:cursor-not-allowed disabled:opacity-40"
        >
          {isSaving ? (
            <Loader2 className="h-4 w-4 animate-spin" aria-hidden />
          ) : (
            <Save className="h-4 w-4" aria-hidden />
          )}
          Save
        </button>
      )}
    </div>
  );
}

export default SaveBar;
