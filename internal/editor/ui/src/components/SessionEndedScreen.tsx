// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: session-ended-screen
import { PlugZap } from 'lucide-react';
import { useEditorStore } from '../store/editorStore';

// The command that restarts the editor.
//
// This used to read `parlay-studio domain-model edit`, which was wrong twice
// over: `parlay-studio` is retired to a redirect stub that exits non-zero,
// and the subcommand form `domain-model edit` never existed in either binary.
// So the single actionable instruction on a terminal error screen sent the
// user to a command that errors. It is a hardcoded string in the UI, which is
// why neither the Go build nor the parlay-studio retirement sweep caught it.
const RESTART_COMMAND = 'parlay domain-edit';

/**
 * Terminal state shown when the editing session is over.
 *
 * Two distinct situations reach here and they need different words:
 *
 *   'done'          the user ended the session deliberately, via the Done
 *                   control. The server shut down because we asked it to.
 *   'unreachable'   the server went away underneath us — killed, crashed, or
 *                   timed out — and a request failed.
 *
 * They used to share one message that asserted the server was "no longer
 * reachable", which is false on every Done path: the shutdown is in flight
 * and the process is exiting normally. Telling a user their server had become
 * unreachable when they had just clicked Done described a fault that had not
 * happened.
 *
 * No spinner, no retry, in either case — the process is going away; the user
 * restarts it from the CLI.
 */
export function SessionEndedScreen() {
  const sessionEnded = useEditorStore((s) => s.sessionEnded);
  const reason = useEditorStore((s) => s.sessionEndReason);
  if (!sessionEnded) return null;

  const endedDeliberately = reason === 'done';

  return (
    <div
      data-testid="session-ended-message"
      role="alertdialog"
      aria-label="Session ended"
      className="fixed inset-0 z-50 flex flex-col items-center justify-center gap-4 bg-slate-900/95 p-8 text-center text-slate-100"
    >
      <PlugZap className="h-10 w-10 text-amber-400" aria-hidden />
      <h1 className="text-xl font-semibold">
        {endedDeliberately ? 'Editing session finished' : 'Editing session ended'}
      </h1>
      <p className="max-w-md text-slate-300">
        {endedDeliberately
          ? 'You ended this session, and the editor server has shut down. Start it again from your terminal when you want to continue.'
          : 'The editor server is no longer reachable, so this session has ended. Restart it from your terminal to continue.'}
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
