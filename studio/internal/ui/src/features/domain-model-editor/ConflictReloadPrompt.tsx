// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: conflict-reload-prompt
import * as AlertDialog from '@radix-ui/react-alert-dialog';
import { useEditorStore } from '../../store/editorStore';

/**
 * On a 409 the save is refused rather than silently overwriting on-disk state.
 * The user must choose: reload-and-reapply (refetch authoritative model, adopt
 * its etag, no auto re-save) or keep-my-draft (dismiss and keep editing).
 */
export function ConflictReloadPrompt() {
  const conflict = useEditorStore((s) => s.conflict);
  const reloadAndReapply = useEditorStore((s) => s.reloadAndReapply);
  const keepDraft = useEditorStore((s) => s.keepDraft);

  const open = conflict !== null;

  return (
    <AlertDialog.Root open={open}>
      <AlertDialog.Portal>
        <AlertDialog.Overlay className="fixed inset-0 z-40 bg-black/40" />
        <AlertDialog.Content
          data-testid="conflict-reload-prompt"
          className="fixed left-1/2 top-1/2 z-50 w-[28rem] max-w-[90vw] -translate-x-1/2 -translate-y-1/2 rounded-lg bg-white p-6 shadow-xl"
        >
          <AlertDialog.Title className="text-lg font-semibold text-slate-900">
            The model changed on disk
          </AlertDialog.Title>
          <AlertDialog.Description
            data-testid="conflict-explanation"
            className="mt-2 text-sm text-slate-600"
          >
            Someone or something updated the domain model after you loaded it,
            so your save was refused to avoid overwriting their changes. Reload
            the latest version and reapply your edits, or keep your draft to
            reconcile manually.
          </AlertDialog.Description>

          <div className="mt-6 flex justify-end gap-3">
            <AlertDialog.Cancel asChild>
              <button
                type="button"
                data-testid="keep-my-draft"
                onClick={() => keepDraft()}
                className="rounded-md border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700"
              >
                Keep my draft
              </button>
            </AlertDialog.Cancel>
            <AlertDialog.Action asChild>
              <button
                type="button"
                data-testid="reload-and-reapply"
                onClick={() => void reloadAndReapply()}
                className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white"
              >
                Reload &amp; reapply
              </button>
            </AlertDialog.Action>
          </div>
        </AlertDialog.Content>
      </AlertDialog.Portal>
    </AlertDialog.Root>
  );
}

export default ConflictReloadPrompt;
