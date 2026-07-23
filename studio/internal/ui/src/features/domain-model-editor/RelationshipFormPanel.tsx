// parlay-feature: domain-model-editor/domain-model-editor-relationships
// parlay-component: relationship-form-panel
import * as Label from '@radix-ui/react-label';
import { Trash2 } from 'lucide-react';
import {
  useEditorStore,
  selectedRelationship,
  selectedRelationshipIndex,
  CARDINALITIES,
} from '../../store/editorStore';

/**
 * Per-relationship form. A `from` entity picker, a `to` entity picker, a
 * cardinality dropdown over the closed set, and a freely-editable name field
 * pre-filled from the endpoints.
 *
 * The pickers offer exactly the declared entities (no free-text endpoint entry)
 * so an undeclared-entity reference is unrepresentable; the cardinality picker
 * offers exactly the four closed values so an unknown cardinality is
 * unrepresentable. Names are unique within the model — a duplicate is flagged
 * field-level at entry, before any save round-trip. Self-referential
 * (from == to) is accepted and not special-cased. The name pre-fill is a
 * convenience: once manually renamed, editing endpoints does not regenerate it.
 *
 * The same form drives two surfaces over one draft: the selected relationship
 * (default), and the uncommitted draw-to-connect proposal (`proposal` prop),
 * which only enters the draft when its commit button is pressed.
 */
