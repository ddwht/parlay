// parlay-feature: domain-model-editor/domain-model-editor-validation
// parlay-component: inline-finding-marker
import { useState } from 'react';
import { useEditorStore, findingForElement } from '../../store/editorStore';

function markerClass(severity: string): string {
  return severity === 'warning'
    ? 'border-amber-400 bg-amber-100 text-amber-900'
    : 'border-red-400 bg-red-100 text-red-900';
}

/**
 * The anchored, in-view rendering of a finding at the element it names. Renders
 * ONLY for a finding whose element (`path`) is representable in the current
 * view — the validation panel remains the complete list. Error and warning
 * markers are visually distinct, matching the finding's severity. Presentation
 * only: a marker never blocks an editing gesture and never mutates the draft.
 * Anchoring is driven by the store's findings-by-element-path selector, so a
 * cross-element state a form cannot see locally (a ref whose target was
 * hand-deleted, a fixture migrated with an out-of-set type) still surfaces
 * here.
 */
export function InlineFindingMarker({ path }: { path: string }) {
  const finding = useEditorStore((s) => findingForElement(s, path));
  const [inspecting, setInspecting] = useState(false);

  if (!finding) return null;

  return (
    <span className="relative inline-flex items-center">
      <button
        type="button"
        data-testid="marker-badge"
        data-severity={finding.severity}
        aria-label={`${finding.severity}: ${finding.code}`}
        onClick={() => setInspecting((v) => !v)}
        className={`rounded border px-1.5 py-0.5 text-[10px] font-semibold uppercase ${markerClass(
          finding.severity,
        )}`}
      >
        {finding.severity === 'warning' ? 'warn' : 'error'}
      </button>
      {inspecting && (
        <span
          data-testid="marker-message"
          role="tooltip"
          className="absolute left-0 top-full z-10 mt-1 w-64 rounded border border-slate-300 bg-white px-2 py-1 text-xs text-slate-700 shadow"
        >
          {finding.fix || finding.message}
        </span>
      )}
    </span>
  );
}

export default InlineFindingMarker;
