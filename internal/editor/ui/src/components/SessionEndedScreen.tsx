// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: session-ended-screen
import { PlugZap } from 'lucide-react';
import { useEditorStore } from '../store/editorStore';

const RESTART_COMMAND = 'parlay-studio domain-model edit';

/**
 * Terminal state shown when the local Studio server goes away after the user
 * has already been working. No spinner, no retry — the process is gone; the
 * user restarts it from the CLI.
 */
export function SessionEndedScreen() {
  const sessionEnded = useEditorStore((s) => s.sessionEnded);
  if (!sessionEnded) return null;

  return (
    <div
      data-testid="session-ended-message"
      role="alertdialog"
      aria-label="Session ended"
      className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-4 bg-slate-900/95 p-8 text-center text-slate-100"
    >
      <PlugZap className="h-10 w-10 text-amber-400" aria-hidden />
      <h1 className="text-xl font-semibold">Studio session ended</h1>
      <p className="max-w-md text-slate-300">
        The Parlay Studio server is no longer reachable. Your editing session
        has ended. Restart it from your terminal to continue.
      </p>
      <code
        data-testid="restart-command"
        className="rounded bg-slate-800 px-3 py-2 font-mono text-sm text-emerald-300"
      >
        {RESTART_COMMAND}
      </code>
    </div>
  );
}

export default SessionEndedScreen;
