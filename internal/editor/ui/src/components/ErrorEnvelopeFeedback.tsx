// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: error-envelope-feedback
import * as Toast from '@radix-ui/react-toast';
import { AlertCircle } from 'lucide-react';
import { useEditorStore } from '../store/editorStore';

/**
 * Surfaces backend error envelopes.
 *  - validation-failed  -> inline field errors rendered next to their controls
 *  - server-error       -> a toast carrying the request id
 * Self-contained: provides its own Toast provider + viewport so it works both
 * standalone and inside the page shell.
 */
export function ErrorEnvelopeFeedback() {
  const fieldErrors = useEditorStore((s) => s.fieldErrors);
  const serverError = useEditorStore((s) => s.serverError);
  const clearServerError = useEditorStore((s) => s.clearServerError);

  return (
    <Toast.Provider swipeDirection="right" duration={1000000}>
      {fieldErrors.length > 0 && (
        <ul className="space-y-1" aria-label="validation errors">
          {fieldErrors.map((fe, i) => (
            <li
              key={`${fe.field}-${i}`}
              data-testid="inline-field-error"
              data-field={fe.field}
              className="flex items-center gap-1 text-sm text-red-600"
            >
              <AlertCircle className="h-4 w-4 shrink-0" aria-hidden />
              <span className="font-medium">{fe.field}</span>
              <span>{fe.message}</span>
            </li>
          ))}
        </ul>
      )}

      <Toast.Root
        open={serverError !== null}
        onOpenChange={(open) => {
          if (!open) clearServerError();
        }}
        data-testid="server-error-toast"
        className="rounded-md border border-red-300 bg-white p-3 shadow-lg"
      >
        <Toast.Title className="font-semibold text-red-700">
          Something went wrong
        </Toast.Title>
        <Toast.Description className="text-sm text-slate-600">
          The server reported an error. Request id:{' '}
          <code data-testid="server-error-request-id">
            {serverError?.requestId}
          </code>
        </Toast.Description>
      </Toast.Root>

      <Toast.Viewport className="fixed bottom-4 right-4 z-50 flex w-96 max-w-full flex-col gap-2" />
    </Toast.Provider>
  );
}

export default ErrorEnvelopeFeedback;
