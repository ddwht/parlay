// parlay-feature: domain-model-editor/domain-model-editor-mvp
// parlay-component: cross-cutting/embedded-ui-bundle-and-stack-pin
import { useEffect } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { useEditorStore } from './store/editorStore';
import DomainModelEditorPage from './features/domain-model-editor/DomainModelEditorPage';

/**
 * Client-side router. The editor lives at /domain-model; every other client
 * route redirects into it so the shell is always the landing view. The router
 * only ever sees client paths — /api/* requests hit the Go harness directly and
 * are never intercepted here.
 */
export function App() {
  const load = useEditorStore((s) => s.load);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <Routes>
      <Route path="/domain-model" element={<DomainModelEditorPage />} />
      <Route path="/" element={<Navigate to="/domain-model" replace />} />
      <Route path="*" element={<Navigate to="/domain-model" replace />} />
    </Routes>
  );
}

export default App;
