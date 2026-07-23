// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
import { useEffect } from 'react';
import { useEditorStore } from '../../store/editorStore';
import { EntityList } from './EntityList';
import { EnumList } from './EnumList';
import { EntityFormPanel } from './EntityFormPanel';
import { EnumFormPanel } from './EnumFormPanel';
import { DoneControl } from './DoneControl';
import { ConflictReloadPrompt } from './ConflictReloadPrompt';
import { SaveBar } from '../../components/SaveBar';
import { ErrorEnvelopeFeedback } from '../../components/ErrorEnvelopeFeedback';
import { SessionEndedScreen } from '../../components/SessionEndedScreen';

/**
 * The Domain Model Editor page — the single route the SPA fallback lands on.
 * It assembles the nine editor fragments across the five layout regions:
 *
 *   header  → DoneControl
 *   sidebar → EntityList, EnumList
 *   main    → EntityFormPanel | EnumFormPanel (by selection), ErrorEnvelopeFeedback
 *   footer  → SaveBar
 *   dialog  → ConflictReloadPrompt, SessionEndedScreen
 */
export function DomainModelEditorPage() {
  const isDirty = useEditorStore((s) => s.isDirty);
  const selection = useEditorStore((s) => s.selection);

  // A dirty draft warns before the tab is closed or reloaded.
  useEffect(() => {
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (isDirty) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, [isDirty]);

  const endSession = () => {
    useEditorStore.setState({ sessionEnded: true });
  };

  return (
    <div className="flex h-screen flex-col bg-slate-50 text-slate-900">
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3">
        <h1 className="text-base font-semibold">Domain Model Editor</h1>
        <DoneControl onShutdown={endSession} />
      </header>

      <div className="flex min-h-0 flex-1">
        <aside className="w-64 shrink-0 overflow-y-auto border-r border-slate-200 bg-white p-4">
          <div className="flex flex-col gap-6">
            <EntityList />
            <EnumList />
          </div>
        </aside>

        <main className="min-w-0 flex-1 overflow-y-auto">
          {selection?.kind === 'enum' ? <EnumFormPanel /> : <EntityFormPanel />}
          <ErrorEnvelopeFeedback />
        </main>
      </div>

      <footer className="shrink-0">
        <SaveBar />
      </footer>

      <ConflictReloadPrompt />
      <SessionEndedScreen />
    </div>
  );
}

export default DomainModelEditorPage;
