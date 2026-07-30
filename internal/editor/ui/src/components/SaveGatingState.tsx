// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: save-gating-state
import {
  useEditorStore,
  errorCount as selectErrorCount,
  isBlocked as selectIsBlocked,
} from '../store/editorStore';

/**
 * The save affordance's error-gated state, layered onto the mvp save bar. While
 * any error-severity finding exists the save bar shows a BLOCKED state naming
 * the error count and linking to the validation panel — it REPLACES the Save
 * affordance with an explanation rather than disabling it silently, so the
 * blocked save is never a surprise. Warning-only findings never gate. The
 * blocked state is present on load before any edit when a freshly-loaded file
 * already carries errors; it clears once the trailing revalidation clears the
 * last error. There is no force-save or override affordance — this client-side
 * state is an affordance, not the enforcement point; the authoritative gate is
 * server-side (save-validation-gate-before-cas).
 *
 * Returns null when nothing is blocked, so the SaveBar renders its normal
 * dirty/save affordance for a clean or warning-only model.
 */
export function SaveGatingState() {
  const blocked = useEditorStore(selectIsBlocked);
  const errors = useEditorStore(selectErrorCount);
  const openValidationPanel = useEditorStore((s) => s.openValidationPanel);

  if (!blocked) return null;

  return (
    <div
      data-testid="blocked-save-bar"
      role="status"
      className="flex items-center gap-3 rounded-md border border-red-300 bg-red-50 px-3 py-2 text-sm text-red-900"
    >
      <span>
        Save blocked — {errors} {errors === 1 ? 'error' : 'errors'} must be
        fixed first.
      </span>
      <button
        type="button"
        data-testid="view-problems"
        onClick={() => openValidationPanel()}
        className="rounded border border-red-400 px-2 py-0.5 text-xs font-medium hover:bg-red-100"
      >
        View problems
      </button>
    </div>
  );
}

export default SaveGatingState;
