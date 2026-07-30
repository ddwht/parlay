// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: deprecated-operations-notice
import { useState } from 'react';
import { useEditorStore } from '../../store/editorStore';

/** Render one deprecated operation entry read-only, regardless of its shape. */
function describeOperation(op: unknown): string {
  if (op && typeof op === 'object') {
    const rec = op as Record<string, unknown>;
    const name = typeof rec.name === 'string' ? rec.name : undefined;
    const kind = typeof rec.kind === 'string' ? rec.kind : undefined;
    if (name && kind) return `${name} (${kind})`;
    if (name) return name;
  }
  return JSON.stringify(op);
}

/**
 * A read-only deprecation notice shown ONLY when the loaded model carries a
 * non-empty top-level `operations:` block. It renders each entry read-only,
 * explains that operations have moved to per-feature `capabilities.yaml`, and
 * points the designer at `parlay migrate-domain-operations`. There is no
 * create/edit/delete affordance for the entries — the only offered action is
 * acknowledging the notice. With an empty or absent `operations:` field nothing
 * renders, so a designer on a clean model never learns the construct existed.
 * The notice is informational, not blocking, and never gates a save; structural
 * passthrough of the operations block on save is the mvp serializer's job.
 */
export function DeprecatedOperationsNotice() {
  const operations = useEditorStore((s) => s.model.operations);
  const [acknowledged, setAcknowledged] = useState(false);

  const hasOperations = Array.isArray(operations) && operations.length > 0;
  if (!hasOperations || acknowledged) return null;

  return (
    <aside
      data-testid="notice-panel"
      role="status"
      className="m-4 flex flex-col gap-3 rounded-lg border border-amber-300 bg-amber-50 p-4"
    >
      <h3 className="text-sm font-semibold text-amber-900">
        Deprecated: top-level operations
      </h3>

      <ul className="flex flex-col gap-1">
        {operations.map((op, index) => (
          <li
            key={index}
            data-testid="operation-entries"
            className="rounded border border-amber-200 bg-white px-2 py-1 text-sm text-slate-700"
          >
            {describeOperation(op)}
          </li>
        ))}
      </ul>

      <p
        data-testid="deprecation-explanation"
        className="text-sm text-amber-900"
      >
        Operations have moved out of the domain model into each feature's{' '}
        <code className="rounded bg-amber-100 px-1">capabilities.yaml</code>. The
        entries above are shown read-only and cannot be edited here.
      </p>

      <p data-testid="migration-pointer" className="text-sm text-amber-900">
        Run{' '}
        <code className="rounded bg-amber-100 px-1">
          parlay migrate-domain-operations
        </code>{' '}
        to move them into per-feature{' '}
        <code className="rounded bg-amber-100 px-1">capabilities.yaml</code>{' '}
        stubs.
      </p>

      <button
        type="button"
        data-testid="acknowledge-notice"
        onClick={() => setAcknowledged(true)}
        className="self-start rounded border border-amber-400 px-3 py-1 text-sm font-medium text-amber-900 hover:bg-amber-100"
      >
        OK
      </button>
    </aside>
  );
}

export default DeprecatedOperationsNotice;
