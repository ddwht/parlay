// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
// parlay-extends: domain-model-editor/domain-model-editor-relationships/cross-cutting/relationships-editor-integration
// parlay-extends: domain-model-editor/domain-model-editor-validation/cross-cutting/validation-surfacing-integration
import { useEffect } from 'react';
import { useEditorStore } from '../../store/editorStore';
import { shutdownSession } from '../../lib/api';
import { EntityList } from './EntityList';
import { EnumList } from './EnumList';
import { RelationshipList } from './RelationshipList';
import { EntityFormPanel } from './EntityFormPanel';
import { EnumFormPanel } from './EnumFormPanel';
import { RelationshipFormPanel } from './RelationshipFormPanel';
import { ERDiagramView } from './ERDiagramView';
import { DeprecatedOperationsNotice } from './DeprecatedOperationsNotice';
import { ValidationPanel } from './ValidationPanel';
import { ValidationCountIndicator } from './ValidationCountIndicator';
import { DoneControl } from './DoneControl';
import { ConflictReloadPrompt } from './ConflictReloadPrompt';
import { SaveBar } from '../../components/SaveBar';
import { SaveGatingState } from '../../components/SaveGatingState';
import { ErrorEnvelopeFeedback } from '../../components/ErrorEnvelopeFeedback';
import { SessionEndedScreen } from '../../components/SessionEndedScreen';

/**
 * The Domain Model Editor page — the single route the SPA fallback lands on.
 * It assembles the editor fragments across the five layout regions:
 *
 *   header  → DoneControl
 *   sidebar → EntityList, EnumList, RelationshipList
 *   main    → a Form|Diagram tab split:
 *               form    → EntityFormPanel | EnumFormPanel | RelationshipFormPanel
 *                         (by selection), ErrorEnvelopeFeedback
 *               diagram → ERDiagramView
 *             plus the self-hiding DeprecatedOperationsNotice
 *   footer  → SaveBar
 *   dialog  → ConflictReloadPrompt, SessionEndedScreen
 *
 * The form panels and the diagram side panels are views over one draft, so an
 * edit in either is immediately reflected in the other and the save bar's dirty
 * state covers both.
 */
export function DomainModelEditorPage() {
  const isDirty = useEditorStore((s) => s.isDirty);
  const selection = useEditorStore((s) => s.selection);
  const activeTab = useEditorStore((s) => s.activeTab);
  const setActiveTab = useEditorStore((s) => s.setActiveTab);

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

  // Ending the session means ending the PROCESS, not just painting an
  // overlay. `parlay domain-edit` is a blocking hook whose process exit is
  // the completion signal for whatever invoked it, so a Done control that
  // only flipped `sessionEnded` left the server serving and the caller
  // blocked until the idle timeout — 30 minutes by default. An agent that
  // told the user "click Done when you're finished" waited out that timeout
  // regardless of what the user did.
  //
  // The flag is still set, and set unconditionally: the request is fire-and-
  // forget by design (shutdownSession never throws, because the server tears
  // the listener down as it goes and the cut-off request IS the success
  // path), so the overlay must not be contingent on it resolving.
  const endSession = () => {
    void shutdownSession();
    useEditorStore.setState({ sessionEnded: true, sessionEndReason: 'done' });
  };

  return (
    <div className="flex h-screen flex-col bg-slate-50 text-slate-900">
      <header className="flex items-center justify-between border-b border-slate-200 bg-white px-4 py-3">
        <h1 className="text-base font-semibold">Domain Model Editor</h1>
        <div className="flex items-center gap-3">
          <ValidationCountIndicator />
          <DoneControl onShutdown={endSession} />
        </div>
      </header>

      <div className="flex min-h-0 flex-1">
        <aside className="w-64 shrink-0 overflow-y-auto border-r border-slate-200 bg-white p-4">
          <div className="flex flex-col gap-6">
            <EntityList />
            <EnumList />
            <RelationshipList />
            <ValidationPanel />
          </div>
        </aside>

        <main className="flex min-w-0 flex-1 flex-col overflow-y-auto">
          <div
            role="tablist"
            aria-label="Editor view"
            className="flex shrink-0 gap-1 border-b border-slate-200 bg-white px-4"
          >
            <button
              type="button"
              role="tab"
              data-testid="tab-form"
              aria-selected={activeTab === 'form'}
              onClick={() => setActiveTab('form')}
              className={`-mb-px border-b-2 px-3 py-2 text-sm font-medium ${
                activeTab === 'form'
                  ? 'border-slate-900 text-slate-900'
                  : 'border-transparent text-slate-500 hover:text-slate-700'
              }`}
            >
              Form editor
            </button>
            <button
              type="button"
              role="tab"
              data-testid="tab-diagram"
              aria-selected={activeTab === 'diagram'}
              onClick={() => setActiveTab('diagram')}
              className={`-mb-px border-b-2 px-3 py-2 text-sm font-medium ${
                activeTab === 'diagram'
                  ? 'border-slate-900 text-slate-900'
                  : 'border-transparent text-slate-500 hover:text-slate-700'
              }`}
            >
              Diagram
            </button>
          </div>

          <div role="tabpanel" className="min-h-0 flex-1">
            {activeTab === 'diagram' ? (
              <ERDiagramView />
            ) : selection?.kind === 'enum' ? (
              <EnumFormPanel />
            ) : selection?.kind === 'relationship' ? (
              <RelationshipFormPanel />
            ) : (
              <EntityFormPanel />
            )}
          </div>

          <DeprecatedOperationsNotice />
          <ErrorEnvelopeFeedback />
        </main>
      </div>

      <footer className="shrink-0">
        <div className="px-4 pt-2">
          <SaveGatingState />
        </div>
        <SaveBar />
      </footer>

      <ConflictReloadPrompt />
      <SessionEndedScreen />
    </div>
  );
}

export default DomainModelEditorPage;
