// parlay-feature: domain-model-editor/feature-contributions
// parlay-component: contribution-review-panel
import { useEffect, useState } from 'react';
import {
  loadContribution,
  type ContributionEnvelope,
  type ContributionElement,
  type ContributionConflict,
} from '../../lib/api';

/**
 * The contribution overlay: what a feature proposes against the model being
 * edited, additions highlighted and conflicts flagged.
 *
 * It renders NOTHING unless the editor was opened with `--contribution` and
 * the named feature actually has one. An ordinary editing session is the
 * common case and must look exactly as it did before contributions existed —
 * which is why the endpoint distinguishes "not reviewing anything" from "the
 * review is empty" rather than leaving the panel to infer it from a delta with
 * no entries.
 *
 * There is no Accept button here on purpose. Accepting means writing the root
 * model, and this editor already has exactly one way to do that: edit the
 * model and save it through the compare-and-swap. A second write path in a
 * review panel would be a second writer for the file the whole design exists
 * to keep single-writer. The designer folds the additions in and saves;
 * `parlay internal domain-impact --apply` is the same merge from the CLI.
 */
export function ContributionReviewPanel() {
  const [envelope, setEnvelope] = useState<ContributionEnvelope | null>(null);
  const [dismissed, setDismissed] = useState(false);

  useEffect(() => {
    let cancelled = false;
    loadContribution()
      .then((env) => {
        if (!cancelled) setEnvelope(env);
      })
      .catch(() => {
        // A failed review query must not take the editor down with it. The
        // model itself loaded through its own endpoint; someone reviewing a
        // contribution that cannot be read is better off with a working
        // editor than a blank page.
        if (!cancelled) setEnvelope({ present: false });
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (!envelope?.present || dismissed) return null;

  const additions = envelope.delta?.additions ?? [];
  const conflicts = envelope.delta?.conflicts ?? [];
  const redundant = envelope.delta?.redundant ?? [];

  return (
    <aside
      data-testid="contribution-panel"
      role="status"
      className="m-4 flex flex-col gap-3 rounded-lg border border-sky-300 bg-sky-50 p-4"
    >
      <h3 className="text-sm font-semibold text-sky-900">
        Proposed by{' '}
        <span data-testid="contribution-feature">{envelope.feature}</span>
      </h3>

      <p data-testid="contribution-explanation" className="text-sm text-sky-900">
        This feature proposes the changes below against the project domain
        model. Nothing here has been applied — fold in what you accept and save,
        and the rest stays proposed.
      </p>

      {conflicts.length > 0 && (
        <section data-testid="contribution-conflicts">
          <h4 className="text-sm font-semibold text-red-800">
            Conflicts ({conflicts.length})
          </h4>
          <p className="text-sm text-red-800">
            The project model already describes these differently. Which
            description is right is a design decision — the merge refuses rather
            than picking one.
          </p>
          <ul className="mt-1 flex flex-col gap-1">
            {conflicts.map((c: ContributionConflict) => (
              <li
                key={c.path}
                data-testid="conflict-entry"
                className="rounded border border-red-200 bg-white px-2 py-1 text-sm text-slate-700"
              >
                <code className="text-xs text-slate-500">{c.path}</code>
                <div>
                  project model: <strong>{c.root}</strong>
                </div>
                <div>
                  proposed: <strong>{c.proposed}</strong>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}

      {additions.length > 0 && (
        <section data-testid="contribution-additions">
          <h4 className="text-sm font-semibold text-sky-900">
            Additions ({additions.length})
          </h4>
          <ul className="mt-1 flex flex-col gap-1">
            {additions.map((a: ContributionElement) => (
              <li
                key={a.path}
                data-testid="addition-entry"
                className="rounded border border-sky-200 bg-white px-2 py-1 text-sm text-slate-700"
              >
                <code className="text-xs text-slate-500">{a.path}</code>
                <div>{a.summary}</div>
              </li>
            ))}
          </ul>
        </section>
      )}

      {additions.length === 0 && conflicts.length === 0 && (
        <p data-testid="contribution-nothing-new" className="text-sm text-sky-900">
          Everything this feature proposes is already in the project model
          {redundant.length > 0 ? ` (${redundant.length} element(s))` : ''}.
        </p>
      )}

      <button
        type="button"
        data-testid="dismiss-contribution"
        onClick={() => setDismissed(true)}
        className="self-start rounded border border-sky-400 px-3 py-1 text-sm font-medium text-sky-900 hover:bg-sky-100"
      >
        Hide
      </button>
    </aside>
  );
}

export default ContributionReviewPanel;