export function RelationshipFormPanel({
  proposal = false,
}: {
  proposal?: boolean;
}) {
  const model = useEditorStore((s) => s.model);
  const selRel = useEditorStore(selectedRelationship);
  const relIndex = useEditorStore(selectedRelationshipIndex);
  const connectProposal = useEditorStore((s) => s.connectProposal);

  const setRelationshipEndpoint = useEditorStore(
    (s) => s.setRelationshipEndpoint,
  );
  const setRelationshipCardinality = useEditorStore(
    (s) => s.setRelationshipCardinality,
  );
  const setRelationshipName = useEditorStore((s) => s.setRelationshipName);
  const deleteRelationship = useEditorStore((s) => s.deleteRelationship);
  const setProposalEndpoint = useEditorStore((s) => s.setProposalEndpoint);
  const setProposalCardinality = useEditorStore(
    (s) => s.setProposalCardinality,
  );
  const setProposalName = useEditorStore((s) => s.setProposalName);
  const commitProposal = useEditorStore((s) => s.commitProposal);
  const cancelProposal = useEditorStore((s) => s.cancelProposal);

  const relationships = model.relationships ?? [];
  const entityNames = model.entities.map((e) => e.name);

  const draft = proposal ? connectProposal : selRel;

  if (!draft) {
    if (proposal) return null;
    return (
      <div
        data-testid="no-relationship-selected"
        className="p-6 text-sm text-slate-400"
      >
        Select a relationship to edit.
      </div>
    );
  }

  // Names unique within the model. In selection mode the currently-edited
  // relationship is excluded from the comparison set; in proposal mode nothing
  // is committed yet, so every declared relationship counts.
  const existingNames = proposal
    ? relationships.map((r) => r.name)
    : relationships.filter((_, i) => i !== relIndex).map((r) => r.name);
  const trimmedName = draft.name.trim();
  const duplicate = trimmedName !== '' && existingNames.includes(trimmedName);

  const onFrom = (v: string) =>
    proposal
      ? setProposalEndpoint('from', v)
      : setRelationshipEndpoint(relIndex, 'from', v);
  const onTo = (v: string) =>
    proposal
      ? setProposalEndpoint('to', v)
      : setRelationshipEndpoint(relIndex, 'to', v);
  const onCardinality = (v: string) =>
    proposal
      ? setProposalCardinality(v)
      : setRelationshipCardinality(relIndex, v);
  const onName = (v: string) =>
    proposal ? setProposalName(v) : setRelationshipName(relIndex, v);

  const canCommit =
    !duplicate && trimmedName !== '' && draft.cardinality !== '';

  return (
    <div className="flex flex-col gap-6 p-4">
      <div className="flex flex-col gap-1">
        <Label.Root
          htmlFor="relationship-name"
          className="text-xs font-semibold uppercase tracking-wide text-slate-500"
        >
          Name
        </Label.Root>
        <input
          id="relationship-name"
          data-testid="edit-name"
          value={draft.name}
          onChange={(e) => onName(e.target.value)}
          placeholder="relationship name"
          className="rounded border border-slate-300 px-2 py-1 text-sm"
        />
        {/* Display mirror of the resolved name (pre-fill or manual). */}
        <span data-testid="name-field" className="text-xs text-slate-500">
          {draft.name}
        </span>
        {duplicate && (
          <p
            data-testid="duplicate-name-message"
            role="alert"
            className="text-sm text-red-600"
          >
            A relationship named "{trimmedName}" already exists.
          </p>
        )}
      </div>

      <div className="flex flex-col gap-1">
        <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          From
        </span>
        <div
          data-testid="from-picker"
          role="radiogroup"
          aria-label="From entity"
          className="flex flex-wrap items-center gap-2"
        >
          {entityNames.map((name) => (
            <button
              key={name}
              type="button"
              data-testid="from-option"
              data-value={name}
              role="radio"
              aria-checked={draft.from === name}
              onClick={() => onFrom(name)}
              className={`rounded border px-2 py-0.5 text-sm ${
                draft.from === name
                  ? 'border-slate-900 text-slate-900'
                  : 'border-slate-200 text-slate-500'
              }`}
            >
              {name}
            </button>
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-1">
        <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          To
        </span>
        <div
          data-testid="to-picker"
          role="radiogroup"
          aria-label="To entity"
          className="flex flex-wrap items-center gap-2"
        >
          {entityNames.map((name) => (
            <button
              key={name}
              type="button"
              data-testid="to-option"
              data-value={name}
              role="radio"
              aria-checked={draft.to === name}
              onClick={() => onTo(name)}
              className={`rounded border px-2 py-0.5 text-sm ${
                draft.to === name
                  ? 'border-slate-900 text-slate-900'
                  : 'border-slate-200 text-slate-500'
              }`}
            >
              {name}
            </button>
          ))}
        </div>
      </div>

      <div className="flex flex-col gap-1">
        <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
          Cardinality
        </span>
        <div
          data-testid="cardinality-picker"
          role="radiogroup"
          aria-label="Cardinality"
          className="flex flex-wrap items-center gap-2"
        >
          {CARDINALITIES.map((c) => (
            <button
              key={c}
              type="button"
              data-testid="cardinality-option"
              data-value={c}
              role="radio"
              aria-checked={draft.cardinality === c}
              onClick={() => onCardinality(c)}
              className={`rounded border px-2 py-0.5 text-sm ${
                draft.cardinality === c
                  ? 'border-slate-900 text-slate-900'
                  : 'border-slate-200 text-slate-500'
              }`}
            >
              {c}
            </button>
          ))}
        </div>
      </div>

      {proposal ? (
        <div className="flex items-center gap-2">
          <button
            type="button"
            data-testid="commit-connection"
            onClick={() => commitProposal()}
            disabled={!canCommit}
            className="rounded bg-slate-900 px-3 py-1 text-sm font-medium text-white disabled:opacity-40"
          >
            Create relationship
          </button>
          <button
            type="button"
            data-testid="cancel-connection"
            onClick={() => cancelProposal()}
            className="rounded border border-slate-300 px-3 py-1 text-sm font-medium text-slate-600"
          >
            Cancel
          </button>
        </div>
      ) : (
        <button
          type="button"
          data-testid="delete-relationship"
          data-relationship={draft.name}
          onClick={() => deleteRelationship(relIndex)}
          className="inline-flex items-center gap-1 self-start rounded border border-red-300 px-3 py-1 text-sm font-medium text-red-700"
        >
          <Trash2 className="h-4 w-4" aria-hidden />
          Delete relationship
        </button>
      )}
    </div>
  );
}

export default RelationshipFormPanel;
