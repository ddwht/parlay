// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: validation-panel
import { useState } from 'react';
import { useEditorStore } from '../../store/editorStore';
import { WHOLE_MODEL_PATH, type Finding } from '../../lib/api';

/** Whether a finding anchors to a concrete, navigable element. */
function isNavigable(f: Finding): boolean {
  return f.field !== '' && f.field !== WHOLE_MODEL_PATH;
}

function severityRowClass(severity: string): string {
  return severity === 'warning'
    ? 'border-amber-300 bg-amber-50'
    : 'border-red-300 bg-red-50';
}

function severityBadgeClass(severity: string): string {
  return severity === 'warning'
    ? 'border-amber-400 bg-amber-100 text-amber-900'
    : 'border-red-400 bg-red-100 text-red-900';
}

/**
 * The COMPLETE validation finding list (sidebar). Each row shows the severity,
 * the closed error code, the schema's actionable fix message, and the element
 * path. Selecting a finding navigates to its owning editor surface and
 * highlights the element; a whole-model finding highlights nothing and keeps
 * its fix text in place. Errors and warnings are visually distinct. Zero
 * findings renders the EXPLICIT clean state, not merely an absence of rows.
 * Populated from the current validate response with no client-side cache.
 */
export function ValidationPanel() {
  const findings = useEditorStore((s) => s.findings);
  const selectFinding = useEditorStore((s) => s.selectFinding);
  const [inspected, setInspected] = useState<number | null>(null);

  if (findings.length === 0) {
    return (
      <section aria-label="Validation" className="flex flex-col gap-2">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Validation
        </h2>
        <div
          data-testid="empty-clean-state"
          className="rounded border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800"
        >
          No problems — the model is clean.
        </div>
      </section>
    );
  }

  return (
    <section aria-label="Validation" className="flex flex-col gap-2">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-slate-500">
        Validation
      </h2>
      <ul className="flex flex-col gap-1">
        {findings.map((f, index) => (
          <li
            key={`${f.code}:${f.field}:${index}`}
            data-testid="finding-rows"
            data-severity={f.severity}
            className={`flex flex-col gap-1 rounded border px-2 py-1 text-left ${severityRowClass(
              f.severity,
            )}`}
          >
            <button
              type="button"
              data-testid="navigate-to-element"
              onClick={() => selectFinding(f)}
              className="flex flex-col items-start gap-1 text-left"
            >
              <span className="flex items-center gap-2">
                <span
                  data-testid="severity-badge"
                  className={`rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase ${severityBadgeClass(
                    f.severity,
                  )}`}
                >
                  {f.severity}
                </span>
                <span
                  data-testid="finding-code"
                  className="font-mono text-xs text-slate-700"
                >
                  {f.code}
                </span>
              </span>
              <span data-testid="finding-message" className="text-sm text-slate-800">
                {f.fix || f.message}
              </span>
              {isNavigable(f) && (
                <span
                  data-testid="element-path"
                  className="font-mono text-[11px] text-slate-500"
                >
                  {f.field}
                </span>
              )}
            </button>

            <button
              type="button"
              data-testid="inspect-finding"
              onClick={() => setInspected(inspected === index ? null : index)}
              className="self-start text-[11px] text-slate-500 underline"
            >
              Details
            </button>
            {inspected === index && (
              <p data-testid="finding-detail" className="text-xs text-slate-600">
                {f.message}
              </p>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

export default ValidationPanel;
