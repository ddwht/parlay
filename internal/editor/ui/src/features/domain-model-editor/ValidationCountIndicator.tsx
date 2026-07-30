// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: validation-count-indicator
import {
  useEditorStore,
  errorCount as selectErrorCount,
  warningCount as selectWarningCount,
  isClean as selectIsClean,
} from '../../store/editorStore';

/**
 * Persistent header indicator near the save bar showing the current finding
 * tally (e.g. "2 errors, 1 warning"), so a clean model reads as clean at a
 * glance. Errors and warnings are counted and styled distinctly. A model with
 * zero findings shows an EXPLICIT clean state ("No problems"), not merely a
 * hidden indicator. Clicking it opens the validation panel. The count flips to
 * the clean state after the trailing revalidation clears the last finding.
 */
export function ValidationCountIndicator() {
  const errors = useEditorStore(selectErrorCount);
  const warnings = useEditorStore(selectWarningCount);
  const clean = useEditorStore(selectIsClean);
  const openValidationPanel = useEditorStore((s) => s.openValidationPanel);

  return (
    <button
      type="button"
      data-testid="open-validation-panel"
      onClick={() => openValidationPanel()}
      className="inline-flex items-center gap-2 rounded-md border border-slate-200 px-2 py-1 text-xs"
    >
      {clean ? (
        <span
          data-testid="clean-state"
          className="flex items-center gap-1 text-emerald-700"
        >
          <span className="h-2 w-2 rounded-full bg-emerald-500" aria-hidden />
          No problems
        </span>
      ) : (
        <>
          {errors > 0 && (
            <span
              data-testid="error-count-badge"
              className="rounded border border-red-400 bg-red-100 px-1.5 py-0.5 font-semibold text-red-900"
            >
              {errors} {errors === 1 ? 'error' : 'errors'}
            </span>
          )}
          {warnings > 0 && (
            <span
              data-testid="warning-count-badge"
              className="rounded border border-amber-400 bg-amber-100 px-1.5 py-0.5 font-semibold text-amber-900"
            >
              {warnings} {warnings === 1 ? 'warning' : 'warnings'}
            </span>
          )}
        </>
      )}
    </button>
  );
}

export default ValidationCountIndicator;
