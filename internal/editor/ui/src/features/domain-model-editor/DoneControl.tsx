// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: done-control
import * as AlertDialog from '@radix-ui/react-alert-dialog';
import { useState } from 'react';
import { Check } from 'lucide-react';
import { useEditorStore } from '../../store/editorStore';

export interface DoneControlProps {
  /** Ends the editing session (server shutdown / window close). */
  onShutdown?: () => void;
}

/**
 * "Done" ends the session. If there is an unsaved draft, clicking Done does NOT
 * shut down immediately — it opens a prompt. Only after the user chooses does
 * the session end. confirm-save-and-finish saves (full etag flow) *before*
 * shutting down.
 */
export function DoneControl({ onShutdown }: DoneControlProps) {
  const isDirty = useEditorStore((s) => s.isDirty);
  const isSaving = useEditorStore((s) => s.isSaving);
  const save = useEditorStore((s) => s.save);
  const [promptOpen, setPromptOpen] = useState(false);

  const shutdown = () => {
    onShutdown?.();
  };

  const onDoneClick = () => {
    if (isDirty) {
      setPromptOpen(true);
    } else {
      shutdown();
    }
  };

  const onSaveAndFinish = async () => {
    const result = await save();
    setPromptOpen(false);
    // Only shut down once the save has actually completed.
    if (result !== null) shutdown();
  };

  const onDiscardAndFinish = () => {
    setPromptOpen(false);
    shutdown();
  };

  return (
    <AlertDialog.Root open={promptOpen} onOpenChange={setPromptOpen}>
      <button
        type="button"
        data-testid="done-button"
        onClick={onDoneClick}
        className="inline-flex items-center gap-2 rounded-md bg-emerald-600 px-4 py-2 text-sm font-medium text-white"
      >
        <Check className="h-4 w-4" aria-hidden />
        Done
      </button>

      <AlertDialog.Portal>
        <AlertDialog.Overlay className="fixed inset-0 z-40 bg-black/40" />
        <AlertDialog.Content
          data-testid="dirty-done-prompt"
          className="fixed left-1/2 top-1/2 z-50 w-[28rem] max-w-[90vw] -translate-x-1/2 -translate-y-1/2 rounded-lg bg-white p-6 shadow-xl"
        >
          <AlertDialog.Title className="text-lg font-semibold text-slate-900">
            You have unsaved changes
          </AlertDialog.Title>
          <AlertDialog.Description className="mt-2 text-sm text-slate-600">
            Save your changes before ending the session, or discard them.
          </AlertDialog.Description>

          <div className="mt-6 flex justify-end gap-3">
            <AlertDialog.Cancel asChild>
              <button
                type="button"
                data-testid="cancel-done"
                className="rounded-md border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700"
              >
                Keep editing
              </button>
            </AlertDialog.Cancel>
            <button
              type="button"
              data-testid="discard-and-finish"
              onClick={onDiscardAndFinish}
              className="rounded-md border border-red-300 px-4 py-2 text-sm font-medium text-red-700"
            >
              Discard &amp; finish
            </button>
            <button
              type="button"
              data-testid="confirm-save-and-finish"
              disabled={isSaving}
              onClick={() => void onSaveAndFinish()}
              className="rounded-md bg-slate-900 px-4 py-2 text-sm font-medium text-white disabled:opacity-40"
            >
              Save &amp; finish
            </button>
          </div>
        </AlertDialog.Content>
      </AlertDialog.Portal>
    </AlertDialog.Root>
  );
}

export default DoneControl;
